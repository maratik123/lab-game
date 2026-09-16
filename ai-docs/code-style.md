# Code style — lab-game

The canonical, growing reference behind `AGENTS.md` § *Code Style*. Rules land here through the learning loop (`/improve`); the file is expected to start thin.

## Source files

Go only, under `cmd/<binary>/` (process entry points) and `internal/<package>/` (everything else). Nothing is exported for outside consumers — this module has none (`AGENTS.md` § API Stability), so `internal/` is the default home and `pkg/` is not used.

## Linter posture

`golangci-lint run` with `.golangci.yml`, strict. A finding is fixed, not silenced.

- `//nolint:<linter> // <reason>` — both parts mandatory (`nolintlint` enforces them). A bare `//nolint` fails the gate.
- A rule that keeps firing on correct code is a config bug: change `.golangci.yml` in a reviewed diff, never scatter suppressions.
- **Enabled beyond the standard set — and the analyzers switched on inside it — with the reason for each (KD-16).** `.golangci.yml` is the authority; this is that file's `linters.enable`, in its own grouping.

  | Linter | Why |
  |---|---|
  | `bodyclose` | a Telegram Bot API response body left open holds a connection |
  | `containedctx` | a context stored in a struct field outlives the call it was made for |
  | `contextcheck` | a function that should thread the caller's context must not invent one |
  | `durationcheck` | multiplying two durations is a unit bug wearing arithmetic |
  | `errorlint` | `%w` wrapping, `errors.Is`/`As` over `==` and type asserts |
  | `exhaustive` | an FSM state or enum switch must be total |
  | `fatcontext` | a context nested in a loop or a function literal grows a chain per iteration |
  | `forbidigo` | no fresh root context, no unstoppable timer or ticker — § *Concurrency* rules 5 and 6 |
  | `makezero` | `make([]T, n)` followed by `append` silently prefixes n zero values |
  | `nilerr` | returning `nil` after a non-nil error check |
  | `noctx` | no outbound HTTP without a context |
  | `rowserrcheck`, `sqlclosecheck` | a forgotten `rows.Err()` reads as an empty result, i.e. silent data loss; rows and statements are closed |
  | `unconvert`, `wastedassign` | dead conversions, dead stores |
  | `gosec` | string-built SQL and the rest of the security set |
  | `asciicheck` | identifiers stay ASCII; it checks identifiers only, so Russian prose is untouched |
  | `goconst`, `misspell`, `predeclared`, `whitespace` | style and clarity |
  | `gocritic` | its own default checkers, plus `deferInLoop`: a `defer` inside a loop does not run until the whole function returns |
  | `nolintlint` | a `//nolint` carries a specific linter and a stated reason |
  | `revive` | incl. exported-symbol doc comments |
  | `unparam` | a parameter that is always the same value at every call site |
  | `govet` (standard) | its own default analyzers, plus `nilness`: a provable nil dereference or an impossible nil comparison |

- **Two carve-outs, each scoped to `forbidigo` alone.** `_test\.go` and `^cmd/` are outside the fresh-root-context and unstoppable-timer patterns and outside nothing else — an exclusion rule that names a `path` and no `linters` list switches **every** linter off for that path. The non-`_test.go` files of a package only tests import are gated exactly like production code.
- **The settings that keep the gate reporting are pinned, and a guard in `internal/gateguard` fails when any of them drifts.** `run.relative-path-mode: gomod` makes the `^cmd/` anchor mean the module root wherever the gate is invoked from. The rest are the defaults that suppress a report: `issues.max-issues-per-linter: 0` and `issues.max-same-issues: 0` stop the gate truncating a class after its first few findings, and `issues.uniq-by-line: false` stops it keeping only one finding per source **line** across linters — which otherwise hides, say, a `deferInLoop` behind an `errcheck` finding that claimed the line first. Under those defaults "every site is handled" is not a measurable claim. The `uniq-by-line` assertion is a present-and-boolean-`false` check, not a zero-value one: the key's own default is `true`, so its **absence** is the unsafe state.
- **An unrecognised key under `linters.settings` is accepted in silence.** `golangci-lint run` reports nothing, prints no warning and exits green on a settings block it ignored, so run `golangci-lint config verify` before believing any green run of a changed `.golangci.yml`.
- **The format gate is `golangci-lint fmt -d`.** It runs every formatter `.golangci.yml` enables — `gofmt`, `goimports` and `gofumpt` — and exits non-zero when any of them would rewrite a file, printing the diff and changing nothing. `golangci-lint fmt` (no `-d`) is the apply form; plain `gofmt` is strictly weaker and accepts files this gate rejects.
- `make verify` runs every gate of `AGENTS.md` § *Build & Test* in one go, from the same sub-targets CI invokes — so a local run and a CI run cannot disagree about what a gate's command is. One gate CI reaches by another route, deliberately, and the `Makefile` header names it with its reason: `actionlint`, because the binary is not preinstalled on `ubuntu-latest`, so CI uses `reviewdog/action-actionlint` while `make actionlint` is the local path. Everything else goes through `make` on both sides, the harness `shellcheck` sweep included — a second spelling of that gate is how `.githooks/**` came to be shellchecked locally and not in CI at all.

