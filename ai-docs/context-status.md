# Implementation status — per task

The detailed, append-only implementation log: one entry per completed task, capturing the design decisions, traps and invariants worth not rediscovering. Written by `/task` Step 9.5, read on demand when touching the area an entry covers.

**This file grows; [`context.md`](context.md) does not.** `context.md` stays a thin orientation page — the per-task detail lives here.

Entry shape:

```markdown
## <issue or plan name> — <one-line outcome> (<PR #N>, <YYYY-MM-DD>)

- **What landed:** …
- **Decisions worth keeping:** …
- **Traps found:** …
- **Invariants this now relies on:** …
```

## Mechanical code-style gates from claude-alduna — one `make verify` every runner shares (PR #6, 2026-08-30)

- **What landed:** `asciicheck` + `gofumpt` in `.golangci.yml`; a tracked `Makefile` with ten `.PHONY` sub-targets and a `verify` aggregate; CI rewired so every Go job invokes a `make` sub-target, with the job matrix and `paths-filter` otherwise unchanged; a two-tier file-size gate (1000 non-test / 1500 `_test.go`) as one POSIX `awk` step; the `PostToolUse` formatting hook and the `PreToolUse` piped-gate guard both updated; a committed guard regression suite run by CI; and the two rule changes propagated to 10 files across 30 sites.

- **Decisions worth keeping:**
  - **The format gate is `golangci-lint fmt -d`**, not `gofmt -l .`. It exits 1 when any enabled formatter would rewrite a file, so it works as a check without post-processing its output. `gofumpt` needs no separate binary — it is a golangci-lint v2 built-in formatter.
  - **`make` matches as a CLASS in the piped-gate guard**, not by target enumeration. Enumeration leaks on six shapes that all run gates: five carry a flag between `make` and the target (`-s`, `-B`, `-C .`, `-j4`, `-f Makefile`), which breaks the `make[[:space:]]+<target>` anchor; the sixth is bare `make`, which leaks because it names no target for the anchor to bind to at all. The class form's only false positive is a dry run, which executes nothing — loud over-blocking beats silent under-blocking.
  - **No dry-run carve-out in that guard.** `tail -n 5` is the canonical spelling of `tail -5`, so exempting that flag would switch the guard off through the exact idiom it exists to catch. The carve-outs are whole-command greps, so any such exemption is a bypass, not a narrowing.
  - **The `gofmt` branch stays in the guard regex** even though `gofmt -l .` is no longer this project's gate — agents still reach for it by habit, so it remains must-block regression cover.
  - **File-size bands: 500 and 800 are prose, 1000 and 1500 are gated.** The source contradicts itself — alduna's `projectlint` flags mechanically at 500 (`internal/projectlint/projectlint.go`, `MaxLines = 500`) while alduna's prose (`docs/code-style.md`) calls 1000 the hard threshold. Resolved in favour of the prose by owner decision.
  - **Neither of alduna's `errcheck` loosenings crossed over.** Both would have weakened this repo's existing "never discard an error" rule (`code-style.md`). `asciicheck` was the whole linter delta; alduna's other linters are already active here via `linters.default: standard`.
  - **Every Makefile target is a gate**, which is what makes class-matching `make` semantically exact. An eleventh non-gate target was refused as drift bait — `go` stays enumerated in the guard precisely because `go list` and `go doc` are legitimately piped.

- **Traps found:**
  - **A pure-`revive` two-tier limit is impossible.** Two `file-length-limit` entries in one config silently collapse to the last one, and `exclusions` can disable a linter per path but not re-parameterise it. Hence one `awk` step owning both tiers.
  - **A fixture proving the file-limit gate must itself be gofmt-clean, or it proves nothing.** `verify` runs `fmt-check` first, so a dirty fixture reds at target 1 of 10 instead of at the gate under test. The naive fix — inserting a blank line — shifts the boundary fixtures by one, flipping 1000 to 1001 at exactly the boundary being proven. Reduce the filler by one to compensate.
  - **Proving a gate is load-bearing needs both halves:** red *at that gate* with the fixture present, and green with it removed. A red alone is satisfied by any red.
  - **`golangci-lint fmt` confines itself to path arguments**, despite a usage line that suggests otherwise. That is what makes the formatting-hook swap a one-call delta rather than a whole-module reformat.
  - **`golangci-lint-action@v9` needs `install-only: true`** when a job's `run:` is a `make` target — the action otherwise runs the linter itself. Its `core.addPath` precedes the early return, so the pinned binary is still on PATH.
  - **A guard regression suite must execute the hook body, not a copy of its regex.** Extracting the body with `jq` removes the drift class instead of documenting it. The suite was also run against the *pre-edit* body, where it fails on exactly 11 rows — proving it discriminates rather than passing anything put in front of it.
  - **The piped-gate guard matches command TEXT, not shell semantics.** It fires on any command merely quoting a gate piped into `tail`/`head`, including documentation of its own fixtures. Use the Write tool for such content.

