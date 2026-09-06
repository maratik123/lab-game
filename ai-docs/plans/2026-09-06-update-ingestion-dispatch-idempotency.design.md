# Design: Update ingestion — long polling, dispatch, operation idempotency, chat allowlist

**Issue:** #22
**Date:** 2026-09-06

## Approach

A new `internal/ingest` package holds the whole front door: the long poll, the router, the
per-update transaction with its bounded retry, the persisted offset, the give-up record, the
observation seam, and the `tg.Gate` implementation the allowlist rides on. Its structural model is
`internal/scheduler` — a Postgres-backed loop with an immutable registry, one transaction per
attempt handed to a consumer-declared handler, a terminal give-up state, and an observer interface
with no exporter — because that shape is already shipped
[measured 1fce8b5:internal/scheduler/doc.go:1-4 · `sed -n '1,4p' internal/scheduler/doc.go` →
`Package scheduler implements a Postgres-backed task scheduler: one worker over the scheduled_task
table, claiming due rows with FOR NO KEY UPDATE ... SKIP LOCKED and executing each in its own
transaction`], already tested against a real database
[measured 1fce8b5:ai-docs/go-test-conventions.md:42 · `grep -n 'testdb.Schema'
ai-docs/go-test-conventions.md` → `42:- Integration tests run against a real Postgres provisioned by
internal/testdb … Each test takes its own schema from testdb.Schema(t) and applies the same
migrations production applies (store.Migrate).`], and the spec names it as the precedent (spec Scope 5, Scope 12).

### Key decisions

**D1 — `internal/ingest`, one package, no sub-packages.** The loop, the router, the offset, the
give-up store, the observer and the gate are one cohesive unit with one contract surface; splitting
them would produce packages that only ever import each other. The package imports `internal/tg`
(the client and the `Gate`/`Call` types), `internal/config`, `internal/store` (the
`ErrAlreadyPosted` sentinel and the owner read of D11), `internal/backoff` (D2) and `telego`.
`internal/tg` gains no import of `internal/store` and no pool: the gate is consumer-declared there
[measured 1fce8b5:internal/tg/gate.go:66-79 · `sed -n '66,79p' internal/tg/gate.go` → `Gate is the
outbound seam #22 installs its ALLOWED_CHAT_IDS allowlist into … This package installs no allowlist
itself`], so its implementation lives here (spec Scope 8's fourth property, AC15).

**D2 — the exponential ramp is lifted into `internal/backoff`, not copied again.** `internal/tg`
and `internal/scheduler` already own a `min(base·2^i, ceiling)` ramp: the equal-jitter delay
[measured 1fce8b5:internal/tg/retry.go:30-44 · `sed -n '30,44p' internal/tg/retry.go` → `func
backoffDelay(base, maxDelay time.Duration, attempt int, jitter func() float64) time.Duration`] and
`internal/scheduler`'s plain doubling [measured 1fce8b5:internal/scheduler/cadence.go:40-55 ·
`sed -n '40,55p' internal/scheduler/cadence.go` → `func backoff(failures int, base, ceiling
time.Duration) time.Duration`]. AC23 needs one more. `design-writer` § Rules forbids another
copy-paste and forbids "minimal surface" as the justification for one, so this task creates
`internal/backoff` exporting `Exponential(attempt int, base, ceiling time.Duration)` (zero-based
attempt) and `EqualJitter(attempt int, base, ceiling time.Duration, jitter func() float64)`, and
re-expresses **both** existing sites in terms of it — `internal/tg` keeps its own `defaultJitter`
and its jitter-bounds contract, `internal/scheduler` keeps its one-based `failures` call shape at
the call site. Adopting it in only one of them would leave the shared package standing beside a
surviving copy of the same ramp one directory away. The call sites after this change are
`internal/backoff` (definition), `internal/tg`, `internal/scheduler` and `internal/ingest`, and #43's
outbound queue is the "more to come" trajectory the rule names. The adoption is behaviour-preserving
by construction and is pinned by tests that already exist in both packages
[measured 1fce8b5:internal/scheduler/cadence_test.go:8,33 and internal/tg/retry_test.go:127,682 ·
`grep -n 'func Test.*[Bb]ackoff' internal/scheduler/cadence_test.go internal/tg/retry_test.go` →
`TestRetry_JitterOptionThreadedThroughToBackoff`, `TestBackoffDelay_JitterBoundsExactly`,
`TestBackoff_exactTable`, `TestBackoff_strictlyGrowingUntilCeiling`]. *Rejected:* a fourth private copy in
`internal/ingest` (the rule's named anti-pattern); a third-party backoff module (the stdlib plus
this arithmetic is the whole requirement — `AGENTS.md` § Dependency Versions asks the comparison and
there is nothing here a package would carry).

**D3 — `allowed_updates` is always transmitted, and the empty-route case rides a reserved sentinel
kind.** `telego.GetUpdatesParams.AllowedUpdates` carries `json:"allowed_updates,omitempty"`
[measured 1fce8b5:telego@v1.11.2/methods.go:28-35 · `sed -n '28,35p'
$(go env GOMODCACHE)/github.com/mymmrac/telego@v1.11.2/methods.go` → `AllowedUpdates []string
\`json:"allowed_updates,omitempty"\``] and this project marshals request parameters with
`encoding/json` [measured 1fce8b5:internal/tg/constructor.go:22-27 · `sed -n '22,27p'
internal/tg/constructor.go` → `body, err := json.Marshal(parameters)`]. `encoding/json` erases an
empty slice under `omitempty` — not only a nil one
[measured 1fce8b5 · `go doc encoding/json.Marshal` → `The "omitempty" option specifies that the
field should be omitted from the encoding if the field has an empty value, defined as false, 0, a
nil pointer, a nil interface value, and any array, slice, map, or string of length zero.`], so "send
an empty list" is unreachable through the generated params struct, and an absent parameter silently
inherits the server's previous setting (spec *Technical constraints* item 2). AC5 therefore forces a
**non-empty** list even with no route registered. The loop transmits, in that case, a one-element
list holding `telego.ShippingQueryUpdates` — the id space of an update this bot cannot receive,
because a shipping query arrives only for an invoice with a flexible price
[measured 1fce8b5:telego@v1.11.2/types.go:81 · `grep -n 'ShippingQuery - Optional'
$(go env GOMODCACHE)/github.com/mymmrac/telego@v1.11.2/types.go` → `81:	// ShippingQuery - Optional.
New incoming shipping query. Only for invoices with flexible price.`], and this project sends no
invoices. The sentinel is present **only** when the route set is empty, so a kind with no route
still never arrives once any mechanic registers one (spec Scope 3). *Rejected:* refusing to start a
routeless loop (makes AC5 vacuous and diverges from `internal/scheduler`, which ships with an empty
registry); bypassing `telego.GetUpdatesParams` with a hand-built request (breaks AC3's
one-ingestion-path guarantee).