## Comments

Comments carry no outward reference — no markdown path, design-section number, acceptance-criterion id, decision anchor, review-register finding id, issue number outside `TODO(#…)`, repository path, URL, or package-qualified symbol of this module named outside the comment's own package. `make comment-refs` gates the lexical half; narration and a bare unqualified name used as a pointer are review-judged. The rule, its exemptions and the gated file set live in [`doc-convention.md`](doc-convention.md) § DOC-4.

A shell script does not document its own invocation grammar in a comment either: it answers `-h` / `--help` with that grammar and exits 0, in one shape copied across every script, gated by `ai-docs/scripts/check-script-shape.sh`.

## Errors

- Wrap with context and `%w`: `fmt.Errorf("materialize node %s: %w", coord, err)`. The prefix names the operation, not the error.
- Never discard: `_ = err` is a defect. If an error genuinely cannot be acted on, say why in a comment on the line that drops it.
- Sentinels are `var ErrNoStamina = errors.New("no stamina")`; compare with `errors.Is`, never `==` on a wrapped error, never string matching.
- A domain rejection is not an infrastructure failure: distinguish "the player cannot afford this" (an expected outcome the handler renders) from "the database is unreachable" (a retry/alert path).
- No `panic` in production code — see [`go-test-conventions.md`](go-test-conventions.md) and [`panic-index.md`](panic-index.md).

## Context

`ctx context.Context` is the first parameter of anything that touches the database, the network, or the scheduler. Never store a context in a struct. Never pass `context.Background()` from inside a request path — thread the caller's. Both are lint-gated: `containedctx` for the stored context, `forbidigo` for a fresh `context.Background()` or `context.TODO()` anywhere outside a `main` package or a test (§ *Linter posture*). A site that genuinely needs a fresh root keeps one only with a stated reason and a named owner, and a detached one also with its own timeout (§ *Concurrency*).

## Concurrency

> **AXIOM — Every goroutine has an owner that can stop it, and four reviewed answers: how it stops, who waits for it, where its error goes, where its panic goes.**
> A launch missing any one of the four is a leak that has not fired yet, not a launch that is fine.
>
> | If you see... | Action |
> |---|---|
> | A `go` statement added to compiled non-test source | **ADD** its row to the launch allow list in `internal/gateguard`, with all four answers filled in — the guard fails on an unkeyed launch and on an empty answer |
> | A goroutine started from `init()` or from a constructor | **REPLACE** it with an explicit lifecycle: `Run(ctx) error`, or `Start` plus `Stop` / `Close` |
> | A blocking operation inside a goroutine that cannot wake on cancellation | **REWRITE** it to select on `ctx.Done()` beside the stop seam |

- The bot is a stateless handler over Postgres; **state lives in the database, not in memory** (KD-7/KD-8). A goroutine holding game state between updates is a design error, not an optimisation.
- `go test -race` is a required gate for any change that adds a goroutine or shared state.

### Ownership rules

1. **No goroutine starts from `init()` or as a constructor's side effect.** A component that starts goroutines exposes an explicit lifecycle instead: a `Run(ctx) error` that blocks until it is stopped or cancelled, or `Start` plus `Stop` / `Close`.
2. **A new long-lived component joins the composition root as a runner or as a closer — never as a free goroutine.** `cmd/bot`'s start-up appends it to the one closer list the unwind and the drain both walk backwards (KD-32), so its shutdown order is a property of that list rather than a claim beside it.
3. **A goroutine does not outlive its context or its stop seam.** Every blocking operation inside it — a channel send or receive, a lock, a network call — can wake on cancellation. A goroutine that outlives its context is a leak even if it ends eventually.
4. **The one named exception is a scheduler handler blocked outside the database after its deadline, and it covers that goroutine alone.** The `Handler` contract covers it: a handler that issues statements on a context of its own never sees the deadline's cancellation, and no mechanism reclaims that goroutine. The exception is written into the contract rather than assumed at the call site. Two halves are **outside** it. A handler blocked **in** the database is reclaimed: after the breach the worker terminates that attempt's own backend from another pooled connection, so the blocked statement fails, the handler returns, and both its goroutine and its connection are released. And the **row's lock** is outside the exception in either case — a lock is held by a backend and cannot outlive one, so the terminate frees the row whether the handler is blocked in the database or out of it, and a terminate that reports no live backend means something else already ended that backend and released its locks with it.
5. **A `CancelFunc` is deferred where its context is derived** — on that line or the next. `context.WithCancelCause` and `context.AfterFunc` carry an explicit reason for the stop.
6. **A timer or a ticker is stopped.** `time.After` and `time.Tick` hand out one that cannot be; both fail the lint gate outside `main` packages and tests (§ *Linter posture*). Use `time.NewTimer` / `time.NewTicker` with a deferred `Stop`, in a helper body rather than inside the loop — a `defer` inside a loop is its own finding.
7. **The sender closes a channel, and nobody else does.** A receiver's early return never strands a sender: either the channel is buffered to the number of senders, or every sender selects on cancellation.
8. **No goroutine per inbound update or message without a bound.**
9. **A goroutine that runs a handler supplied through an interface recovers a panic at that boundary**, records its stack, and routes it into the same failure path a returned error takes. A package's **own** loops are not wrapped: a panic there is this module's own defect, and swallowing it hides the bug the crash would have named.
10. **A library that holds background goroutines — an HTTP transport, a pool, a client, a logger — is closed on the shutdown path**, through the composition root's closer list.
11. **A test stops everything it started, through `t.Cleanup`.**
12. **A runner is covered both when it is stopped and when it is cancelled**, each case asserting that `Run` returns within a bound.