- **Invariants this now relies on:**
  - `AGENTS.md` sits at **34 984 bytes** against its own 34 986 ceiling. Any edit to lines 47, 48, 77, 102 or 108 needs its byte arithmetic re-run *before* the edit lands — the five-line set balances only as a set, and two of the five grow while three pay for them.
  - `.claude/skills/task/SKILL.md` is **39 539 bytes** against a 40 000 hard cap.
  - `harness` is the only CI job whose `paths-filter` matches `.claude/**`, so a guard added there has nowhere else to live.
  - `Makefile` is present in the `go` paths-filter; without that entry a Makefile-only change silently skips every Go job.
  - The file-limit gate has no per-file escape hatch. The exemption channel is a reviewed path prune in the `file-limits` recipe, recorded in `code-style.md` — an inline escape on a hard limit is what turns a hard limit soft.

## Ledger core — `store.Post`, the owner/scope/account model and container-backed tests (PR #TBD-at-Step-12, 2026-09-02)

- **What landed:** `internal/store` (10 production Go files: `Migrate`, `NewPool` with the shopspring codec, `CreateOwner`, `Post`, enum and catalog mirrors, nine sentinels) and `internal/testdb` (testcontainers provisioning, `LAB_GAME_TEST_DSN` override, schema per test, never a skip). One forward-only goose migration creating 3 enums and 10 tables — `owner`, `scope_definition`, `account_definition`, `scope`, `account`, `account_balance`, `player_operation`, `manual_correction`, `journal_entry`, `posting` — with World and the player `attributes` catalog seeded. 33 test functions in 17 files, all against a real `postgres:18` container: the rapid property for the zero invariant, the `QueryTracer` phase/capture-order test, the `-race` anti-deadlock stress, FK-coverage and balance-row invariant checks, the catalog-mirror equality test. CI's `go` filter now matches `**/*.sql`.
- **Decisions worth keeping:** `kind` and `controlled` are catalog data (`account_definition`), never caller input — a posting is `(account, amount)` and "wrong kind on wrong account" is unrepresentable; accounts and zero balance rows are created with their owner, so every overdraft is refused by `CHECK (balance >= 0)` and `Post` needs one plain `UPDATE` per controlled account in ascending `account_id` (no upsert, no sign split); the exclusive arc and `ts` live on `journal_entry`, 1:1 with its basis document; singular table names; identity columns (`BY DEFAULT` where seeded, `ALWAYS` where only `Post` writes); every FK is index-covered, partial `IS NOT NULL` allowed, enforced by a migration test with a planted positive control; enums are add-only, one `ALTER TYPE` per migration file; images pinned to the major tag (`postgres:18`); `pgregory.net/rapid` for properties.
- **Traps found:** `INSERT … ON CONFLICT DO UPDATE` evaluates the `CHECK` on the *proposed* row, so an upsert refuses a covered debit with `23514` — the round-1 design had "measured" it on credits only; `pg_index.indkey` is zero-based (take the prefix `WITH ORDINALITY`); rootless podman's healthcheck never leaves `starting` (gate on `pg_isready`); the shell's `grep` is ugrep and `\|` inside `-E` is a literal pipe under GNU grep too — a grep-shaped AC needs a positive control, and its command cannot live in a markdown table cell (the cell escaping recreates the defect); `go mod tidy` prunes a `require` no package imports, so a dependencies-only subtask cannot commit on its own; a `.gitignore` pattern with an inner slash is root-anchored (`**/testdata/rapid/`); `decimal.New` takes an `int64` coefficient — 25-digit amounts are built from `New(r, 18)` plus a scale-5 fraction; `pgxdecimal.Register` prepends on every call (register once, in `AfterConnect`).
- **Invariants this now relies on:** every controlled account has exactly one `account_balance` row from creation (owner-creation code + the AC18 test) — `Post` treats zero rows affected as a broken invariant, never as overdraft; the Go mirrors of the three enums and two catalogs equal the database rows (test); no non-test statement in `store` targets `posting`/`journal_entry` with `UPDATE`/`DELETE` (grep with a planted control); `cmd/bot` never links testcontainers (`go list -deps` count 0); the test suite fails rather than skips when no database is reachable.