**D4 — the update's kind is a table, not reflection; the drift check is the test's job.** `Kind` is a
string type whose values are the Bot API's `allowed_updates` tokens, which `telego` already declares
as constants [measured 1fce8b5:telego@v1.11.2/methods.go:38-65 · `sed -n '38,65p'
$(go env GOMODCACHE)/github.com/mymmrac/telego@v1.11.2/methods.go` → the `// Update types you want
your bot to receive` const block, from `MessageUpdates = "message"` to
`ManagedBot = "managed_bot"`]. Production code carries one explicit table row per token: the kind,
a `present func(*telego.Update) bool` probe, and optional extractors for the update's own date and
for its destination chat id — supplied per **payload type** rather than per kind, so the `*telego.Message`
kinds share one date/chat helper, the `*telego.ChatMemberUpdated` kinds share another, and a kind
whose payload declares neither carries nil. Derivation is total: an update matching no row yields the
zero `Kind`, which is unrouted (AC9). `NewRouter` refuses a `Kind` with no table row, so a route can
never be registered into a hole. Drift against a `telego` bump is caught by a **test** that reflects
over `telego.Update`'s exported pointer fields and asserts each one's json tag is either a table row
or a named exemption — reflection in the test, explicitness in production. *Rejected:* reflection in
production (a clever derivation for a surface a reviewer must be able to read); a table covering only
the kinds today's mechanics need (a silent hole the moment #37 or #42 registers a kind nobody added).

**D5 — the offset advances inside the handler's own transaction.** The offset is a row (spec
Scope 4), and the advance for a successfully handled update is written in the same transaction as
that handler's effects, so "handled" and "confirmed" commit together and the crash window between
them is empty. The other settled outcomes advance in a transaction of their own: an unrouted update
(no handler ran), a give-up (the failed attempt's writes are already rolled back, and the give-up row
is written with the advance), and a duplicate — where the loop rolls the attempt back first, because
`ErrAlreadyPosted` means the effects already exist from the first delivery and any other write the
handler made before `Post` is a replayed effect, not a wanted one. `store.Post`'s own contract
guarantees the transaction is still usable in that case
[measured 1fce8b5:internal/store/errors.go:32-35 · `sed -n '32,35p' internal/store/errors.go` →
`ErrAlreadyPosted is returned when the batch's PlayerOperation basis replays a (source,
operation_id) pair already posted. The transaction is usable; nothing was written.`], which is what
makes the rollback a choice rather than a necessity. The advance statement is guarded and monotone —
it writes only when the new value exceeds the stored one — so a re-run can never move the offset
backwards. AC6, AC24, AC25.

**D6 — one attempt is one transaction; a retry is a new one.** The loop begins a transaction per
attempt, hands the `pgx.Tx` to the handler, and commits or rolls back itself — `store.Post`'s
division [measured 1fce8b5:internal/store/post.go:44-46 · `sed -n '44,46p' internal/store/post.go` →
`Post applies one balanced batch of postings under basis, inside the caller's transaction tx. The
caller owns the transaction: Post neither commits nor rolls back.`] and `internal/scheduler`'s. A
failed attempt is rolled back before the next begins (AC24), the delay between attempts is
`backoff.Exponential`, and the wait selects on `ctx.Done()` so a cancellation mid-retry leaves the
update unsettled with the offset behind it (AC25). No savepoint machinery: unlike the scheduler,
this loop has no settlement statement that must survive a handler's failed subtransaction, so
rollback-and-begin-again is both simpler and stronger.

**D7 — the handler runs inline, under `recover`.** No goroutine, therefore no leak to reason about
(AC26), and no per-update execution deadline — the spec asks for none, and the scheduler's
hijack-the-connection machinery exists for a lock-release problem this loop does not have. A
recovered panic rolls the attempt back and counts as a failure for the retry policy (spec Scope 9),
and is reported to the observer as its own outcome, distinct from a returned error (AC10). The
package's non-test source contains no `panic(`, `log.Fatal` or `os.Exit` (AC11); `recover` raises
none, so `ai-docs/panic-index.md` gains no row — it is empty today and this task keeps it so
[measured 1fce8b5:ai-docs/panic-index.md · `cat ai-docs/panic-index.md` → `**The project targets
zero production panics and currently holds it** — the table below is empty.`].

**D8 — the outcome classification is a sentinel test, and duplicates never retry.** A handler error
whose chain `errors.Is`-matches `store.ErrAlreadyPosted` settles the update as a duplicate: counted
as an idempotency hit, no attempt consumed, offset advanced (AC22). Every other error, and every
recovered panic, takes the retry path. The classification survives `%w` wrapping because it is
`errors.Is`, and the `errorlint` linter already forbids the `==` comparison that would not
[measured 1fce8b5:.golangci.yml · `grep -n 'errorlint' .golangci.yml` → `- errorlint          # %w
wrapping, errors.Is/As over == and type asserts`].

