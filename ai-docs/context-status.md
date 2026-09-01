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
