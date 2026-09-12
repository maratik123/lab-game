# Code style — lab-game

The canonical, growing reference behind `AGENTS.md` § *Code Style*. Rules land here through the learning loop (`/improve`); the file is expected to start thin.

## Source files

Go only, under `cmd/<binary>/` (process entry points) and `internal/<package>/` (everything else). Nothing is exported for outside consumers — this module has none (`AGENTS.md` § API Stability), so `internal/` is the default home and `pkg/` is not used.

## Linter posture

`golangci-lint run` with `.golangci.yml`, strict. A finding is fixed, not silenced.

- `//nolint:<linter> // <reason>` — both parts mandatory (`nolintlint` enforces them). A bare `//nolint` fails the gate.
- A rule that keeps firing on correct code is a config bug: change `.golangci.yml` in a reviewed diff, never scatter suppressions.
- Enabled beyond the defaults, and why (KD-16): `exhaustive` (FSM/enum switches must be total), `rowserrcheck` + `sqlclosecheck` (a forgotten `rows.Err()` reads as an empty result, i.e. silent data loss), `errorlint`, `nilerr`, `bodyclose`, `noctx`, `contextcheck`, `gosec`, `revive`, `asciicheck` (identifiers stay ASCII; it checks identifiers only, so the Russian `docs/**` corpus is untouched).
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

`ctx context.Context` is the first parameter of anything that touches the database, the network, or the scheduler. Never store a context in a struct. Never pass `context.Background()` from inside a request path — thread the caller's.

## Concurrency

- The bot is a stateless handler over Postgres; **state lives in the database, not in memory** (KD-7/KD-8). A goroutine holding game state between updates is a design error, not an optimisation.
- Every goroutine has an owner that can stop it and a documented exit condition.
- `go test -race` is a required gate for any change that adds a goroutine or shared state.

## Database access

- One transaction per game operation, with its basis document and postings inside it ([`domain-invariants.md`](domain-invariants.md)).
- Balance `UPDATE`s go through `store.Post`, which owns the capture order; a handler that locks accounts by hand can deadlock and will be rejected in review.
- Always check `rows.Err()` after iterating (`rowserrcheck` enforces it); always close what you open (`sqlclosecheck`).
- Queries are parameterised. String-built SQL is a `gosec` finding and a security bug.

## Magic numbers vs balance constants

Two different rules, and conflating them is the common mistake:

| Kind of value | Where it belongs |
|---|---|
| A structural constant (the number of edges of a hex, a protocol limit) | A named Go constant next to the code that owns it |
| A **balance** value (stamina cap, step cost, timers, shop rates, door price curve, `budget(dist)`, combat dice) — and **chunk size**, which `docs/DESIGN.md` §2.2.2 calls «размер — конфиг» (*ориентир* attaches to its 16×16 reference number, not to the size) and which ships in `config/balance.yaml` as `world.chunk.cols`/`rows` | **Configuration**, per `docs/DESIGN.md` §16.5 — never a Go literal, never a Go constant |
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