**D9 — the `operation_id` grammar is `<space>:<id>`, and its derivation is unexported.** `IDSpace`
is a string type with a member for the `update_id` sequence and one for the `callback_query.id`
sequence; the builder joins the space token, `:` and the raw identifier, refusing an empty space or
an empty id. `internal/store` treats `operation_id` as opaque
[measured 1fce8b5:internal/store/basis.go:28-34 · `sed -n '28,34p' internal/store/basis.go` →
`operationID is that client's idempotency key — store treats it as opaque`], which is what makes the
grammar this package's to define. The builder is **unexported**, and `ingest.Update` carries the
derived keys as fields — the canonical one in the `update_id` space, plus the callback-query one
where the update is a callback query. That is the structural form of spec Scope 6's third
requirement: a handler cannot assemble an `operation_id` from raw Telegram fields because it has no
function to assemble one with. AC13, AC14. A third id space costs a new member and a new field, never
a migration of stored rows.

**D10 — the gate asks *who*, never *what kind of chat*.** `ingest.Gate` implements `tg.Gate`:
`ChatNone` is allowed, `ChatUnknown` is refused, and a `ChatKnown` destination is allowed when its
`Key` parses as an `int64` present in `config.Config.AllowedChatIDs`, or when an `owner` row exists
with `kind = 'player'` **and** that `telegram_id`. The predicate names the kind because the schema's
uniqueness is on the pair and a chat's own id lives in the same column
[measured 1fce8b5:internal/store/migrations/00001_ledger_core.sql:6-12 · `sed -n '6,12p'
internal/store/migrations/00001_ledger_core.sql` → `CREATE UNIQUE INDEX owner_kind_telegram_id_key
ON owner (kind, telegram_id) WHERE telegram_id IS NOT NULL`], so a lookup keyed on `telegram_id`
alone would match every chat the bot was ever added to (AC33). A token that parses as no integer —
a `@channelusername` — is refused with no special case, since it can be neither an `AllowedChatIDs`
member nor an `owner.telegram_id` (AC28). A lookup error refuses the call (AC34). The cache holds
**positive** results only, for the process's lifetime; nothing negative is remembered, so a player
who presses Start after a refusal is allowed on the next attempt with no restart (AC35).

**D11 — the owner read is `internal/store`'s, and it takes a queryer, not a transaction.** The gate
runs on the outbound path, where there is no transaction to borrow, so the new read is
`store.PlayerExists(ctx, q, telegramID)` over a consumer-declared `store.Queryer` — the single
`QueryRow(ctx, sql, args ...any) pgx.Row` method that both `pgx.Tx`
[measured 1fce8b5:pgx/v5@v5.10.0/tx.go:147 · `grep -n 'QueryRow(ctx context.Context'
$(go env GOMODCACHE)/github.com/jackc/pgx/v5@v5.10.0/tx.go` → `QueryRow(ctx context.Context, sql
string, args ...any) Row`] and `*pgxpool.Pool`
[measured 1fce8b5:pgx/v5@v5.10.0/pgxpool/pool.go:774 · same command over `pgxpool/pool.go` →
`func (p *Pool) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row`] already satisfy.
This keeps `owner`'s schema knowledge in the package that owns the table and leaves `Post`'s path
untouched (AC21). `ingest.PlayerLookup` is the consumer-declared interface the gate depends on, with
a pool-backed implementation in this package, so the gate is testable against a fake and against a
real schema without either being a mock of this package's own making (AC29).

**D12 — the observation is one per attempt plus one per attempt-less settlement.** §13.2 wants
handler duration, handler errors *and* panics, and idempotency hits on the health surface
[measured 1fce8b5:docs/DESIGN.md:410 · `sed -n '410p' docs/DESIGN.md` → `Обработка: длительность на
апдейт, ошибки/паники хендлеров, срабатывания идемпотентности (дубли — норма, всплеск — сигнал).`].
A single terminal observation per update would erase a panic that a later attempt recovered from, so
the loop reports one `Observation` per handler call, carrying the kind, the attempt number, the
outcome, the call's duration and the lag; unrouted and give-up each report one of their own. A
`LoopObservation` per cycle carries the cycle's duration, the batch size and any poll error (AC17).
The package declares the interface and imports no metrics library; a nil observer is checked, not
called (AC19). Exposition stays #23's.

**D13 — lag is reported only where the update carries its own date.** `Observation` pairs a
`time.Duration` with a boolean saying whether it is meaningful; a kind whose payload declares no date
sets the boolean false rather than a zero duration, because a zero lag is indistinguishable from a
healthy poll and §13.2 makes lag the dashboard's star metric (AC18). `telego.CallbackQuery` declares
no date field [measured 1fce8b5:telego@v1.11.2/types.go:3729-3754 · `sed -n '3729,3754p'
$(go env GOMODCACHE)/github.com/mymmrac/telego@v1.11.2/types.go` → the struct's fields are `ID`,
`From`, `Message`, `InlineMessageID`, `ChatInstance`, `Data`, `GameShortName`], while
`telego.Message` and `telego.ChatMemberUpdated` each declare a Unix `Date`
[measured 1fce8b5:telego@v1.11.2/types.go:619-620,3958-3959 · `sed -n '619,620p;3958,3959p'
$(go env GOMODCACHE)/github.com/mymmrac/telego@v1.11.2/types.go` → `// Date - Date the message was
sent in Unix time. …` / `Date int64` and `// Date - Date the change was done in Unix time` /
`Date int64`]. The extractor lives in D4's table.