### Reviewer's checklist

Per `go` statement:

- Who waits for it to finish? If nobody does, why is that acceptable?
- How does it stop? **No answer is a blocker**, not a nit.
- Can every blocking operation inside it wake on cancellation?
- Where does its error go? Where does its panic go?
- Is the number of such goroutines bounded?

Per channel:

- Who closes it, and is that the only closer?
- What happens to a sender when the receiver leaves early?

Per context:

- Why does a `context.Background()` or `context.TODO()` sit below `main`? A surviving site carries a stated reason and a named owner.
- Is `cancel` called on every path?
- Does a detached operation — one whose work continues after the function that created its context returned, with nothing in the process joining it — carry its own timeout as well as an owner?

Per resource:

- Is `Close` / `Shutdown` / `Stop` called in a `defer`, or on the service's explicit shutdown path?

Per pull request:

- A pull request that adds a goroutine says in its description how that goroutine dies.

## Database access

- One transaction per game operation, with its basis document and postings inside it ([`domain-invariants.md`](domain-invariants.md)).
- Balance `UPDATE`s go through `store.Post`, or through `store.Move` when the same document also moves an item instance; `store.Post`'s body owns the capture order both of them take, and a handler that locks accounts by hand can deadlock and will be rejected in review.
- Always check `rows.Err()` after iterating (`rowserrcheck` enforces it); always close what you open (`sqlclosecheck`).
- Queries are parameterised. String-built SQL is a `gosec` finding and a security bug.

## Magic numbers vs balance constants

Two different rules, and conflating them is the common mistake:

| Kind of value | Where it belongs |
|---|---|
| A structural constant (the number of edges of a hex, a protocol limit) | A named Go constant next to the code that owns it |
| A **balance** value (stamina cap, step cost, timers, shop rates, door price curve, `budget(dist)`, combat dice) | **Configuration**, per `~/lab-private/DESIGN.md` §16.5 — the tracked balance YAML, never a Go literal, never a Go constant |
| A **world** value: a world's seed, its generation inputs (**chunk size** — the radius R of a hexagonal chunk, where *ориентир* attaches to its reference value R = 9 and not to R being configurable — the algorithm weights, the growing-tree bias, and the island, extra-passage and portal shares), its gate-spacing parameter k, and its resource profile, naming style, lexicon and bestiary | **Configuration**, in the tracked **world set** — one YAML file per world, authored per world per `~/lab-private/DESIGN.md` §2.2 / §2.2.2. Not the balance file, and not among §16.5's balance numbers; never a Go literal, never a Go constant |
| A test fixture value | Inline in the test, named only when it aids reading |

## Determinism

Generation, combat and replay take an explicit seed and are pure: no `time.Now()`, no `math/rand` global, no dependence on map-iteration order. When output order matters, sort explicitly — Go randomises map iteration deliberately, and a test that passes today because of a lucky order is a future flake.

## Naming and shape

See [`go-api-naming.md`](go-api-naming.md). In short: no stutter, consumer-declared interfaces, `New…` constructors, `Err…` sentinels, `…Unchecked` for precondition-skipping variants.

## File size

Four bands on one axis, and only the top two are gated:

| Lines | What it means | Enforced by |
|---|---|---|
| 500 | Reasonable limit — the size a split is carried back to | Prose: author and reviewer |
| 800 | Soft: plan the split now | Prose: author and reviewer |
| 1000 | **Hard**, for a non-test `.go` file | Gated — `make file-limits` |
| 1500 | **Hard**, for a `_test.go` file | Gated — `make file-limits` |

Counting is raw lines, comments and blanks included; a `_test.go` file is governed by 1500 and never by 1000. Refactor before merge. Counter-rule: do not over-split — one type per file is not a Go idiom, and a package of ten 40-line files is harder to read than one 400-line file.

**The two hard bands have no per-file escape, deliberately.** There is no `//nolint` channel and no magic comment: an inline escape hatch on a hard limit is the thing that turns a hard limit soft. A generated tree, or a file that genuinely cannot be divided, is exempted by adding a path prune to the `file-limits` recipe in `Makefile`, in a reviewed diff — the same posture § *Linter posture* states for lint, one gate over.
