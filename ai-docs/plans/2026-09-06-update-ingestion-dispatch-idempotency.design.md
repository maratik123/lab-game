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
re-expresses **both** existing sites in terms of it. The call sites after this change are
`internal/backoff` (definition), `internal/tg`, `internal/scheduler` and `internal/ingest`, and #43's
outbound queue is the "more to come" trajectory the rule names.

**`Exponential` clamps at `ceiling` *before* the doubling that would overflow, and that is part of
the exported contract rather than an implementation detail the tests happen to miss.** Both ramps
being replaced are overflow-proof by construction, and by construction in the same way: each tests
the ceiling *before* doubling and leaves the loop there — `internal/tg`'s at
[measured c221784:internal/tg/retry.go:33-34 · `sed -n '33,34p' internal/tg/retry.go` →
`if d > maxDelay/2 {` / `d = maxDelay`], `internal/scheduler`'s at
[measured c221784:internal/scheduler/cadence.go:46-47 · `sed -n '46,47p'
internal/scheduler/cadence.go` → `if d >= ceiling {` / `return ceiling`]. A shared implementation
written as `base << attempt`, or as a loop whose ceiling test comes *after* the doubling, is
therefore a regression rather than a translation: `time.Duration` is an `int64` of nanoseconds, and a
doubling past its range wraps to a negative value. That attempt argument is operator-reachable, not
theoretical — the attempt caps are parsed by `lookupPositiveInt`, which bounds a value from below
only [measured c221784:internal/config/transport.go:207-217 · `sed -n '207,217p'
internal/config/transport.go` → `func lookupPositiveInt(lookup Lookup, key string) (int, bool,
error)` whose only rejection is `if err != nil || n <= 0`], and `internal/scheduler` feeds the row's
`consecutive_failures` straight into the ramp at the persisted-`run_at` computations
[measured c221784:internal/scheduler/settle.go:142,242 · `grep -n 'backoff(k,'
internal/scheduler/settle.go` → `142:		runAt := s.Add(backoff(k, cfg.RetryBaseDelay,
cfg.RetryMaxDelay))` and the same expression at `242`]. A negative delay there persists a `run_at` in
the past: a hot-looping dead task, which would be a data-visible regression inside a change this
design bills as behaviour-preserving. `Exponential`'s stated contract is therefore, for **every**
`attempt` including arbitrarily large ones: the result is strictly positive, never exceeds `ceiling`,
and never decreases as `attempt` grows. Subtask 1's table pins it at attempts far past any a naive
shift survives.

**The contract quantifies over `base` too, because one adopter already depends on that half.**
`base > ceiling` is not a degenerate case a shared implementation may leave undefined here: a shipped
assertion in `internal/tg` pins exactly it, at the zeroth attempt, through the post-loop clamp
[measured df9b1a2:internal/tg/retry_test.go:724-725 · `sed -n '724,725p' internal/tg/retry_test.go` →
`if got := backoffDelay(40*time.Second, shippedMax, 0, func() float64 { return 0 }); got !=
shippedMax/2 {` with the failure message `post-loop clamp: base alone already exceeds maxDelay`], and
`internal/scheduler`'s ramp answers the same input at its own post-loop clamp
[measured df9b1a2:internal/scheduler/cadence.go:51-53 · `sed -n '51,53p'
internal/scheduler/cadence.go` → `if d > ceiling {` / `return ceiling`]. An `Exponential` that clamps
only *inside* the doubling loop returns `base` for `attempt = 0` and breaks that assertion the moment
`internal/tg` re-points at it. So the contract reads: for every `attempt` **and every `base`**,
including a `base` already above `ceiling`, the result is strictly positive, never exceeds `ceiling`,
and never decreases as `attempt` grows — and subtask 1's table carries a `base > ceiling` row of its
own, so `internal/backoff`'s table covers the adopters' full input domain instead of leaning on
subtask 2 to catch a hole in it.

**Scope — settled by the owner, round 2.** `internal/backoff` ships **and** both `internal/tg` and
`internal/scheduler` adopt it. Adopting it in only one would leave the shared package standing
beside a surviving copy of the same ramp one directory away, which is the outcome
`design-writer` § Rules exists to prevent. This is no longer an open question.

**Per package, the decision is *delete and re-point*, never *keep a thin adapter*.** `AGENTS.md`
§ API Stability's table names a delegating wrapper as the thing to delete
[measured febae63:AGENTS.md:108 · `grep -n 'Keep \`func OldName' AGENTS.md` → `108:> | Keep \`func
OldName(...)\` delegating to \`NewName\` "for compat" | **DELETE** it — call sites update
directly |`], and an unexported one is
not exempt: there is no downstream client forcing either shape, so an adapter here would exist
solely so that call sites need not change. In `internal/scheduler` the adapter is not merely
disfavoured but unavailable under its present name — Go refuses a package-scope identifier that an
import of the same name also declares, and the refusal is package-wide rather than per-file
[measured febae63 · a probe module declaring `func sub()` in `a.go` and importing
`probe2/sub` in `b.go`, `go build ./...` → `./a.go:3:6: sub already declared through import of
package sub ("probe2/sub")` / `./b.go:3:8: other declaration of sub`], so keeping `func backoff`
would force either a rename or an import alias — cost with no benefit.