**D14 — the schema is one forward migration — the offset's table and the give-up table — and the
offset is a guarded singleton.** `internal/store/migrations/00004_ingest.sql`, goose, no `-- +goose Down` section, in
the discipline `store.Migrate` already applies
[measured 1fce8b5:internal/store/migrate.go:19-23 · `sed -n '19,23p' internal/store/migrate.go` →
`Migrate applies every pending migration in internal/store/migrations to pool's database,
forward-only (no -- +goose Down section exists in this package).`]. `ingest_offset` is a single row
pinned by `CHECK (id = 1)` and seeded by the migration itself; `ingest_dead_update` carries the
update's identity, its kind, a nullable chat id, the attempt count and the last error — the shape
`scheduler.DeadTask` already has [measured 1fce8b5:internal/scheduler/task.go:78-90 · `sed -n
'78,90p' internal/scheduler/task.go` → `DeadTask is one give-up row … Type / InstanceKey / RunAt /
ConsecutiveFailures / LastError`] — and **not** the raw update payload. Table names are singular per
KD-17. **Forward-only, and its rollback:** both tables are created by this migration, so no row
predates it and no code reads a second shape during a deploy window; undoing it is a later forward
migration that drops them, at the cost of the offset (a fresh deploy then resumes from Telegram's
earliest unconfirmed update). Widening the offset to a per-bot key is likewise a later forward
migration adding a column and relaxing the `CHECK` — this task takes the MVP's one-bot-per-database
shape rather than a speculative key.

**D15 — the tuning keys join KD-27's optional-with-default class.** `internal/config` gains an
`Ingest` struct and a `LAB_GAME_INGEST_` reader, mirroring `loadScheduler` exactly
[measured 1fce8b5:internal/config/scheduler.go:15-20 · `sed -n '15,20p' internal/config/scheduler.go`
→ the `LAB_GAME_SCHEDULER_POLL_INTERVAL` … `LAB_GAME_SCHEDULER_TASK_TIMEOUT` constants]: the keys are
appended by `EnvKeys()`, never by the unexported `envKeys()` the required-variable suites iterate.
The keys are the poll interval, the long-poll timeout, the batch limit, the retry attempt cap, the
retry base delay and the retry maximum delay. Their defaults are chosen operational tuning, never
balance numbers: poll interval `1s`, long-poll timeout `25s`, batch limit `100`, attempt cap `5`,
retry base delay `1s`, retry maximum delay `8s`. The retry values are picked together: under that
cap and that ramp the worst-case head-of-line stall a poisoned update imposes on the sequential loop
is `1s + 2s + 4s + 8s`, a quarter-minute, which is the term spec Scope 7 says the cap exists to
bound. The batch limit is validated against the
Bot API's stated range [measured 1fce8b5:telego@v1.11.2/methods.go:19-22 · `sed -n '19,22p'
$(go env GOMODCACHE)/github.com/mymmrac/telego@v1.11.2/methods.go` → `// Limit - Optional. Limits the
number of updates to be retrieved. Values between 1-100 are accepted.` above
`Limit int \`json:"limit,omitempty"\``]. `.env.example` gains a line
per key with its default as the value, because the disjointness test asserts set equality between
that file, `EnvKeys()` and the keys the loader consults (KD-23).

**D16 — the long-poll timeout is cross-checked against `AttemptTimeout` in `config.Load`.**
`Transport.AttemptTimeout` bounds every attempt including the long poll
[measured 1fce8b5:internal/tg/caller.go:170-176 · `sed -n '170,176p' internal/tg/caller.go` →
`attemptCtx, cancel = context.WithTimeout(ctx, to)` under `if to := c.client.transport.AttemptTimeout;
to > 0`], and it defaults to 30s [measured 1fce8b5:internal/config/transport.go:123-128 · `sed -n
'123,128p' internal/config/transport.go` → `AttemptTimeout: 30 * time.Second` among
`defaultTransport`'s fields]. A long-poll timeout at or above it turns every healthy poll into a
cancelled attempt. `Load` is the only place holding both structs, so the check lands there and
reports a `*KeyError` naming `LAB_GAME_INGEST_LONG_POLL_TIMEOUT` (AC27). The loop additionally
requires a whole number of seconds' worth of timeout, since the Bot API parameter is an integer count
of seconds, and transmits it explicitly so no poll degrades into short polling.

**D17 — the composition order the gate implies, stated once.** `Options.Gate` is set at `tg.New` and
never afterwards [measured 1fce8b5:internal/tg/client.go:28-31,127-135 · `sed -n '28,31p;127,135p'
internal/tg/client.go` → `Gate, when non-nil, is consulted before every outbound call` and
`c := &Client{… gate: opts.Gate, …}`], and the gate needs only the allowlist and a pool. So the
wiring order is gate, then client, then loop — no cycle. `cmd/bot` stays a scaffold this task does
not wire (spec § Out of scope); the order is recorded so the first mechanic's composition root does
not rediscover it.

### Rejected alternatives, at the level of the whole approach

- **A separate "seen updates" table.** Refused by §11 and by the spec's Key decisions; idempotency is
  the basis document's unique constraint or it is nothing.
- **Concurrent dispatch.** The MVP runs one friendly chat, per-chat ordering is free in a sequential
  loop, and the update-lag metric this task ships is the evidence that would justify changing it
  (spec § Deferred).
- **A per-update execution deadline modelled on the scheduler's.** The scheduler's hijack machinery
  exists to release a locked `scheduled_task` row; nothing here holds a row another worker waits on,
  and a second poller is unrepresentable (spec *Technical constraints* item 4).

## Decomposition

| # | Task | Files | Depends on |
|---|------|-------|------------|
| 1 | New shared package `internal/backoff`: `Exponential` and `EqualJitter`, package comment, exported doc comments, table + monotonicity tests | `internal/backoff/backoff.go`, `internal/backoff/backoff_test.go` | — |
| 2 | Adopt `internal/backoff` in the two shipped sites; delete both local ramps (clean break, no shim) | `internal/scheduler/cadence.go`, `internal/scheduler/settle.go`, `internal/tg/retry.go` | 1 |
| 3 | `config.Ingest` + the `LAB_GAME_INGEST_` reader; `EnvKeys()` append; the `LongPollTimeout` < `AttemptTimeout` cross-check in `Load`; `.env.example` lines | `internal/config/ingest.go`, `internal/config/config.go`, `internal/config/env.go`, `internal/config/ingest_test.go`, `.env.example` | — |
| 4 | Forward migration `00004_ingest.sql`: `ingest_offset` (guarded singleton, seeded) and `ingest_dead_update` | `internal/store/migrations/00004_ingest.sql` | — |
| 5 | `store.Queryer` and `store.PlayerExists` — the read-only owner lookup, with its tests | `internal/store/owner.go`, `internal/store/owner_test.go` | — |
| 6 | `internal/ingest` core types: package comment, `Kind` with D4's table and its date/chat extractors, `IDSpace` and the unexported `operation_id` builder, `Handler`/`Update`, `Route`/`Router`/`NewRouter`/`Kinds`, the package's sentinels | `internal/ingest/doc.go`, `internal/ingest/kind.go`, `internal/ingest/operation.go`, `internal/ingest/router.go`, `internal/ingest/errors.go`, plus their `_test.go` files | — |
| 7 | Offset and give-up storage: read/guarded-advance statements, `DeadUpdate`, `DeadUpdates` over a caller-owned `pgx.Tx` with a deterministic order | `internal/ingest/offset.go`, `internal/ingest/dead.go`, plus their `_test.go` files | 4, 6 |
| 8 | The observation seam: `Outcome`, `Observation`, `LoopObservation`, `Observer`, and the nil-checked report helpers | `internal/ingest/observe.go`, `internal/ingest/observe_test.go` | 6 |
| 9 | The loop: `Options`/`New`/`OptionError`, the requested-kinds set with D3's sentinel, `PollOnce`, `Run`, per-attempt transaction, `recover`, sentinel classification, bounded retry, settlement and offset advance, give-up row | `internal/ingest/loop.go`, `internal/ingest/loop_test.go`, `internal/ingest/retry_test.go` | 1, 3, 6, 7, 8 |
| 10 | The gate: `PlayerLookup`, the pool-backed implementation, `Gate`, `NewGate`, the positive-only cache | `internal/ingest/gate.go`, `internal/ingest/gate_test.go` | 5, 6 |
| 11 | Guard tests: no `panic(`/`log.Fatal`/`os.Exit` in the package's non-test source, no metrics-library import, the `telego.Update` field-vs-kind-table drift check, and the package's `TestMain` over `testdb.Main` | `internal/ingest/guards_test.go`, `internal/ingest/main_test.go` | 6, 9, 10 |
| 12 | Propagation: the layout paragraph and the code inventory in `ai-docs/context.md`; the allowlist and sanitisation bullets in `ai-docs/domain-invariants.md` (the player carve-out, the cache-invalidation obligation, the give-up table as a new sanitisation target); the new Key-Decision entries; the `INDEX.md` row | `ai-docs/context.md`, `ai-docs/domain-invariants.md`, `ai-docs/key-decisions.md`, `ai-docs/plans/INDEX.md` | 1–11 |

## Handoff plan

Grouping is required for every design with `M ≥ 1`, and this design's `M` is the count of rows in
§ Decomposition. Each group below is homogeneous by change-type — **code** (`*.go`, migrations, and
the code-gated `.env.example`) or **instructions/harness** (`*.md`, `.claude/**`, `AGENTS.md`,
`ai-docs/**`) — never both; the maximum group size is `10` consecutive subtasks and each group ends
at whichever comes first, the size cap, a change-type switch, or a dependency-forced boundary; the
terminal group's size stays within `1..=10`; the groups are minimised (the code subtasks are
clustered rather than interleaved with the documentation one); and the total stays at or under the
default maximum of `4` design-defined groups, so no user approval for an overflow is needed.

- **Handoff into Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § Compaction recovery (re-entry). The handoff binds at the start of **every** group, the first
  included.
- **Group A** — model `sonnet`, effort `medium` (pinned) via the `code-writer` subagent, 1M-token
  window — subtasks 1–6 (code change-type: `*.go`, `*.sql`, `.env.example`). The code subtasks as a
  block exceed the `10` cap, so they split into two same-model code groups; the boundary is placed
  where the dependency graph already puts a seam — everything through 6 is standalone or
  dependency-satisfied within the group, and 7 opens the database-backed half once 4 and 6 have
  landed.
- **Handoff after Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § Compaction recovery (re-entry). The parent `/task` resumes in Group B with fresh context.
- **Group B** — model `sonnet`, effort `medium` (pinned) via the `code-writer` subagent, 1M-token
  window — subtasks 7–11 (code change-type: `*.go`). Within the `≤ 10` cap.