| package | deleted | re-pointed to |
|---|---|---|
| `internal/tg` | `backoffDelay` | `backoff.EqualJitter` (attempt argument first), at the production call site in `internal/tg/caller.go` and at the direct calls in `internal/tg/retry_test.go` [measured febae63 · `rg -U --type go -n 'backoffDelay\s*\(' internal/tg/` → the declaration in `internal/tg/retry.go`, call sites in `internal/tg/caller.go` and `internal/tg/retry_test.go`, plus failure-message mentions in the latter; no other file]. `defaultJitter` and the jitter-bounds contract stay in `internal/tg`. |
| `internal/scheduler` | `backoff` | `backoff.Exponential`, at the persisted-`run_at` computations in `internal/scheduler/settle.go` and at the direct calls in `internal/scheduler/cadence_test.go`, `internal/scheduler/failure_test.go` and `internal/scheduler/deadline_test.go` [measured febae63 · `rg -U --pcre2 --type go -n '(?<![A-Za-z_.])backoff\s*\(' internal/scheduler/` → the declaration in `internal/scheduler/cadence.go`, calls in `internal/scheduler/settle.go`, `internal/scheduler/cadence_test.go`, `internal/scheduler/failure_test.go` and `internal/scheduler/deadline_test.go`, plus comment and failure-message mentions in the last three; no other file]. The one-based→zero-based translation is written at the call site, where the `consecutive_failures` value it converts is visible [measured febae63:internal/scheduler/settle.go:142 · `sed -n '142p' internal/scheduler/settle.go` → `runAt := s.Add(backoff(k, cfg.RetryBaseDelay, cfg.RetryMaxDelay))`, with `k` the row's `consecutive_failures`]. |

The scan above is multiline-aware (`rg -U`) per `design-writer` § Rules, and it reaches three
classes the implementor must not conflate: **call expressions**, which re-point; **comment prose**
naming `backoff(k)`, which is re-worded to name the shared function; and **`t.Errorf` format
strings**, which are failure-message text, not assertions — re-wording them is free, and doing so
does not touch the expected values the gate below fixes.

**What "behaviour-preserving" can be gated on.** A delete-and-re-point does not leave the shipped
ramp tests *compiling* unchanged, so "they pass unchanged" is not an available gate and must not be
written as one. The first half of the gate is narrower and actually checkable: in
`internal/tg/retry_test.go`, `internal/scheduler/cadence_test.go`,
`internal/scheduler/failure_test.go` and `internal/scheduler/deadline_test.go`, **every
*pre-existing* assertion and every *pre-existing* expected value stays byte-identical; only the call
expression is re-pointed.** A changed pre-existing expected value in any of them is the signal that
the adoption was not behaviour-preserving, and is a scope-boundary item for the orchestrator rather
than something the implementor absorbs. Those tests exist today and are what the re-pointing
preserves [measured 1fce8b5:internal/scheduler/cadence_test.go:8,33 and
internal/tg/retry_test.go:127,682 · `grep -n 'func Test.*[Bb]ackoff'
internal/scheduler/cadence_test.go internal/tg/retry_test.go` →
`TestRetry_JitterOptionThreadedThroughToBackoff`, `TestBackoffDelay_JitterBoundsExactly`,
`TestBackoff_exactTable`, `TestBackoff_strictlyGrowingUntilCeiling`].

**That half is necessary and NOT sufficient, and the hole sits exactly where the translation is.**
`internal/scheduler`'s persisted-`run_at` sites are the ones needing the one-based→zero-based
translation, and the shipped assertions over a persisted retry `run_at` compute their expected value
through **the same function the call site uses**
[measured df9b1a2:internal/scheduler/failure_test.go:154 and internal/scheduler/deadline_test.go:252
· `rg -n --pcre2 '(?<![A-Za-z_.])backoff\s*\(' internal/scheduler/*_test.go` → `want :=
backoff(attempt, cfg.RetryBaseDelay, cfg.RetryMaxDelay)` at each, the test's `attempt` asserted equal
to the row's `consecutive_failures` on the preceding lines; every other test hit is `cadence_test.go`
pinning the function itself, or comment and failure-message prose]. So an implementor applying the
*natural* mechanical re-point — `backoff(x, …)` → `backoff.Exponential(x, …)` at production and test
sites alike — omits the translation **consistently**: every expected value stays byte-identical,
`cadence_test.go`'s exact table still passes (it pins `Exponential`, not the call site),
`internal/backoff`'s own table still passes, and `make verify` is green — while every scheduler
retry's persisted `run_at` doubles and the give-up cadence moves with it. That is the data-visible
regression the paragraphs above name, and **no shipped suite can see it**. `internal/tg` is not
exposed: its expected values are literals and its adoption needs no translation — which is why the
hole is invisible from the adopter that does have a table, and why "the shared package has its own
table" is not an answer to it.

**So the second half of the gate pins the CALL SITE, independent of the shared function.** In
`internal/scheduler/failure_test.go`, which drives the one-shot settlement
[measured df9b1a2:internal/scheduler/settle.go:242 · `sed -n '242p' internal/scheduler/settle.go` →
`runAt := s.Add(backoff(k, cfg.RetryBaseDelay, cfg.RetryMaxDelay))`], and in
`internal/scheduler/deadline_test.go`, which drives the drain settlement at the identical expression
[measured df9b1a2:internal/scheduler/settle.go:142 · `sed -n '142p' internal/scheduler/settle.go` →
the same `s.Add(backoff(k, cfg.RetryBaseDelay, cfg.RetryMaxDelay))`], the persisted `run_at` bracket
is computed from a **literal one-based ramp written out in the test** — an indexed table of
durations keyed by the loop's `attempt`, with no call to `backoff`, to `backoff.Exponential`, or to
any shift or doubling expression inside it. Both tests already iterate attempts that sit strictly
below their ceiling under a `200ms` base
[measured df9b1a2:internal/scheduler/failure_test.go:83 and internal/scheduler/deadline_test.go:201 ·
`sed -n '83p' internal/scheduler/failure_test.go; sed -n '201p' internal/scheduler/deadline_test.go`
→ `cfg.RetryBaseDelay = 200 * time.Millisecond` in each probe config, against the `1s` and `500ms`
ceilings their base configs set], so the one-based ramp (`200ms`, `400ms`) and the zero-based one
(`400ms`, `800ms`-clamped-to-`500ms`) disagree at **every** attempt either test asserts: the omitted
translation reds both, by a readable factor rather than by a flake.

**That substitution is the sole carve-out from the byte-identical rule, and in substance it is not an
exception to it.** The literal ramp reproduces the *same durations* the shipped
`backoff(attempt, …)` computes for those attempts — which is exactly what step (a)'s green run
against the still-shipped ramp proves before anything is re-pointed. What changes is where the
expected value comes from, never what it is; and after step (a) the literal ramp is itself a
pre-existing expected value that step (c)'s re-point must leave byte-identical. Nowhere else in
these files does new assertion text land.

**Ordering is part of the deliverable, and a mutation probe is what makes the instrument
trustworthy.** The literal ramp lands **first**, against the still-shipped one-based `backoff`, and
is green there; only then is the re-point applied. And because a green instrument is a claim about
the instrument until it has been seen to go red (`AGENTS.md` § Patterns 2), the implementor confirms
it discriminates before trusting it: with the literal ramp in place, apply the *wrong* translation
(`Exponential(k, …)` at both `settle.go` sites), run
`go test ./internal/scheduler/ -run 'TestFailurePolicy|TestDeadline'`, observe **RED**, then restore.
The restore is a `cp` backup of `settle.go` taken before the probe and copied back — never
`git checkout -- <file>`, which `AGENTS.md` § Workflow names as the command that silently discards
every other uncommitted edit to the file. A probe that comes back **green** means the gate is not the
gate this design asked for: that is a STOP for the orchestrator, not something to reason past.

`internal/backoff` carries its own exact table besides, in its own zero-based units, so the shared
function's contract is pinned independently of either adopter — but that table says nothing about
*which argument a call site passes*, which is the whole reason the literal ramp exists.

*Rejected:* a fourth private copy in `internal/ingest` (the rule's named anti-pattern); a
third-party backoff module (the stdlib plus this arithmetic is the whole requirement — `AGENTS.md`
§ Dependency Versions asks the comparison and there is nothing here a package would carry); a thin
unexported adapter in either package (it would shrink the diff and make the gate literal, at the
price of the shim `AGENTS.md` § API Stability tells you to delete — and the translation it would
hide, one-based `failures` against a zero-based attempt, is exactly what a reader of `settle.go`
should be able to see).

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
invoices [measured febae63 · `grep -rn 'invoice\|shipping' --include=*.go internal/ cmd/` → no
match (exit 1)]. The sentinel is present **only** when the route set is empty, so a kind with no route
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
backwards. The value it writes is the settled update's `update_id + 1`, because that is what the
column holds and what the next poll transmits verbatim (D14). AC6, AC24, AC25.

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

**The cache's shape is a mutex-guarded `map[int64]struct{}`, not a bare map.** `AllowCall` sits on
the outbound path, which is concurrent by construction — the per-chat and class-global limiters
downstream of it exist precisely because concurrent senders do, and they hold their own state under
a single mutex [measured febae63:internal/tg/limit.go:202-207 · `sed -n '202,207p'
internal/tg/limit.go` → `type Limiter struct {` / `mu     sync.Mutex` / `global [3]*schedule` /
`chats  map[chatKey]*schedule`]. An unsynchronised map on that seam is a `concurrent map writes`
fatal error on the one component whose job is to stop a write to an unintended chat, and it is a
race `go test -race` is a required gate for (`AGENTS.md` § Go Test Conventions). The critical
section is a map lookup and, on a miss that the lookup confirms, one insert; the database call
itself happens **outside** the lock, so a slow lookup never serialises other destinations, at the
cost of a benign duplicate lookup on a first concurrent miss for the same id (the value written is
the same either way). *Rejected:* `sync.Map` — its wins are stable read-mostly keys under heavy
core-count contention, which this seam does not have, and it would trade the shipped limiter's
already-reviewed mutex idiom for a second concurrency vocabulary in the same call path.

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
real schema without either being a mock of this package's own making (AC29). **What that pool handle
can and cannot see is D18's subject**, and the answer there is a designed property of the handler
contract rather than a limitation of this decision.

**`Queryer` has a shipped near-duplicate inside `internal/store`, and subtask 5 re-points it rather
than adding a second spelling.** The package's own test file already declares an unexported
interface of exactly this shape for exactly this reason
[measured c221784:internal/store/post_test.go:72-75 · `sed -n '72,75p' internal/store/post_test.go` →
`// queryRower is satisfied by both pgx.Tx and *pgxpool.Pool.` / `type queryRower interface {` /
`QueryRow(ctx context.Context, sql string, args ...any) pgx.Row`], and its consumer is the
`balanceOf` helper in the same file. Exporting `Queryer` beside it and leaving `queryRower` standing
would put two names for one method set in one package — the shape `design-writer` § Rules refuses at
the package level and there is no reason to accept at the file level. Subtask 5 deletes `queryRower`
and re-points `balanceOf` at `store.Queryer`; `AGENTS.md` § API Stability's delete-the-wrapper rule
applies unchanged to an unexported test-local one.

**Why two interfaces over one read, stated so a later reader does not collapse them.** They narrow
at two different seams and each is declared by its own consumer, which is this project's rule for
interfaces (`AGENTS.md` § API Naming). `store.Queryer` narrows what `PlayerExists` needs from its
*handle* — one `QueryRow` method — so the same read serves a `pgx.Tx` on a handler's path and a
`*pgxpool.Pool` on the gate's without `internal/store` naming a pool type in a signature.
`ingest.PlayerLookup` narrows what the *gate* needs from the read — one boolean answer — which is
what lets the cache scenarios substitute a stub carrying a call counter and assert that a second
`AllowCall` for a cached id issues no second lookup (AC35). Collapsing them costs one of those
two properties: giving the gate a `Queryer` puts a fake of `internal/store`'s row-scanning
semantics inside `internal/ingest` (a mock of another package's read, and no place to count calls),
while giving `internal/store` a `PlayerLookup` puts the gate's caching vocabulary into the table's
owning package.

**D12 — the observation is one per attempt plus one per attempt-less settlement.** §13.2 wants
handler duration, handler errors *and* panics, and idempotency hits on the health surface
(`docs/DESIGN.md` §13.2).
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
pinned by `CHECK (id = 1)` and seeded by the migration itself, and **its column holds the offset to
transmit, not the last update seen — the name says which.** `GetUpdatesParams.Offset` is defined as
one greater than the highest `update_id` already received
[measured df9b1a2:telego@v1.11.2/methods.go:11-18 · `sed -n '11,18p'
$(go env GOMODCACHE)/github.com/mymmrac/telego@v1.11.2/methods.go` → `Offset - Optional. Identifier
of the first update to be returned. Must be greater by one than the highest among the identifiers of
previously received updates. … An update is considered confirmed as soon as getUpdates … is called
with an offset higher than its update_id.`], so the column is `next_update_id`: settling
`update_id = n` writes `n + 1`, and the loop transmits the stored value **verbatim** as
`GetUpdatesParams.Offset`, with the `+ 1` living in exactly one statement — the advance — and nowhere
on the poll path. The alternative shape, storing `n` and adding one at transmission, is **refused**,
and not on taste: combined with D5's guarded-monotone advance it is a permanent **live-lock**. The
same update is re-fetched every cycle, its handler's `Post` returns `ErrAlreadyPosted`, the loop
settles it as a duplicate and writes `n` again, the guard rejects the write because `n` does not
exceed `n`, and the offset never moves — and the only symptom is the idempotency-hit counter §13.2
puts on the health surface, where duplicates are ordinary traffic and only a *spike* reads as a
signal (`docs/DESIGN.md` §13.2), and which nothing exposes at all until #23 lands (spec § Out of
scope). One semantic, named in the column, pinned by subtask 9's transmitted-offset scenario. The seeded value is
`0`, which the Bot API reads as "the earliest unconfirmed update" — the first-start behaviour
§ Risks already states. `ingest_dead_update` carries the
update's identity, its kind, a nullable chat id, the attempt count and the last error — the shape
`scheduler.DeadTask` already has [measured 1fce8b5:internal/scheduler/task.go:78-90 · `sed -n
'78,90p' internal/scheduler/task.go` → `DeadTask is one give-up row … Type / InstanceKey / RunAt /
ConsecutiveFailures / LastError`] — and **not** the raw update payload. Table names are singular per
KD-17. **Forward-only, and its rollback:** both tables are created by this migration, so no row
predates it and no code reads a second shape during a deploy window; undoing it is a later forward
migration that drops them, at the cost of the offset (a fresh deploy then resumes from Telegram's
earliest unconfirmed update).

**Granularity — settled by the owner, round 2.** The offset row stays the singleton this decision
specifies. Widening it to a per-bot key is a later forward migration adding a column and relaxing
the `CHECK`; this task takes the MVP's one-bot-per-database shape rather than a speculative key.
This is no longer an open question.

**Both of `ingest_dead_update`'s identifying columns are §12.5 sanitisation targets — the chat id
*and* `last_error`.** The chat id column is the obvious one. `last_error` is free text produced by
whatever the handler returned, so it can embed a real chat or user id that a column-targeted rewrite
would walk straight past, and §12.5's whole obligation is that a restored snapshot cannot address a
real person. The only read this design gives the column is `DeadUpdates`' projection (AC37), which
carries it to an operator and branches on nothing in it, so the column is diagnostic data at rest
and sanitisation is free to **blank** it rather than rewrite ids inside it — cheaper and safer than
a regex over arbitrary error prose `[derived → AC36, AC37, and the `DeadUpdates` scenarios in
§ Test Design subtask 7]`. **`ingest_offset` is a §12.5 target too, and it is the one that bullet already names.** §12.5's
sanitisation transaction is required to «сбросить updates offset», and until this task there was no
table for that clause to refer to — after it, `ingest_offset` is that table by name, and its reset is
a `UPDATE ingest_offset SET …` on the singleton row rather than a rewrite of ids. So subtask 12
records **three** obligations against the same bullet, not one: reset `ingest_offset`, rewrite
`ingest_dead_update`'s chat id, blank `ingest_dead_update.last_error`. The design states this
so §12.5's script author does not have to infer it, and subtask 12 writes the same obligation into
`ai-docs/domain-invariants.md` beside the existing sanitisation bullet
[measured febae63:ai-docs/domain-invariants.md:115 · `sed -n '115p' ai-docs/domain-invariants.md` →
`A production snapshot reaches testing only through sanitisation — rewrite chat/user ids, drain the
outbound notification queue, reset the updates offset.`].

**D15 — the tuning keys join KD-27's optional-with-default class.** `internal/config` gains an
`Ingest` struct and a `LAB_GAME_INGEST_` reader, mirroring `loadScheduler` exactly
[measured 1fce8b5:internal/config/scheduler.go:15-20 · `sed -n '15,20p' internal/config/scheduler.go`
→ the `LAB_GAME_SCHEDULER_POLL_INTERVAL` … `LAB_GAME_SCHEDULER_TASK_TIMEOUT` constants]: the keys are
appended by `EnvKeys()`, never by the unexported `envKeys()` the required-variable suites iterate.
The keys are the poll interval, the long-poll timeout, the batch limit, the retry attempt cap, the
retry base delay and the retry maximum delay. Each key's *role* is fixed by a decision rather than
left to the implementor: the poll interval is the wait between cycles and, per D19, the whole bound
on the retry rate against a failing Bot API; the long-poll timeout is D16's; the retry three are
D6's, consumed through `backoff.Exponential`; the batch limit is `GetUpdatesParams.Limit`. Their defaults are chosen operational tuning, never
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
reports a `*KeyError` naming `LAB_GAME_INGEST_LONG_POLL_TIMEOUT` (AC27).

**The whole-seconds requirement is a second `config.Load` check on the same key, not a loop-side
truncation.** The Bot API parameter is an integer count of seconds
[measured c221784:telego@v1.11.2/methods.go:24-26 · `sed -n '24,26p'
$(go env GOMODCACHE)/github.com/mymmrac/telego@v1.11.2/methods.go` → `// Timeout - Optional. Timeout
in seconds for long polling. Defaults to 0, i.e. usual short polling. Should be positive …` above
`Timeout int \`json:"timeout,omitempty"\``], so a duration that is not a whole number of seconds
cannot be transmitted as configured. The loop must therefore never be handed one: a value such as
`25500ms` is rejected by `Load` with a `*KeyError` naming `LAB_GAME_INGEST_LONG_POLL_TIMEOUT`,
exactly as the `AttemptTimeout` cross-check on the same key does, and both checks live beside each
other. *Rejected:* letting the loop truncate to whole seconds — it would silently run a different
timeout than the operator configured, and it would loosen the margin the strict
`LongPollTimeout < AttemptTimeout` inequality was computed against, since the check would then be
comparing a value the loop does not use. One key, one owner, one error shape. The loop transmits the
timeout explicitly on every poll, so no poll degrades into short polling.

**The check is a strict inequality by design; no slack term is enforced, and the margin ships in the
defaults.** `LongPollTimeout < AttemptTimeout` admits a pair as tight as `25s` against `26s`,
leaving a second for the whole round trip — deliberately. The slack an operator actually needs is a
property of their network path to their `telegram-bot-api` instance, not a number this package can
pick, and a minimum enforced here would reject a legitimately tuned pair on a fast local hop while
still being too small on a slow one. What the design *does* commit to is that the shipped defaults
carry a real margin — `25s` against `AttemptTimeout`'s `30s` — and that both values are separately
tunable and both appear in `.env.example`, so the margin is visible rather than implicit. Only the
degenerate case, where every healthy poll is guaranteed to be cancelled, is a start-up error.

**D17 — the composition order the gate implies, stated once.** `Options.Gate` is set at `tg.New` and
never afterwards [measured 1fce8b5:internal/tg/client.go:28-31,127-135 · `sed -n '28,31p;127,135p'
internal/tg/client.go` → `Gate, when non-nil, is consulted before every outbound call` and
`c := &Client{… gate: opts.Gate, …}`], and the gate needs only the allowlist and a pool. So the
wiring order is gate, then client, then loop — no cycle. `cmd/bot` stays a scaffold this task does
not wire (spec § Out of scope); the order is recorded so the first mechanic's composition root does
not rediscover it.

**D18 — an outbound call that only a row of the handler's own uncommitted transaction would permit is
not issued from inside the handler. The gate refusing it is correct behaviour, not a defect to work
around.** D11 gives the gate a `*pgxpool.Pool`, so `PlayerExists` runs on a connection of its own,
and a row the loop's still-open transaction has written is not visible on it. §1's onboarding is
exactly the shape that meets this — button in the chat → the player presses Start → the player's
`owner` row is created → the bot writes to the player's DM, whose chat id is in no
`ALLOWED_CHAT_IDS` list, since §1 also puts every game command in DM (`docs/DESIGN.md` §1; the spec
quotes both clauses at Scope 8). Written naively —
create the row, then DM before returning — the gate refuses that DM on the attempt and identically on
every retry (each retry is a fresh transaction, D6, so the row is no more visible), and the update
ends in `ingest_dead_update`. This design states the constraint rather than leaving #30 to discover
it. Nothing in the project overrides the transaction isolation level: the tree's one production
`Begin` takes no options [measured c221784:internal/scheduler/execute.go:65 · `sed -n '65p'
internal/scheduler/execute.go` → `tx, err := conn.Begin(ctx)`], so the visibility rule the gate lives
under is the server's default one, and subtask 10's uncommitted-owner-row scenario is what pins it
against a real Postgres `[derived → subtask 10's uncommitted-owner-row scenario]`.

**The rule belongs on the `Handler` contract, and it is not a gate workaround.** A handler MUST NOT
issue an outbound Bot API call whose permission rests on a row its own uncommitted transaction
created. The reason survives even if the gate were made transaction-aware: a Telegram send is not
rollback-able, so a message justified by a row the transaction then rolls back has already reached a
real person. That is the dual-write hazard the outbound notification queue exists to remove
(`docs/DESIGN.md` §13.2, which puts that queue on the health surface beside the update metrics this
task ships). **The designed route is that queue (#43)**: the handler enqueues a
row inside its own transaction, and the send happens after the commit, at which point `PlayerExists`
sees the `owner` row on any connection. #43 is out of this task's scope (spec § Out of scope), so
until it lands a mechanic needing a post-commit send performs it after the loop has committed,
outside the handler — and this design adds no speculative after-commit hook for a handler that does
not exist yet.

**Why not the obvious alternative — making the gate transaction-aware.** Giving the gate a way to
consult the in-flight transaction means smuggling a `pgx.Tx` into `tg.Gate.AllowCall`'s `ctx`, because `Call` carries only
the method, its class and its destination [measured c221784:internal/tg/gate.go:53-63 · `sed -n
'53,63p' internal/tg/gate.go` → `type Call struct {` with `Method string`, `Class MethodClass`,
`Chat ChatRef`]. That puts a transaction in a context value on the outbound seam, makes
`internal/tg`'s gate contract transaction-aware in a package the spec keeps free of the database
(spec Scope 8's fourth property), and buys a *worse* answer: a permission derived from a row that may
still roll back. Refusing is the direction the spec already fixed for this gate — it fails closed.

Three surfaces carry the obligation so it is met rather than rediscovered: the `Handler` doc comment
states it beside the ctx-propagation obligation `scheduler.Handler`'s already carries
[measured c221784:internal/scheduler/task.go:92-101 · `sed -n '92,101p' internal/scheduler/task.go` →
`A Handler MUST propagate the ctx it is handed to every call it makes on tx`] (subtask 6); subtask
10's Test Design pins the behaviour with a negative scenario; and subtask 12 writes it into
`ai-docs/domain-invariants.md` beside the allowlist bullets.

**D19 — `Run` observes a poll failure and keeps polling; only cancellation ends it.** A `getUpdates`
failure is ordinary traffic against a self-hosted `telegram-bot-api` instance — one 502 must not stop
all ingestion, and `Run` returning it would demand an external restart for a transient fault. So:
`PollOnce` returns the poll error to its caller **and** reports it through `LoopObservation.Err`
(D12); `Run` discards `PollOnce`'s return, having already observed it, and continues. `Run` returns
non-nil only on cancellation, and returns `ctx.Err()` (AC26). This is `internal/scheduler`'s shipped
worker shape, transposed without variation
[measured c221784:internal/scheduler/worker.go:145-161 · `sed -n '145,161p'
internal/scheduler/worker.go` → `ticker := time.NewTicker(w.cfg.PollInterval)` then a loop whose body
is a `ctx.Done()` check returning `ctx.Err()`, `_ = w.RunOnce(ctx)`, and a `select` on `ctx.Done()`
returning `ctx.Err()` or on `<-ticker.C`], and its doc comment already argues the same point for the
same reason [measured c221784:internal/scheduler/worker.go:136-139 · `sed -n '136,139p'
internal/scheduler/worker.go` → `Run does not stop on a RunOnce error — each cycle already reports it
through ObserveLoop (design D12) — because a worker that stopped on a transient discovery failure
would need an external restart for no reason.`].

**The poll interval is the whole retry-rate bound, and no consecutive-failure ramp is added.** The
loop waits `Ingest.PollInterval` between cycles — after a successful cycle and after a failed one
alike — so a failing Bot API is polled once per interval rather than in a tight loop, which is the
busy-loop an observe-and-`continue` with no wait would produce. A second, failure-counting ramp on top would be a third backoff
policy in a package that already sits behind two: `internal/tg` retries a retryable `getUpdates`
attempt up to `RetryMaxAttempts`, waiting `EqualJitter` between attempts, before returning anything
to its caller [measured c221784:internal/tg/caller.go:122-126 · `sed -n '122,126p'
internal/tg/caller.go` → `if !out.retryable || attempts >= c.client.transport.RetryMaxAttempts {`
returning a give-up error, so the loop continues otherwise], so a poll error reaching `Run` is one
the transport layer has already backed off over. Adding a ramp here would compound two delays for one
fault and give the loop a state variable that survives across cycles, which `internal/scheduler` also
declined. If measured behaviour later shows the flat interval is too aggressive against a sustained
outage, that is a tuning change to `LAB_GAME_INGEST_POLL_INTERVAL`, not a new mechanism.

An offset consequence, stated because it is what makes the policy safe: a failed poll returns no
updates, so nothing is settled and the offset does not move (D5). A cycle that could not reach the
server therefore leaves the loop exactly where it was, and the next cycle re-asks for the same
`offset`.

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
| 2 | Adopt `internal/backoff` in the two shipped packages per D2, in this order. **(a) First, the call-site gate**, against the still-shipped one-based `backoff`: in `internal/scheduler/failure_test.go` and `internal/scheduler/deadline_test.go`, replace the `backoff(attempt, …)`-derived expected `run_at` with a **literal one-based ramp table** that calls no ramp function, and confirm it is green — the green run is what proves the literals are the durations the shipped ramp already computes, which is why this substitution is the sole carve-out from (c)'s byte-identical rule rather than a break in it. **(b) Then prove the instrument**: apply the wrong translation (`Exponential(k, …)`) at both `settle.go` sites over a `cp` backup, run the scheduler suite (`go test ./internal/scheduler/`), require **RED** on the failure- and deadline-policy tests, restore from the backup (never `git checkout -- <file>`). A green probe is a STOP for the orchestrator. **(c) Then the adoption**: delete `backoffDelay` and `backoff`, re-point **every** call site — production and test — with the one-based→zero-based translation written at `settle.go`'s call sites, keeping every *pre-existing* assertion and expected value byte-identical. The listed sites are a floor enumerated by `rg -U`; `make verify` is re-run after they clear, and any newly revealed site outside this contract is surfaced, not absorbed | `internal/tg/retry.go`, `internal/tg/caller.go`, `internal/tg/retry_test.go`, `internal/scheduler/cadence.go`, `internal/scheduler/settle.go`, `internal/scheduler/cadence_test.go`, `internal/scheduler/failure_test.go`, `internal/scheduler/deadline_test.go` | 1 |
| 3 | `config.Ingest` + the `LAB_GAME_INGEST_` reader; `EnvKeys()` append; **both** `Load` cross-checks on `LAB_GAME_INGEST_LONG_POLL_TIMEOUT` per D16 — strictly below `AttemptTimeout`, and a whole number of seconds — each a `*KeyError` naming that variable; `.env.example` lines | `internal/config/ingest.go`, `internal/config/config.go`, `internal/config/env.go`, `internal/config/ingest_test.go`, `.env.example` | — |
| 4 | Forward migration `00004_ingest.sql`: `ingest_offset` (guarded singleton, seeded) and `ingest_dead_update`. **The two new tables break `internal/store`'s exact base-table assertion**, which enumerates the whole set and compares with `slices.Equal` [measured febae63:internal/store/migrate_test.go:41-47 · `sed -n '41,47p' internal/store/migrate_test.go` → `want := []string{` … `"account", "account_balance", "account_definition",` … `"scope", "scope_definition",` … `}`]; that set is extended with both new names in this subtask, and the new tables' constraints get their cases in `schema_test.go`, so Group A leaves `internal/store` green on its own rather than relying on its consumer in Group B | `internal/store/migrations/00004_ingest.sql`, `internal/store/migrate_test.go`, `internal/store/schema_test.go` | — |
| 5 | `store.Queryer` and `store.PlayerExists` — the read-only owner lookup, with its tests; **and the delete-and-re-point of the package's shipped test-local near-duplicate** per D11: `queryRower` is removed and `balanceOf` takes `store.Queryer`, so one method set has one name in this package | `internal/store/owner.go`, `internal/store/owner_test.go`, `internal/store/post_test.go` | — |
| 6 | `internal/ingest` core types: package comment, `Kind` with D4's table and its date/chat extractors, `IDSpace` and the unexported `operation_id` builder, `Handler`/`Update`, `Route`/`Router`/`NewRouter`/`Kinds`, the package's sentinels. **The `Handler` doc comment carries two obligations, not one**: propagate the handed `ctx` to every call on `tx` (D7's risk row), and issue no outbound Bot API call whose permission rests on a row this transaction has not committed (D18) | `internal/ingest/doc.go`, `internal/ingest/kind.go`, `internal/ingest/operation.go`, `internal/ingest/router.go`, `internal/ingest/errors.go`, plus their `_test.go` files | — |
| 7 | Offset and give-up storage: read/guarded-advance statements, `DeadUpdate`, `DeadUpdates` over a caller-owned `pgx.Tx` with a deterministic order | `internal/ingest/offset.go`, `internal/ingest/dead.go`, plus their `_test.go` files | 4, 6 |
| 8 | The observation seam: `Outcome`, `Observation`, `LoopObservation`, `Observer`, and the nil-checked report helpers | `internal/ingest/observe.go`, `internal/ingest/observe_test.go` | 6 |
| 9 | The loop: `Options`/`New`/`OptionError`, the requested-kinds set with D3's sentinel, `PollOnce`, `Run` with D19's poll-error policy (observe through `LoopObservation.Err` and continue at the poll interval; return non-nil only on cancellation, as `ctx.Err()`), per-attempt transaction, `recover`, sentinel classification, bounded retry, settlement and offset advance (writing `update_id + 1` per D14), give-up row. **The Files column names this row's home, not a one-file mandate**: the responsibilities listed here are a plausible run at the soft file-size bands [measured df9b1a2:AGENTS.md:128 · `grep -n 'File size' AGENTS.md` → `- **File size:** soft 500/800; hard 1000, and 1500 for \`_test.go\` — both gated; exemptions and the don't-over-split counter-rule are in \`code-style.md\`.`], so split along the seams this row already names — the poll cycle, the per-update attempt loop, the settlement — deliberately at authoring time rather than reactively at the gate, after reading `code-style.md`'s don't-over-split counter-rule | `internal/ingest/loop.go` (splitting as above), `internal/ingest/loop_test.go`, `internal/ingest/retry_test.go` | 1, 3, 6, 7, 8 |
| 10 | The gate: `PlayerLookup`, the pool-backed implementation, `Gate`, `NewGate`, the positive-only cache | `internal/ingest/gate.go`, `internal/ingest/gate_test.go` | 5, 6 |
| 11 | Guard tests: no `panic(`/`log.Fatal`/`os.Exit` in the package's non-test source, no metrics-library import, the `telego.Update` field-vs-kind-table drift check, the package's `TestMain` over `testdb.Main`, and the structural source walks § Test Design specifies — AC2's ctx-first walk, AC8's no-transaction-handed-out walk, and AC38's no-second-Bot-API-path walk | `internal/ingest/guards_test.go`, `internal/ingest/main_test.go` | 6, 9, 10 |
| 12 | Propagation: the layout paragraph and the code inventory in `ai-docs/context.md`; the allowlist and sanitisation bullets in `ai-docs/domain-invariants.md` — the player carve-out, the cache-invalidation obligation, D18's no-outbound-call-on-an-uncommitted-row obligation, and the three concrete §12.5 targets this task creates per D14 (reset `ingest_offset`, which is what the existing «reset the updates offset» clause now names; rewrite `ingest_dead_update`'s chat id; blank its `last_error` free text); the new Key-Decision entries; the `INDEX.md` row. **`ai-docs/context-status.md` is deliberately absent from this row, and its absence is a decision, not an omission**: spec *Technical constraints* item 8 names it among the propagation class, but it is `/task` Step 9.5's append-only per-task log [measured df9b1a2:ai-docs/context-status.md:3 · `sed -n '3p' ai-docs/context-status.md` → `The detailed, append-only implementation log: one entry per completed task … Written by /task Step 9.5, read on demand when touching the area an entry covers.`], so it is a history surface the workflow appends to after implementation — the same standing `AGENTS.md` § Propagation Rule step 4 gives `ai-docs/learnings.md` and `ai-docs/plans/done/**`, which it says to leave untouched. Rewriting past entries in it here would be the propagation error, not the fix | `ai-docs/context.md`, `ai-docs/domain-invariants.md`, `ai-docs/key-decisions.md`, `ai-docs/plans/INDEX.md` | 1–11 |

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
  - **Spawn contract, stated because subtask 2 is the highest-consequence work in this `sonnet`
    group.** It rewrites persisted-`run_at` arithmetic in two shipped packages that no AC of this
    task names, so the group's prompt must carry all of the following, in this order.
    - **The call-site gate of D2, as a deliverable and not as a check** — the literal one-based ramp
      in `internal/scheduler/failure_test.go` and `internal/scheduler/deadline_test.go`, landed and
      green **before** the re-point. Stated explicitly because the byte-identical-assertion gate,
      taken alone, does **not** discriminate the omitted one-based→zero-based translation: the
      shipped expected values are computed through the very function the call site uses, so the
      natural mechanical re-point keeps every suite green while doubling every persisted retry
      delay. The prompt says that, in those words, so the implementor cannot read the
      byte-identical rule as the whole gate.
    - **The mutation probe, with its refusal condition.** With the literal ramp in place, the wrong
      translation applied over a `cp` backup of `settle.go` must turn
      `go test ./internal/scheduler/ -run 'TestFailurePolicy|TestDeadline'` **RED**; restore from the
      backup, never with `git checkout -- <file>`. If the probe comes back green, the implementor
      **stops and returns to the orchestrator** rather than proceeding — a gate that cannot go red
      is not evidence about the adoption (`AGENTS.md` § Patterns 2).
    - **`go test ./internal/tg/ ./internal/scheduler/` is green before the group returns**, run as
      its own step rather than folded into a final `make verify`. The point is *where* a changed
      pre-existing expected value surfaces — inside the group, with the adoption diff still in hand
      and the implementor able to escalate it as the scope-boundary item D2 says it is, rather than
      at `make verify` after five more subtasks have landed on top of it. `internal/store`'s suite
      gets the same treatment for subtask 5's `queryRower` re-point (`go test ./internal/store/`),
      for the same reason at a smaller scale.
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
  well-tested code that no AC of this task names, and its blast radius reaches production and test
  files in both packages.** The production reach is `internal/tg/caller.go`'s retry wait and
  `internal/scheduler/settle.go`'s persisted `run_at` computations; the test reach is the direct
  calls in `internal/tg/retry_test.go`, `internal/scheduler/cadence_test.go`,
  `internal/scheduler/failure_test.go` and `internal/scheduler/deadline_test.go`
  `[measured febae63 · `rg -U --type go -n 'backoffDelay\s*\(' internal/tg/` and
  `rg -U --pcre2 --type go -n '(?<![A-Za-z_.])backoff\s*\(' internal/scheduler/` → those files and
  no others, each package's declaration included]`.
  Mitigation, in two parts, because the first part alone is refuted by measurement. **(1)** After a
  delete-and-re-point those test files do **not** compile unchanged, so "they pass unchanged" is a
  mitigation that cannot hold; what holds is that **every *pre-existing* assertion and expected value
  in them stays byte-identical and only the call expression is re-pointed**, so a changed
  pre-existing expected value is the signal that the adoption was not behaviour-preserving and is a
  scope-boundary item for the orchestrator, not something the implementor absorbs. **(2)** That gate
  does **not** discriminate the one defect class it most needs to — the omitted one-based→zero-based
  translation at `internal/scheduler`'s persisted-`run_at` sites — because the shipped assertions
  over a persisted retry `run_at` compute their expected value through the same function the call
  site uses `[measured df9b1a2:internal/scheduler/failure_test.go:154 and
  internal/scheduler/deadline_test.go:252 · `rg -n --pcre2 '(?<![A-Za-z_.])backoff\s*\('
  internal/scheduler/*_test.go` → `want := backoff(attempt, cfg.RetryBaseDelay, cfg.RetryMaxDelay)`
  at each; every other test hit pins the function itself in `cadence_test.go`, or is comment and
  failure-message prose]`, so a re-point that omits the translation *consistently* leaves every
  assertion byte-identical and `make verify` green while doubling every persisted retry delay. D2
  therefore adds a **call-site** gate ahead of the adoption: a literal one-based ramp in
  `failure_test.go` and `deadline_test.go`, green against the shipped ramp first, then proven to go
  RED under the wrong translation before the re-point lands — `[derived → subtask 2's literal-ramp
  gate and its mutation probe]`. The tests being preserved are the ones already shipped
  `[measured 1fce8b5:internal/scheduler/cadence_test.go:8,33; internal/tg/retry_test.go:127,682 ·
  `grep -n 'func Test.*[Bb]ackoff' internal/scheduler/cadence_test.go internal/tg/retry_test.go` →
  `TestRetry_JitterOptionThreadedThroughToBackoff`, `TestBackoffDelay_JitterBoundsExactly`,
  `TestBackoff_exactTable`, `TestBackoff_strictlyGrowingUntilCeiling`]`.
- **A shared `Exponential` written the obvious way overflows where the two ramps it replaces could
  not, and the damage lands in persisted data.** Both shipped ramps test the ceiling *before*
  doubling and leave the loop there `[measured c221784:internal/tg/retry.go:33-34 and
  internal/scheduler/cadence.go:46-47 · `sed -n '33,34p' internal/tg/retry.go; sed -n '46,47p'
  internal/scheduler/cadence.go` → `if d > maxDelay/2 {` / `d = maxDelay` and `if d >= ceiling {` /
  `return ceiling`]`, so neither can wrap `time.Duration`'s `int64`. A `base << attempt`, or a loop
  whose ceiling test comes after the doubling, can — and the attempt is operator-reachable, since the
  attempt caps are parsed by a helper that bounds only from below `[measured
  c221784:internal/config/transport.go:207-217 · `sed -n '207,217p' internal/config/transport.go` →
  `func lookupPositiveInt(…)` whose only rejection is `if err != nil || n <= 0`]` and
  `internal/scheduler` feeds `consecutive_failures` straight in `[measured
  c221784:internal/scheduler/settle.go:142,242 · `grep -n 'backoff(k,' internal/scheduler/settle.go`
  → the same `s.Add(backoff(k, cfg.RetryBaseDelay, cfg.RetryMaxDelay))` at both lines]`. A negative
  delay there writes a `run_at` in the past — a hot-looping dead task, inside a change billed as
  behaviour-preserving. Mitigation: D2 makes clamp-before-overflow part of `Exponential`'s stated
  contract rather than an implementation accident, and subtask 1's table asserts it at attempts far
  past any a naive shift survives — `[derived → AC23 and subtask 1's overflow cases]`.
- **The gate cannot see a row the handler's own open transaction wrote, and §1's onboarding is
  exactly that shape.** A handler that creates a player's `owner` row and DMs that player before
  returning is refused on every attempt and lands in `ingest_dead_update`. Mitigation: D18 makes this
  a `Handler`-contract rule rather than a discovery — the obligation is on the `Handler` doc comment
  (subtask 6), pinned by a negative scenario (subtask 10) and written into
  `ai-docs/domain-invariants.md` (subtask 12) — and the designed route for a post-commit send is
  #43's notification queue, which is out of this task's scope. The residual risk is that #30 writes
  the naive shape anyway; the doc comment and the invariants bullet are what a reviewer of #30 reads
  — `[derived → subtask 10's uncommitted-owner-row scenario and subtask 12]`.
- **`Run` never returning on a persistently failing Bot API is the deliberate choice, so the failure
  is visible only through the observation seam.** D19 keeps the loop polling at
  `LAB_GAME_INGEST_POLL_INTERVAL` and reports each failure through `LoopObservation.Err`; with no
  exporter in this task (#23 owns exposition), a sustained outage is silent to an operator until #23
  lands. That is the same standing `internal/scheduler` already ships under, and §13.2's alerting
  pairs canary failures with rising update lag rather than watching this loop's return value.
  Mitigation: the poll error is reported, not swallowed, and `PollOnce` still returns it, so a caller
  that wants to stop on one has the primitive — `[derived → AC17 and subtask 9's poll-error
  scenario]`.
- **`00004_ingest.sql` reds `internal/store`'s own suite the moment it lands**, because that suite
  asserts the whole base-table set by equality rather than by containment, so any new table is a
  failure until the expectation is extended `[measured febae63:internal/store/migrate_test.go:41-49 ·
  `sed -n '41,49p' internal/store/migrate_test.go` → the `want := []string{…}` base-table list
  followed by `if !slices.Equal(tables, want)`]`. Mitigation: subtask 4 owns the migration and that
  expectation in one step, so its group leaves `internal/store` green without waiting for its
  consumer in the next group — `[derived → AC7, AC30]`.
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
- **`AllowCall` is a concurrent seam, so the cache's shape is a correctness question, not a
  performance one.** An unsynchronised map there is a `concurrent map writes` fatal error on the
  component whose whole job is to stop a write to an unintended chat — the worst possible place for
  a crash — and the outbound path is concurrent by construction, which is why the limiter
  immediately downstream keeps its own state under a mutex `[measured
  febae63:internal/tg/limit.go:202-207 · `sed -n '202,207p' internal/tg/limit.go` → `type Limiter
  struct {` / `mu     sync.Mutex` / `chats  map[chatKey]*schedule`]`. Mitigation: D10 fixes the
  concrete shape (a mutex-guarded `map[int64]struct{}`, with the lookup issued outside the lock),
  and subtask 10 carries a scenario driving `AllowCall` from concurrent goroutines under
  `go test -race`, which `AGENTS.md` § Go Test Conventions makes a required gate for shared state —
  `[derived → AC35 and subtask 10's concurrency scenario]`.
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

`internal/ingest`'s database-backed cases take a fresh schema from `testdb.Schema(t)` and apply
`store.Migrate`, and its Bot API cases point `tg.New` at a `tgtest.Server` through
`Options.HTTPClient`, `tgtest.BaseURL` and `tgtest.Token` — no mock of this package's own making
anywhere `[derived → AC29]`.

**Subtask 1 — `internal/backoff`** (`internal/backoff/backoff_test.go`)
- Entry points: `Exponential`, `EqualJitter`.
- Scenarios: an exact table of `(attempt, base, ceiling) → delay` including the zeroth attempt, the
  attempt at which the ceiling is first reached, and every attempt past it; strict positivity and
  non-decrease across a run of attempts; the ceiling never exceeded; a negative attempt clamped to
  the zeroth; **a `base` already greater than `ceiling`, at the zeroth attempt, returning `ceiling`
  exactly** — the row that makes this table cover the adopters' full input domain rather than leaving
  that half to subtask 2, because `internal/tg` ships an assertion on precisely that input and
  `internal/scheduler`'s ramp answers it at its own post-loop clamp (D2); `EqualJitter` bounded
  within `[d/2, d)` for a jitter stub returning 0 and one returning a value just under 1, its
  `base > ceiling` counterpart at `ceiling/2`, and its monotonicity for a fixed jitter
  `[derived → AC23 and D2's `base > ceiling` contract clause]`.
- **Overflow cases, the ones D2's contract exists for.** Explicit table rows at attempts far past any
  a naive `base << attempt` survives — `64` (where a shift of a nanosecond base has already consumed
  `int64`'s range) and `1000` (where no shift is even defined) — asserting for each that the result
  is **exactly** `ceiling` and **strictly positive**, and the same pair for `EqualJitter` asserting
  the result stays within `[ceiling/2, ceiling)`. The assertions are on the value, never on "it did
  not panic": a wrapped `time.Duration` is a silent negative, not a crash, so a case that only ran
  the function would pass against the defect it was written to catch. Repeated for a base at the
  small end (`1ns`, where doubling reaches the wrap soonest) and at an operationally realistic one
  (`1s`) `[derived → AC23 and D2's clamp-before-overflow contract]`.
- Fixtures: a deterministic `jitter func() float64` stub. No database, no bubble.

**Subtask 2 — the adoption** (no new test file; the shipped test files listed in § Decomposition
row 2 are edited)
- Entry points: unchanged — the production behaviour under test is `internal/tg`'s retry wait and
  `internal/scheduler`'s persisted `run_at`, now computed through `internal/backoff`.
- The first half of the gate, stated as what a delete-and-re-point can achieve: in
  `internal/tg/retry_test.go`, `internal/scheduler/cadence_test.go`,
  `internal/scheduler/failure_test.go` and `internal/scheduler/deadline_test.go`, **every
  *pre-existing* assertion and expected value is byte-identical to the shipped one and only the call
  expression is re-pointed**; the suites are then green. "Passing unchanged" is *not* the gate and
  must not be recorded as one — after the local ramps are deleted those files do not compile until
  their call expressions move, so a test that has not been edited has not passed `[derived → AC30]`.
- **The second half — a new assertion, and the one that can go red.** New test text in
  `internal/scheduler/failure_test.go` and `internal/scheduler/deadline_test.go`: the persisted
  `run_at` bracket is computed from a **literal one-based ramp table** indexed by the loop's
  `attempt`, containing no call to `backoff`, to `backoff.Exponential`, or to any shift or doubling
  expression. It lands and is green **against the shipped one-based ramp** before the re-point, and
  it is the assertion an omitted one-based→zero-based translation reds. **It substitutes for the
  `backoff(attempt, …)`-derived expected value at those brackets, and that is the sole carve-out from
  the bullet above** — not a real exception, because the literal durations it writes are the ones the
  shipped ramp already computes, which is what its green run against the shipped ramp establishes
  before the re-point; from that point on it is itself a pre-existing expected value the re-point
  must leave byte-identical. Nowhere else in these files does new assertion text land
  `[derived → AC30 and D2's call-site gate]`.
- **The instrument is proven before it is trusted.** With the literal ramp in place, the wrong
  translation is applied at both `settle.go` sites over a `cp` backup, and
  `go test ./internal/scheduler/ -run 'TestFailurePolicy|TestDeadline'` must report **FAIL**; the
  backup is then copied back (never `git checkout -- <file>`, which discards every other uncommitted
  edit to the file). A probe that passes means the gate does not discriminate and is a STOP for the
  orchestrator, not a result to reason past — `AGENTS.md` § Patterns 2 `[derived → D2's mutation
  probe]`.
- **What the `internal/backoff` table of subtask 1 does *not* cover, said plainly so no later reader
  re-derives the false version.** That table pins the shared function's own contract in zero-based
  units, independently of either adopter — and says nothing about *which argument a call site
  passes*. A re-pointed call site that drops the translation therefore produces **no** disagreement
  between the two suites: both stay green together. The call-site gate above is the only instrument
  in this design that sees it `[derived → AC23, AC30]`.

**Subtask 3 — `config.Ingest`** (`internal/config/ingest_test.go`)
- Entry points: `Load`, and the package's existing disjointness and required-variable suites.
- Scenarios: every ingest key absent takes its documented default; each key present and valid is
  parsed; each key present and malformed produces a `*KeyError` naming that variable and wrapping
  `ErrInvalidValue`; a batch limit outside the Bot API's accepted range is rejected; a long-poll
  timeout at or above `AttemptTimeout` is rejected naming the long-poll variable, and one below it is
  accepted; **a long-poll timeout that is not a whole number of seconds — `25500ms`, and `1500ms` as
  a second value below any plausible `AttemptTimeout` so the case cannot pass for the neighbouring
  check's reason — is rejected with a `*KeyError` naming `LAB_GAME_INGEST_LONG_POLL_TIMEOUT`, while
  `25s` is accepted** (D16: `Load` owns this check, the loop never truncates); the recorded-lookup
  disjointness suite and the `.env.example` set-equality suite still
  pass with the new keys `[derived → AC27]`.
- Fixtures: the package's existing map-backed `Lookup`; no process environment is touched.

**Subtask 4 — the migration** (`internal/store/migrate_test.go`, `internal/store/schema_test.go`)
- Entry point: `store.Migrate` against a fresh schema.
- Scenarios: the shipped exact base-table assertion — which compares the whole set with
  `slices.Equal`, so it fails on any addition until extended — is extended with `ingest_offset` and
  `ingest_dead_update`, and nothing else about it changes; a second `Migrate` is a no-op; the offset
  row exists exactly once after migration and a second row is refused by the singleton `CHECK`; a
  give-up row with an empty kind is refused; the migration file carries no `-- +goose Down` section
  and the shipped migration files' contents are unchanged `[derived → AC7]`.
- This subtask's group must leave `internal/store` green on its own — its consumer lands in a later
  group — so the expectation edit belongs here, in the same step as the `.sql` file, not with the
  code that reads the tables `[derived → AC30]`.
- Fixtures: `testdb.Schema(t)`, `slog.New(slog.DiscardHandler)`.

**Subtask 5 — `store.PlayerExists`** (`internal/store/owner_test.go`)
- Entry point: `PlayerExists(ctx, q, telegramID)`.
- Scenarios: a `kind = 'player'` owner with that `telegram_id` reports true; a `kind = 'chat'` owner
  with the *same* `telegram_id` reports false — the pair, not the column, is the predicate; an id
  with no owner row reports false; a closed pool surfaces the error rather than a false; the same
  call through a `pgx.Tx` and through a `*pgxpool.Pool` behaves identically `[derived → AC28, AC33]`.
- **The `queryRower` re-point is gated by compilation, not by a new test.** `post_test.go`'s
  `balanceOf` is the only consumer, so deleting the near-duplicate and re-pointing it at
  `store.Queryer` either compiles or does not; the package's shipped posting tests are what prove the
  helper still reads the same balances, and — as in subtask 2 — **every assertion and expected value
  in them stays byte-identical, only the parameter type changes**. `go test ./internal/store/` is run
  as its own step before the group returns `[derived → AC30]`.
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
  - *The transmitted offset after a settlement — D14's persisted semantic, pinned on the wire.* After
    an update with a known `update_id` is settled, the **next** `getUpdates` request body carries
    `offset` equal to that `update_id + 1`, decoded from the request the `tgtest` handler received;
    the same assertion is made for each settled outcome the loop has — handled, duplicate, unrouted
    and given-up — since each of them advances. This is the scenario that fails if the column's meaning
    and the transmitted value disagree, which is the live-lock D14 refuses: storing the raw
    `update_id` and transmitting it re-fetches the same update forever while the guarded advance
    silently rejects every re-write, and the only other symptom is the idempotency-hit counter
    §13.2 puts on the health surface — where duplicates are ordinary traffic — with no exporter
    reading it until #23. The poll-failure scenario below asserts the offset is
    *unchanged* across failing cycles, which passes under either semantic, so it does not reach this
    `[derived → AC6, AC25 and D14]`.
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
  - *Poll failure — D19's claims, asserted separately.* A `tgtest` handler answering `getUpdates`
    with a 5xx for the first N cycles and then with a normal batch, driven through `Run`. The
    assertions are: `Run` **does not exit** while the server is failing (it is still running when the
    handler starts succeeding, and the update from the first successful cycle is handled); it **does
    not busy-loop** — the failing cycles are counted at the `tgtest` handler and their count over a
    window is consistent with the configured poll interval rather than with an unbounded spin, which
    is the assertion that fails on a `continue` with no wait; the **offset does not advance** across
    the failing cycles (read from the row, and the first successful `getUpdates` still carries the
    pre-failure offset — the claim D19 rests on, since a failed poll settles nothing); and the error
    **reaches the observer** through `LoopObservation.Err`, once per failed cycle. Plus the direct
    primitive: `PollOnce` on a failing server returns the error to its caller `[derived → AC17 and
    D19]`.
    - The interval assertion needs a poll interval large enough that a spin is distinguishable from
      the configured cadence and small enough not to slow the suite; `config.Ingest`'s
      millisecond-scale test values give that, and the assertion is a floor on elapsed time across a
      known number of failing cycles rather than an equality on a duration — a wall-clock equality
      here would be the flake `internal/scheduler`'s timing tests already avoid.
  - *Cancellation.* Cancelling the loop's context mid-retry leaves the update unsettled with the
    offset behind it; a long poll in flight when the context is cancelled returns promptly and `Run`
    returns without a goroutine surviving the test; `Run`'s returned error is `ctx.Err()`, and
    cancellation is the **only** input under which `Run` returns non-nil — the poll-failure scenario
    above is the negative half of that same claim `[derived → AC25, AC26]`.
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
- **The uncommitted-owner-row scenario — D18's behaviour, pinned rather than incidental.** Against a
  real schema and the pool-backed `PlayerLookup`: open a transaction, `store.CreateOwner` a
  `kind = 'player'` row with a `telegram_id` absent from `AllowedChatIDs` — the shipped constructor
  already takes the transaction, so the fixture needs nothing new
  [measured c221784:internal/store/owner.go:41 · `grep -n '^func CreateOwner' internal/store/owner.go`
  → `41:func CreateOwner(ctx context.Context, tx pgx.Tx, kind OwnerKind, telegramID *int64) (Owner,
  error) {`] — and, **without committing**, call `AllowCall` for that destination on a gate holding
  the pool. The call is
  **refused**. Then commit, and a second `AllowCall` for the same destination is **allowed** with no
  restart and no new gate. The pair is what makes the scenario an assertion about visibility rather
  than about the allowlist: the same id, the same gate, the same pool, and the only variable is the
  commit. It also proves the refusal left nothing negative in the cache, which is AC35's requirement
  read from the other side. A comment on the test names D18, so a later reader meets the rule rather
  than the symptom `[derived → AC34, AC35 and D18]`.
- Concurrency scenario, run under `go test -race`: many goroutines call `AllowCall` on one gate at
  once — a mix of ids that the stub allows and ids it refuses, including several goroutines racing
  on the *same* not-yet-cached allowed id — and every call returns the answer the stub's fixed
  answers imply, with no data race reported and no panic. This is the scenario that would fail on a
  bare `map[int64]bool`, so it is the one that makes D10's mutex load-bearing rather than
  decorative; the same-id racers additionally pin that a duplicate first lookup is benign (both
  writers write the same value) `[derived → AC35 and D10's cache shape]`.
- Integration scenario: with the gate installed at `tg.New`, a refused outbound call reaches the
  `tgtest` server not at all and is reported through `tg.Observation` with no attempt recorded, and a
  subsequent allowed call into the same chat and method class is not delayed by a limiter window the
  refusal would have spent `[derived → AC16]`. This is AC38 by **example** — one refused call, one
  server that never saw it. AC38's **structural** claim, that no production path can skip the gate at
  all, is not an example's to make and is discharged in subtask 11 `[derived → AC38]`.
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
- **AC8's structural half, which the happy path of subtask 9 does not reach.** That scenario shows
  that *a* handler receives the loop's transaction; AC8's second clause is the wider claim that **no
  exported entry point of the package lets a handler obtain a transaction the loop does not own**.
  That is a structural property an example cannot make — the same shape as AC38's structural half
  below, and it gets the same discharge here rather than a scenario. The `go/ast` walk over the
  package's non-test files asserts that no exported function or method of `internal/ingest`
  **returns** a `pgx.Tx` or a `pgx.Conn`, and that no exported type declares an exported field of
  either type — so the only transaction a handler can reach is the one the loop passes it as the
  `Handler` parameter. The walk distinguishes **parameters** from results and fields, and the two
  shapes that distinction deliberately leaves legal are named in the test so a later reader cannot
  relax the guard by re-deriving them: `DeadUpdates`, which *takes* a caller-owned `pgx.Tx` (AC37) —
  a consumer of a transaction, never a source of one — and `Options.Pool`, the `*pgxpool.Pool` the
  constructor is handed, which is a connection source rather than a transaction and is reachable
  only at wiring time, not from inside a handler. An exported accessor added later that hands the
  loop's `tx` out reds the suite `[derived → AC8]`.
- **AC38's structural half, which the integration scenario of subtask 10 does not reach.** That
  scenario proves *a* refused call never reaches the server; AC38 is the wider claim that no
  production path outside `internal/tg` can issue an outbound call at all without passing the gate.
  It splits in two, and both halves are discharged structurally rather than by example. The
  `internal/tg` half is **already shipped** and this task adds nothing to that package
  [measured c221784:internal/tg/guards_test.go:216 · `grep -n
  'func TestGuard_RefusingGateBlocksTheAccessor' internal/tg/guards_test.go` →
  `216:func TestGuard_RefusingGateBlocksTheAccessor(t *testing.T) {`], so AC38 points at that guard
  by name rather than re-proving it here. The `internal/ingest` half is this subtask's: a source walk
  asserting the package's non-test files construct no `telego.Bot`, no `http.Client` and no
  `telegoapi` caller of their own, and reach the Bot API only through the `*telego.Bot` that
  `tg.Client.API()` returns — which is AC3's structure and AC38's second half in one walk
  `[derived → AC3, AC38]`.
- Additional scenario, because no enabled linter covers it: no type declared in `internal/ingest` has
  a `context.Context` field, and **every exported method of the package takes `ctx context.Context`
  as its first parameter unless it is listed in a named, commented exemption set in the test file
  itself** — asserted by a `go/ast` walk over the package's non-test files, since `containedctx` is
  not among the enabled linters
  [measured c221784:.golangci.yml:12-37 · `grep -n 'enable:\|containedctx\|bodyclose\|whitespace'
  .golangci.yml` → `12:  enable:`, `14:    - bodyclose`, `37:    - whitespace`, `60:  enable:` (the
  formatters block), and no `containedctx` line] `[derived → AC2]`.
  - **The exemption set is what makes the guard decidable, and the wording is deliberate.** AC2's own
    predicate — "every exported method **that reaches the network or the database**" — is not
    computable from an AST: reachability is a whole-program property, so a walk written to AC2's
    literal words either over-approximates (failing on a legitimate ctx-less accessor such as
    `Router.Kinds`) or gets quietly relaxed at implementation time into something that proves less
    than AC2. Inverting it fixes that: the walk demands `ctx` first from **every** exported method,
    and the only escape is an entry in an exemption set declared in the test file with a comment
    saying why that method reaches neither. The guard is then total and mechanical, the
    over-approximation is discharged once per exemption instead of silently, and an added ctx-less
    method reds the suite until someone writes the reason down — which is strictly stronger than
    AC2's literal reading, since a method that *does* reach the database can only pass by someone
    entering a false justification into the diff a reviewer reads `[derived → AC2]`.
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

- **A per-update execution deadline.** D7 ships none. A handler that ignores its context stalls the
  sequential loop with no bound; the scheduler solved the same problem with a transaction-local
  `statement_timeout` and a hijackable connection. Nothing in this task's ACs requires it and no
  production handler exists yet, so it is recorded rather than built.
- **Whether `cmd/bot` becomes a running process here.** The spec says no, on #19's precedent; this
  design follows the spec and records the composition order (D17) instead. Changing that is a scope
  change, not a design amendment.

**Settled in round 2 and no longer open** — recorded here so a later reader does not reopen them:
the `internal/tg` half of D2's adoption stays in scope (`internal/backoff` ships and **both**
adopters are re-expressed in terms of it — D2), and the offset row stays the singleton D14
specifies, with the per-bot widening left to a later forward migration (D14).