- **Handoff after Group B:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § Compaction recovery (re-entry).
- **Group C** — model `inherit` (the orchestrator's), effort inherited from the orchestrator
  (typically xHigh) — **not** pinned — via the `general-purpose` subagent with no inline `model=`,
  1M-token window — subtask 12 (instructions/harness change-type: `ai-docs/**`). Terminal group;
  its size is within the `1..=10` range. The change-type switch at the Group B/C boundary is what
  forces this group even though Group B is under the cap.

## Risks

- **The `internal/tg` and `internal/scheduler` adoption of `internal/backoff` (D2) changes shipped,
  well-tested code that no AC of this task names.** Mitigation: both packages already own exact-table
  and monotonicity tests for their ramps, and the adoption must leave them passing unchanged; a
  failure there is a scope-boundary item for the orchestrator, not something the implementor absorbs
  — `[measured 1fce8b5:internal/scheduler/cadence_test.go:8,33; internal/tg/retry_test.go:127,682 ·
  `grep -n 'func Test.*[Bb]ackoff' internal/scheduler/cadence_test.go internal/tg/retry_test.go` →
  `TestRetry_JitterOptionThreadedThroughToBackoff`, `TestBackoffDelay_JitterBoundsExactly`,
  `TestBackoff_exactTable`, `TestBackoff_strictlyGrowingUntilCeiling`]`.
- **The sentinel kind of D3 is a claim about what this bot can receive, and only the shipped code can
  keep it true.** If a future mechanic ever sends an invoice with a flexible price, the sentinel
  becomes a live update kind. Mitigation: the sentinel is transmitted only while the route set is
  empty, so the first registered route removes it; the loop settles it as unrouted if it ever
  arrives — `[derived → AC5, AC9]`.
- **A first start on a database seeded at offset 0 asks the Bot API for the earliest unconfirmed
  update, not for "new updates from now".** `getUpdates` with no offset returns "updates starting
  with the earliest unconfirmed update"
  `[measured 1fce8b5:telego@v1.11.2/methods.go:11-18 · `sed -n '11,18p'
  $(go env GOMODCACHE)/github.com/mymmrac/telego@v1.11.2/methods.go` → `Offset - Optional. Identifier
  of the first update to be returned. … By default, updates starting with the earliest unconfirmed
  update are returned.`]`, so a first start can face whatever backlog the server still holds, drained
  one transaction at a time. Mitigation: the batch limit bounds one cycle, the loop is restart-safe
  because each update is settled with its own advance, and the give-up path bounds a poisoned
  update — `[derived → AC6, AC24, AC25]`.
- **A handler that ignores its context stalls the loop indefinitely** (D7 ships no per-update
  deadline). Mitigation: the contract states the obligation on the `Handler` doc comment, as
  `scheduler.Handler`'s already does; no production handler exists in this task to violate it, and
  the deadline is recorded in § Open questions rather than built speculatively —
  `[measured 1fce8b5:internal/scheduler/task.go:92-101 · `sed -n '92,101p'
  internal/scheduler/task.go` → `A Handler MUST propagate the ctx it is handed to every call it makes
  on tx`]`.
- **The gate's positive cache is unbounded for the process's lifetime and is never invalidated.**
  Nothing in this task deletes an owner row, so the cache cannot go stale in the wrong direction;
  a later mechanic that deletes or re-keys a player owner row must invalidate it. Mitigation: the
  obligation is written into `ai-docs/domain-invariants.md` in subtask 12, not only into a code
  comment — `[derived → AC35 and subtask 12]`.
- **Timing tests cannot run inside a `testing/synctest` bubble here.** KD-26 makes `synctest` this
  project's clock, but the loop's every path touches a real Postgres connection, which is not durably
  blockable inside a bubble. Mitigation: the retry-timing cases use small real durations from
  `config.Ingest`, exactly as `internal/scheduler`'s failure and deadline tests already do; the
  bubble is used only where a case touches no database — `[measured 1fce8b5:internal/scheduler/failure_test.go:150-154
  · `sed -n '150,154p' internal/scheduler/failure_test.go` → `want := backoff(attempt,
  cfg.RetryBaseDelay, cfg.RetryMaxDelay)` inside a real-duration assertion]`.
- **`golangci-lint`'s `exhaustive` is enabled, so every switch on `Kind`, `Outcome` or `IDSpace` must
  be total.** `default-signifies-exhaustive: true` is configured, so a `default` clause satisfies it.
  Mitigation: the design's derivations are table lookups, not switches, and the one place a switch is
  natural — rendering an `Outcome`'s name — carries a `default` —
  `[measured 1fce8b5:.golangci.yml:40-41 · `grep -n -A2 'exhaustive:' .golangci.yml` →
  `40:    exhaustive:` / `41:      default-signifies-exhaustive: true`]`.
- **The build and lint gates truncate, so any "N sites" count taken from a first run is a floor**
  `[measured 1fce8b5:.claude/agents/design-writer.md:129 · `grep -n 'too many errors'
  .claude/agents/design-writer.md` → `Measured on this toolchain: `go build ./...` prints at most
  **10 errors per package** … `golangci-lint run` defaults to `--max-issues-per-linter 50` and
  `--max-same-issues 3``]`. Mitigation: subtask 2's adoption re-runs
  `make verify` after the enumerated sites clear, and a newly revealed out-of-contract class is
  surfaced to the orchestrator rather than absorbed — `[derived → AC30]`.
- **The `.env.example` set-equality test fails by name in both directions**, so an ingest key added
  to the loader without a matching line (or the reverse) reds the config package. Mitigation: subtask
  3 owns both edits in one step — `[measured 1fce8b5:internal/config/disjoint_test.go:12-17 ·
  `sed -n '12,17p' internal/config/disjoint_test.go` → `This is AC16's "disjoint in fact, not only in
  prose": the recorded set is what the loader actually consulted, not what a reader believes it
  consults.`]`.

## Test Design

Everything below is about artefacts this task creates; the tags are `[derived → …]` throughout.
`internal/ingest`'s database-backed cases take a fresh schema from `testdb.Schema(t)` and apply
`store.Migrate`, and its Bot API cases point `tg.New` at a `tgtest.Server` through
`Options.HTTPClient`, `tgtest.BaseURL` and `tgtest.Token` — no mock of this package's own making
anywhere `[derived → AC29]`.

**Subtask 1 — `internal/backoff`** (`internal/backoff/backoff_test.go`)
- Entry points: `Exponential`, `EqualJitter`.
- Scenarios: an exact table of `(attempt, base, ceiling) → delay` including the zeroth attempt, the
  attempt at which the ceiling is first reached, and every attempt past it; strict positivity and
  non-decrease across a run of attempts; the ceiling never exceeded; a negative attempt clamped to
  the zeroth; `EqualJitter` bounded within `[d/2, d)` for a jitter stub returning 0 and one returning
  a value just under 1, and its monotonicity for a fixed jitter `[derived → AC23]`.
- Fixtures: a deterministic `jitter func() float64` stub. No database, no bubble.

**Subtask 2 — the adoption** (no new test file)
- The gate is that `internal/scheduler`'s and `internal/tg`'s existing backoff tests pass unchanged
  after the local ramps are deleted `[derived → AC30]`.

**Subtask 3 — `config.Ingest`** (`internal/config/ingest_test.go`)
- Entry points: `Load`, and the package's existing disjointness and required-variable suites.
- Scenarios: every ingest key absent takes its documented default; each key present and valid is
  parsed; each key present and malformed produces a `*KeyError` naming that variable and wrapping
  `ErrInvalidValue`; a batch limit outside the Bot API's accepted range is rejected; a long-poll
  timeout at or above `AttemptTimeout` is rejected naming the long-poll variable, and one below it is
  accepted; the recorded-lookup disjointness suite and the `.env.example` set-equality suite still
  pass with the new keys `[derived → AC27]`.
- Fixtures: the package's existing map-backed `Lookup`; no process environment is touched.

**Subtask 4 — the migration** (`internal/store/migrate_test.go`, `internal/store/schema_test.go`)
- Entry point: `store.Migrate` against a fresh schema.
- Scenarios: applying every migration creates both tables; a second `Migrate` is a no-op; the offset
  row exists exactly once after migration and a second row is refused by the singleton `CHECK`; a
  give-up row with an empty kind is refused; the migration file carries no `-- +goose Down` section
  and the shipped migration files' contents are unchanged `[derived → AC7]`.
- Fixtures: `testdb.Schema(t)`, `slog.New(slog.DiscardHandler)`.

**Subtask 5 — `store.PlayerExists`** (`internal/store/owner_test.go`)
- Entry point: `PlayerExists(ctx, q, telegramID)`.
- Scenarios: a `kind = 'player'` owner with that `telegram_id` reports true; a `kind = 'chat'` owner
  with the *same* `telegram_id` reports false — the pair, not the column, is the predicate; an id
  with no owner row reports false; a closed pool surfaces the error rather than a false; the same
  call through a `pgx.Tx` and through a `*pgxpool.Pool` behaves identically `[derived → AC28, AC33]`.
- Fixtures: `testdb.Schema(t)`, `store.CreateOwner` for both kinds.

**Subtask 6 — kinds, the grammar, the router** (`internal/ingest/kind_test.go`,
`operation_test.go`, `router_test.go`)
- Entry points: the kind derivation, the unexported `operation_id` builder, `NewRouter`, `Kinds`,
  and the router's lookup.
- Scenarios (kind): a table case per row of D4's table, each built by setting only that payload field
  on a `telego.Update` and asserting the derived kind, its date extraction (present or absent) and
  its chat-id extraction (present or absent); an update with no payload field set derives the zero
  kind `[derived → AC4, AC18]`.
- Scenarios (grammar): the `update_id` space and the `callback_query.id` space produce different
  values from the *same* raw identifier; a
  derived value always carries a non-empty space token, the separator and a non-empty identifier; an
  empty space or an empty identifier is refused; no space token contains the separator
  `[derived → AC13, AC14]`.
- Scenarios (router): registering a duplicate kind is refused; registering a kind with no table row
  is refused; `Kinds` returns the registered set in a deterministic order; a lookup for an
  unregistered kind reports absence `[derived → AC4]`.
- Fixtures: hand-built `telego.Update` values; no database.

**Subtask 7 — offset and give-up storage** (`internal/ingest/offset_test.go`, `dead_test.go`)
- Entry points: the offset read, the guarded advance, `DeadUpdates`.
- Scenarios: a fresh schema reads the seeded offset; an advance to a higher value is stored and read
  back by a *new* reader (no in-process value is the sole record); an advance to a value at or below
  the stored one is a no-op; a rolled-back advance leaves the stored value untouched; `DeadUpdates`
  returns rows in a deterministic order under a caller-owned transaction, honours its limit, and
  neither commits nor rolls back `[derived → AC6, AC25, AC37]`.
- Fixtures: `testdb.Schema(t)`, `store.Migrate`.

**Subtask 8 — the observation seam** (`internal/ingest/observe_test.go`)
- Entry points: the nil-checked report helpers and each observation type's rendering.
- Scenarios: a nil observer is not called; each `Outcome` renders a distinct name and an
  out-of-range value renders the fallback; a lag-absent observation is distinguishable from a
  zero-lag one `[derived → AC18, AC19]`.
- Fixtures: a recording observer collecting observations under a mutex, reused by subtasks 9 and 10.

**Subtask 9 — the loop** (`internal/ingest/loop_test.go`, `retry_test.go`)
- Entry points: `New`, `PollOnce`, `Run`.
- Scenarios:
  - *Request shape.* Every `getUpdates` the loop issues carries an `allowed_updates` field, with an
    empty router and with a populated one; every registered kind appears in it; the offset and the
    long-poll timeout are transmitted. Asserted by decoding the request body the `tgtest` handler
    receives `[derived → AC3, AC4, AC5]`.
  - *Happy path.* A routed update reaches its handler with the loop's transaction and the derived
    operation key; the handler's writes survive; the offset advances past it; the observer sees a
    handled outcome with a duration `[derived → AC8, AC17]`.
  - *Unrouted.* An update whose kind has no route leaves the database unchanged, is reported as
    unrouted, advances the offset, and does not stop the loop `[derived → AC9]`.
  - *Duplicate.* A handler that posts under a `store.PlayerOperation` twice for the same update
    produces exactly one `player_operation`, one `journal_entry` and one `posting` row; the second
    delivery is reported as an idempotency hit, consumes no retry attempt, and advances the offset —
    including when the handler wraps the sentinel with `%w` `[derived → AC12, AC22]`.
  - *Failure and retry.* A handler failing every attempt is retried in a transaction of its own each
    time, with a strictly positive and non-shrinking delay, up to the configured cap; after each
    failed attempt the database holds nothing that attempt wrote; after the cap the loop writes
    exactly one give-up row carrying the identity, kind, chat id, attempt count and last error, keeps
    no raw payload anywhere, advances the offset and continues polling `[derived → AC23, AC24,
    AC36]`.
  - *Panic.* A handler that panics does not terminate the test process, leaves no row behind, is
    reported distinctly from a returned error, and is retried under the same cap `[derived → AC10]`.
  - *Cancellation.* Cancelling the loop's context mid-retry leaves the update unsettled with the
    offset behind it; a long poll in flight when the context is cancelled returns promptly and `Run`
    returns without a goroutine surviving the test `[derived → AC25, AC26]`.
  - *Option validation.* A nil client, a nil pool, a nil router or a non-positive tuning field is
    refused by `New` with an error naming the field `[derived → AC27]`.
- Fixtures: `tgtest.Server` scripting `getUpdates` responses (a helper marshalling a `[]telego.Update`
  into `tgtest.Success`), a handler stub whose behaviour per call is table-driven (succeed, fail,
  panic, post twice), the recording observer of subtask 8, `testdb.Schema(t)` with `store.Migrate`,
  and a `config.Ingest` with millisecond-scale retry delays. No `synctest` bubble in the
  database-backed cases (see § Risks).

**Subtask 10 — the gate** (`internal/ingest/gate_test.go`)
- Entry points: `NewGate`, `Gate.AllowCall`, and the same gate installed into a real `tg.Client`.
- Scenarios: `ChatNone` allowed; `ChatUnknown` refused; a `ChatKnown` id inside the allowlist
  allowed; one outside it allowed when a `kind = 'player'` owner row carries that `telegram_id`;
  one outside it refused when only a `kind = 'chat'` owner row carries it; a token that parses as no
  integer refused; a lookup that errors refuses; a destination refused for want of a player row is
  allowed once the row exists, within the same gate instance and with no restart; a positive result
  is served from the cache without a second lookup (counted through a lookup stub)
  `[derived → AC15, AC28, AC33, AC34, AC35]`.
- Integration scenario: with the gate installed at `tg.New`, a refused outbound call reaches the
  `tgtest` server not at all and is reported through `tg.Observation` with no attempt recorded, and a
  subsequent allowed call into the same chat and method class is not delayed by a limiter window the
  refusal would have spent `[derived → AC16, AC38]`.
- Fixtures: a `PlayerLookup` stub (fixed answers, an error mode, and a call counter), the pool-backed
  implementation over `testdb.Schema(t)`, `tgtest.Server`, `config.Transport` defaults.

**Subtask 11 — guards** (`internal/ingest/guards_test.go`, `main_test.go`)
- Entry points: a source walk over the package's non-test files, and a reflection walk over
  `telego.Update`.
- Scenarios: no `panic(`, `log.Fatal` or `os.Exit` appears in the package's non-test source; no
  metrics library is imported; every exported pointer field of `telego.Update` has its json tag
  either in D4's kind table or in a named exemption set, and every table row's token is a tag some
  field declares; the event-type dictionary migration and its Go mirror are unchanged by this task;
  `internal/store`'s posting entry points are unchanged `[derived → AC11, AC19, AC20, AC21]`.
- Additional scenario, because no enabled linter covers it: no type declared in `internal/ingest` has
  a `context.Context` field, and every exported method of the package that reaches the network or the
  database takes `ctx context.Context` first — asserted by an `go/ast` walk over the package's
  non-test files, since `containedctx` is not among the enabled linters
  [measured 1fce8b5:.golangci.yml:12-37 · `grep -n 'enable:\|containedctx\|bodyclose\|whitespace'
  .golangci.yml` → `12:  enable:`, `14:    - bodyclose`, `37:    - whitespace`, and no
  `containedctx` line] `[derived → AC2]`.
- Fixtures: the repository-root resolver `internal/tg/guards_test.go` already establishes for this
  shape; `testdb.Main` in `TestMain`.

**Cross-cutting — the gates, not a test file**
- AC1's doc-comment and package-comment obligations are held by `revive`'s `exported` and
  `package-comments` rules, which `make verify` runs
  [measured 1fce8b5:.golangci.yml:45-48 · `grep -n 'revive\|- name: exported\|- name:
  package-comments' .golangci.yml` → `35:    - revive             # incl. exported-symbol doc
  comments`, `45:    revive:`, `47:        - name: exported`, `48:        - name:
  package-comments`] `[derived → AC1]`.
- AC30 is `make verify` in full, and AC31 is the coverage ratchet the pre-commit hook and CI both
  run against the recorded value; new statements this task adds are covered by the suites above
  `[derived → AC30, AC31]`.
- AC32 is subtask 12's whole job, and the sweep is `AGENTS.md` § Propagation Rule step 4's — a
  case-insensitive `grep -rni` over `.claude/`, `AGENTS.md` and `ai-docs/` for every claim this diff
  falsifies, plus the repo-root user-facing docs, not only the members the spec names
  `[derived → AC32]`.

## Open questions

- **Does the `internal/tg` half of D2's adoption stay in scope?** The design includes it because
  leaving it out preserves a third copy of the same ramp beside the new shared package, which
  `design-writer` § Rules forbids. It is nonetheless a change to a shipped transport that no
  acceptance criterion of this task names, and the orchestrator may prefer to split it into its own
  issue. If it is split, `internal/backoff` still ships and `internal/scheduler` still adopts it.
- **A per-update execution deadline.** D7 ships none. A handler that ignores its context stalls the
  sequential loop with no bound; the scheduler solved the same problem with a transaction-local
  `statement_timeout` and a hijackable connection. Nothing in this task's ACs requires it and no
  production handler exists yet, so it is recorded rather than built.
- **Whether `cmd/bot` becomes a running process here.** The spec says no, on #19's precedent; this
  design follows the spec and records the composition order (D17) instead. Changing that is a scope
  change, not a design amendment.
- **The offset's granularity.** D14 takes the MVP's one-bot-per-database singleton and states the
  widening as a later forward migration. If the owner expects a token change before the first
  mechanic ships, the column is cheaper to add now than later.
