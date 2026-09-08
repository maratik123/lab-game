# Design: Comment reference ban — doc comments stay, outward references go

**Issue:** #68
**Date:** 2026-09-08

## Approach

The spec asks for work that only looks like one task: a **rule**, a **machine gate** for the
mechanically-decidable half of it, a **sweep** of every comment in the gated set, and the
**propagation** of the rule text into every instruction file whose claim the sweep falsifies. The
design keeps them separable, and orders them so that the gate exists before the sweep it
scopes, and is wired into the runners only after the code-side sweep has made the tree green.

The one genuinely hard decision is *how the gate reads a comment*. Everything else follows from it.

### D1 — The gate is a Go program, not a shell guard

The gate must decide, for every file of the gated set, which byte ranges are comments. That is a
lexing problem in every grammar of the gated set at once, and the tree already contains the shapes
that defeat a regex:

- Go string literals carry `//` — a URL inside a string reads as a comment to anything that scans
  for the marker
  [measured `72d5bf7:cmd/bot/main_test.go` § `validEnv` ·
  `git ls-files '*.go' | xargs grep -nE '"[^"]*//'` → among the hits,
  `"LAB_GAME_BOT_API_BASE_URL": "https://api.telegram.org",` ·
  `ast-index outline "cmd/bot/main_test.go"` → `validEnv [function]`]. AC5 exists because of exactly
  this.
- Shell heredoc bodies in the harness suites carry `#`-leading lines holding banned tokens
  [measured `72d5bf7:ai-docs/scripts/test-doc-edit-guard.sh:70-77` · `sed -n '60,90p' ai-docs/scripts/test-doc-edit-guard.sh` →
  `## Open questions")            # lands on the KD-2 cell mention`, inside a `<<'PY'` heredoc]. A
  `#`-anchored scanner reports that line, and no sweep can silence it without rewriting a fixture,
  which AC15 forbids.

`AGENTS.md` § *Dependency Versions* refuses "it's only 20 lines" as an argument for hand-rolling
what an established package already does. A heredoc- and quote-aware shell comment extractor **is**
a parser, and both the Go one and the shell one are available:

| Grammar | Extractor | Evidence it answers the question |
|---|---|---|
| Go | `go/scanner` with `ScanComments` (stdlib) | reports only real comments; the `//` inside `"https://…"` and inside a raw string are not reported; a block comment arrives as one multi-line token [measured `72d5bf7` · `go/scanner` probe in a scratch module under `tmp/`, since removed → the doc comment, the trailing comment and the block comment, and nothing from either string literal] |
| Shell | `mvdan.cc/sh/v3/syntax`, `KeepComments(true)` + `syntax.Walk` | heredoc bodies, `#` inside single and double quotes, `$#` and `${#a[@]}` are not reported; a trailing end-of-file comment is; the shebang **is** reported and therefore needs the KD-5 exemption [measured `72d5bf7` · same scratch module → only the shebang, the standalone comment, the trailing comment and the EOF comment] |
| YAML | `go.yaml.in/yaml/v3` node comments (already a direct requirement) [measured `72d5bf7:go.mod` · `grep yaml go.mod` → `go.yaml.in/yaml/v3 v3.0.5`] | recovers exactly the YAML-level comment lines a lexical `#` scan finds in `.golangci.yml`, and reports nothing from `ci.yml`'s `filters: |` block scalar — which is precisely the KD-6 boundary [measured `72d5bf7` · yaml.v3 probe over `.github/workflows/ci.yml`, `config/balance.yaml`, `.golangci.yml`] |
| SQL, `Makefile`, `.gitignore`, `.env.example` | small lexical scanners in the same package | the requirement here is a marker plus quoting — a scanner, not a grammar. **No package survey was run for this row's grammars**, so this is the design's weakest dependency call and the one to push on: it is argued from the shape of the requirement, not from an absence |

**Rejected alternative — a bash guard beside the existing ones.** It matches the shape of every
other gate in this repository and the spec's § *Conventions the gate inherits* names shell as what
guard scripts are here. It loses on the counts below, in order of weight. (a) It would hand-roll, in
awk, parsers the ecosystem maintains — the refused argument in `AGENTS.md` § *Dependency Versions*,
and the one the measurements above show is load-bearing rather than theoretical.
(b) **Its own fixtures cannot be written.** A regression suite for a reference gate needs fixtures
that *contain* banned references; in bash they land in heredocs, which is the exact construct the
naive extractor misreads, so the suite would flag itself and AC6 could not be met. In Go the
fixtures are string literals, and `go/scanner` does not report a string literal — the problem
disappears by construction. (c) The Go form inherits this project's test conventions for free:
table-driven subtests, the coverage ratchet, `go test` in CI.

What the shell convention actually asks for is kept in full: the gate is reachable through
`Makefile`, CI invokes the same target, it ships a regression suite CI runs, and it is itself a file
of the gated set and so must satisfy its own rule.

**Rejected alternative — a `go test` that walks the tree.** It would be the cheapest thing to
write, but AC2 is about *staged* content, and a test over the worktree cannot answer that question.
The command form answers both.

### D2 — Package and command layout

- `internal/commentref` — the extractors (D1), the file-class router, the banned-class classifier,
  and an exported entry point that takes named content and returns findings. Split across files so
  none approaches the hard line limit `Makefile`'s `file-limits` target enforces.
- `cmd/commentrefs` — a thin `main` over a testable `run(args []string, stdout, stderr io.Writer) int`,
  so the coverage ratchet sees the logic, not an uncoverable `main`. `cmd/bot/main_test.go` already
  establishes that a `cmd` package here carries a test file
  [measured `72d5bf7` · `git ls-files cmd/bot` → `cmd/bot/main.go`, `cmd/bot/main_test.go`].

### D3 — One new dependency: `mvdan.cc/sh/v3`

Latest release `v3.14.1`, and it brings nothing else with it — a scratch module requiring it
resolves to a `go.sum` naming only that module
[measured · `go list -m -versions mvdan.cc/sh/v3` → `… v3.14.0 v3.14.1`; scratch module `go get mvdan.cc/sh/v3@v3.14.1` →
`go: added mvdan.cc/sh/v3 v3.14.1`, its `go.sum` naming only `mvdan.cc/sh/v3`]. It is the parser
behind `shfmt`, and it is what the requirement in D1 names. Added with `go get` then `go mod tidy`,
never by hand-editing `go.mod`, per `AGENTS.md` § *Dependency Versions*.

### D4 — What the machine gate decides, and what stays review-judged

The gate reports a finding as `<file>:<line>: <class>: <matched text>` (AC4). The classes it
decides are the lexically unambiguous ones:

| Class the gate decides | Shape |
|---|---|
| `locator` | a path-shaped token followed by `:` and a decimal — with or without a source extension |
| `markdown-path` | a token ending in `.md` |
| `ac-id` | `AC` followed by a decimal, on a word boundary |
| `decision-anchor` | `KD-` or `D` followed by a decimal, on a word boundary |
| `section` | the `§` sign |
| `issue` | `#` followed by a decimal, after the `TODO(#…)` form has been masked out |
| `repo-path` | a token whose first segment is a top-level directory of this repository, or a token ending in a source extension of the gated set, or one of the repository-root file names |
| `url` | an `http` or `https` scheme marker |
| `module-symbol` | `<pkg>.<Exported>` where `<pkg>` is the name of a package of this module and is **not** the scanned file's own package (its `_test` suffix stripped) |

The gate does **not** decide the rest of the symbol class — a bare `Type.Field`, or a
package-qualified symbol from the standard library or a third-party module. A pattern that caught
those would fire on `time.Duration`, `errors.Is` and `context.Context` in contract prose, which are
the most common qualified names in this tree's comments
[measured `72d5bf7` · a probe over `git ls-files '*.go'` extracting `<lower>.<Upper>` occurrences from
comment text ranked `telego`, `errors`, `backoff`, `time`, `http`, `context` as the leading
qualifiers]. That half of AC1 is therefore verified the way AC16 is: by review, against the diff.
`module-symbol` is included because a project package name is unambiguous, and because a reference
into a sibling package of this module is the rot-prone case the owner's reason names.

This is a deliberate reading of AC2/AC3, and it is the one place where the design narrows an AC:
the gate refuses a banned reference **of a class it decides**. See `## Open questions`.

### D5 — Exemptions, applied before classification

- **A machine-read directive** (KD-5): a comment whose text begins with `go:`, `nolint:`, `+build`,
  `+goose`, `shellcheck`, or `!` (the shebang, which the shell parser reports as a comment — D1)
  has its directive token stripped; whatever human reason text follows is classified normally.
- **`TODO(#<issue>)`** (KD-10): masked out before the `issue` class runs, so a `TODO` keeps its
  owner and a bare issue number elsewhere in the same comment is still caught.
- **The comment's own subject** (KD-4) and **a same-package contract symbol** (KD-9) need no
  machine handling: the gate decides no unqualified symbol at all, and `module-symbol` compares
  against the file's own package clause.

### D6 — Modes, exit codes, and how pre-commit reads the staged content

`commentrefs` has the input modes below and one report format:

- no arguments — every tracked path of the gated set, read from the worktree;
- `--staged` — the staged, gated paths of the index (`A`/`C`/`M`/`R`), each read **from the index
  blob**, not from the worktree, so a partial stage is judged as it will be committed;
- explicit paths — read from the worktree, for a targeted local run.

Exit 0 = no finding. Exit 1 = at least one finding. Exit 2 = the gate could not run (an unreadable
file, a parse error, no git worktree) — an instrument failure that must not read as a clean tree,
the shape `check-citations.sh` already models for its own high-water mark
[measured `72d5bf7:.claude/skills/ai-audit/scripts/check-citations.sh:51,63` ·
`grep -n 'INSTRUMENT reading\|Instrument failure' .claude/skills/ai-audit/scripts/check-citations.sh` →
`# The high-water mark is an INSTRUMENT reading.` and `Instrument failure, not a citation finding`].

`git` is invoked with a fixed argument vector and no shell. `gosec` may still flag the subprocess;
if it does, the suppression is a specific `//nolint:gosec` with a stated reason, which is what
`nolintlint`'s `require-specific` and `require-explanation` settings demand
[measured `72d5bf7:.golangci.yml:42-48` · `cat -n .golangci.yml` → `nolintlint: require-explanation: true, require-specific: true`].

### D7 — Wiring: `Makefile`, CI, pre-commit

- `Makefile` gains a `comment-refs` target running the command over the whole tracked set, and
  `verify` gains it in its prerequisite list (AC9). No skill's `allowed-tools` line needs editing —
  the grants are wildcard
  [measured `72d5bf7` · `grep -rno "Bash(make[^)]*)" .claude/skills/*/SKILL.md .claude/agents/*.md` →
  `Bash(make *)` in `task/SKILL.md`, `pr-ci-failed/SKILL.md`, `project-review/SKILL.md` and their siblings].
- `.github/workflows/ci.yml` gains a **new filter key** naming every path class the gate covers —
  the Go, shell, SQL, YAML, `Makefile`, `.gitignore`, `.env.example` and `.githooks/**` classes —
  and a **new job** gated on it that sets up Go and runs `make comment-refs` (AC3). Neither existing
  filter key covers the set: the `go` key does not name `**/*.sh` and the `harness` key does not name
  `**/*.go`, and **neither names `.githooks/**`**
  [measured `72d5bf7:.github/workflows/ci.yml:37-61` · `cat -n .github/workflows/ci.yml`].
- `.githooks/pre-commit` becomes a symbolic link to a new `.githooks/pre-commit.sh` in the same
  directory (D8), and the dispatcher runs the reference gate over the staged set before `exec`ing
  the ratchet.

### D8 — The pre-commit dispatcher, and why it must be able to do nothing

Today `.githooks/pre-commit` is a regular file that `exec`s the ratchet
[measured `72d5bf7:.githooks/pre-commit` · `git ls-files -s .githooks/pre-commit` → `100755 c78cd82… .githooks/pre-commit`].
After the change the tracked entry is a symlink and the content lives in `pre-commit.sh`, which
keeps resolving its own paths from the worktree root — the property the existing dispatch suite
locks
[measured `72d5bf7:ai-docs/scripts/test-precommit-dispatch.sh:25,119-122` · `cat -n ai-docs/scripts/test-precommit-dispatch.sh` →
`[ -x .githooks/pre-commit ]` and `grep -q 'git rev-parse --show-toplevel' .githooks/pre-commit`].
Both assertions survive a symlink, because `grep` and `[ -x ]` follow one.

That suite builds a throwaway repository by copying `.githooks` into a sandbox, and `cp -r`
preserves a symlink on this toolchain
[measured · scratch probe, a directory holding `real.sh` and a symlink to it, `cp -r` → the copy
lists `link -> real.sh`]. So the sandbox inherits the new shape. But the sandbox has **no Go module
and no gated path staged**, and a dispatcher that unconditionally ran the gate there would refuse
every fixture commit and turn that suite red. The dispatcher therefore runs the gate only when
there is at least one staged gated path **and** the repository root carries a `go.mod`; otherwise it
says so on stderr and moves on — the same loud-skip direction the ratchet takes when `go` is
missing.

### D9 — One `--help` shape, applied identically (KD-17, AC19)

Placed immediately after the `set` line, before any other work, so `--help` can have no side effect:

```bash
usage() {
  cat <<'USAGE'
Usage:
  <the invocation grammar the removed prose carried, one line per form>
USAGE
}

case "${1:-}" in
  -h|--help) usage; exit 0 ;;
esac
```

`--help` prints to stdout and exits 0. A script that already refuses a wrong call keeps refusing it,
printing the same `usage` to stderr and exiting non-zero. `doc-edit-guard.sh` needs this shape
rather than a smaller edit, because its present `usage` reads its own comment block back out of the
file — an implementation the sweep destroys
[measured `72d5bf7:ai-docs/scripts/doc-edit-guard.sh:36` · `sed -n '1,45p' ai-docs/scripts/doc-edit-guard.sh` →
`usage() { sed -n '2,32p' "$0" | sed 's/^# \{0,1\}//'; exit 1; }`]. `coverage-ratchet.sh` already
parses `--check`, so it extends a `case` rather than introducing one.

The fixed line `  -h|--help) usage; exit 0 ;;` is the shape marker a verifier greps for.

### D10 — The `--help` membership set is derived twice, from two different trees

The set that owes a `--help` is "every tracked `*.sh` whose comments **carried** usage prose", and
after the sweep no comment carries any, so the criterion is unavailable in the post-sweep tree
`[derived → AC20]`. Therefore:

- the sweep and the Step-9 verifier each derive the set from the **merge base with `main`**
  (`git show <base>:<path>`), never from a list in this document, exactly as the spec requires;
- what survives forward is a new guard suite (`ai-docs/scripts/test-script-help.sh`) asserting the
  properties that *are* checkable against any later tree: a tracked `*.sh` whose comments carry
  usage prose must answer `--help`; every script that answers `--help` uses the D9 shape; `--help`
  exits 0, prints a non-empty block, and runs nothing else.

That suite is the design's answer to the spec's third open question. `AGENTS.md` sets the bar at
"any file with ~50+ lines of substantial logic"
[measured `72d5bf7:AGENTS.md` § *Workflow* · `grep -n '50+ lines' AGENTS.md` →
``- Any file with ~50+ lines of substantial logic MUST have tests (`_test.go` beside it).``]; the
suite clears it not by the flag's size but by the set's — the condition spans every script in the
tree and rots the moment one is added.

### D11 — What AC22 reaches, and what it does not

AC22 ("every instruction-file site that told a reader how to invoke one of those scripts now points
at `--help` or is removed") is read as: a site that **reproduces a script's invocation grammar** —
its modes, its argument order, its flags — replaces that grammar with a pointer to the script's own
`--help`, or drops it where it was incidental. A site that merely **names** a script — a row in a
table, an `allowed-tools` grant, a CI `run:` line, a "run this script" imperative carrying no
grammar — is untouched, because nothing in it can go stale that `--help` would fix. Markdown keeps
naming paths: the ban governs comments in code, and the spec is explicit that the two rules must not
be conflated.

### D12 — The sweep's contract is the gate's silence, not a site count

Each sweep subtask's completion condition is "`commentrefs <paths…>` reports nothing for this
subtask's paths, and the review-judged classes (D4) are clear in the diff". No per-file site tally
is written into this document: a tally is true for one commit, the implementor and the verifier
measure it anyway, and the gate is a better contract than a number, because it is re-runnable. The
truncating-gate caveat in this subagent's own rules — where "N sites" is a floor because the gate
stopped printing — is answered by the gate printing every finding and capping nothing
`[derived → the command scenarios in § Test Design]`.

Where a sentence exists only to carry a removed reference, the sentence goes with it; outside
`config/**` and `.env.example` the fact is lost, by the owner's answer. Note that the *failure
messages* these guard scripts print are not comments and are therefore untouched — a good deal of
the rationale the sweep strips from `check-ac-shape.sh`'s header survives in the message it prints
on a hit.

### D13 — `config/**` gains prose (AC11)

`config/balance.yaml` is today the only comment-bearing file under `config/` — the directory's only
other tracked entry cannot carry a comment
[measured `72d5bf7` · `git ls-files config/` → `config/balance.yaml`, `config/world/.gitkeep`] — and
its per-key documentation is largely a design-section pointer, in Russian
[measured `72d5bf7:config/balance.yaml:1-75` · `cat -n config/balance.yaml`]. Every **leaf** key ends
up with English prose that says what the key controls and what changing it does, at the
completeness `.env.example` reaches per variable; a group key gains prose when the group needs an
introduction. The placeholder status of the numbers is a fact about the file and survives, restated
without the issue number and without the design-section pointer that currently carries it. No value
changes.

### D14 — `.env.example` (AC12)

Its per-variable prose already meets the bar. What goes is the header's package name, test path,
markdown path and issue number
[measured `72d5bf7:.env.example:1-16` · `cat -n .env.example`], and the per-key pointers — the
`design D10` / `D13` / `D15` anchors and the `internal/config` and `GetUpdatesParams.Limit` symbols.
What stays, restated in its own words (KD-15), is the file's contract: a variable the configuration
loader does not read does not belong here. The cross-key constraint on the long-poll window stays —
it names a sibling key of the same file, which is not outward. The key set must still match the
loader's, which is asserted by an existing test
[measured `72d5bf7` · `git ls-files internal/config/disjoint_test.go` → present].

### D15 — Rule text and propagation (AC13, AC14)

`ai-docs/doc-convention.md` is rewritten, not retired (KD-11): § DOC-4 inverts from "cite the
section" to the ban, carrying the banned-class table, the exemptions, the record that the narration
half is review-judged (KD-13), and the record that shell in workflow `run:` blocks and in
`.claude/settings.json` hook bodies obeys the rule without a gate (Scope item 7). § DOC-3 and
§ DOC-5 stand.

The propagation class is not bounded by any list, and the design does not pretend otherwise: the
sweep runs `grep -rni` over `.claude/`, `AGENTS.md`, `ai-docs/` for each changed claim, per
`AGENTS.md` § *Propagation Rule* step 1. The members already located are the design's starting set,
not its boundary — they are enumerated in the decomposition row that owns them, and they include
the sites that **mandate** a banned reference
— `.claude/agents/self-review.md` § *4. Safety and correctness* and § *6. Documentation*,
`.claude/agents/review-findings.md` § *2. API design* and § *6. Documentation conformance*,
`.claude/skills/project-review/SKILL.md` § *Step 4: Final verify* (whose doc-convention item
additionally scans for the *wrong* shape of the citation the ban removes outright),
`ai-docs/go-api-naming.md` § *The `…Unchecked` AXIOM*, `AGENTS.md` § *API Naming*, and
`ai-docs/doc-convention.md` § DOC-3 and § DOC-4
[measured `72d5bf7` ·
`grep -rn "guarantees it\|§2.2.4\|never by line" .claude/agents/self-review.md .claude/agents/review-findings.md .claude/skills/project-review/SKILL.md ai-docs/go-api-naming.md AGENTS.md`,
each hit resolved to its own heading by `head -n <hit> <file> | grep -nE '^#{1,4} ' | tail -1`];
the sites that make a
claim about a comment the sweep rewrites (`AGENTS.md` § *Build & Test* on the ratchet header,
`ai-docs/code-style.md` § *Linter posture* on the `Makefile` header, `ai-docs/context-status.md` on
`.env.example`'s header), the sites naming `.githooks/pre-commit` as a regular file
(`.claude/skills/task/reference.md` § Step 2, `AGENTS.md` § *Build & Test*), and the declared sync
groups a new gate command and a new CI job trip (`ai-docs/propagation-groups.md` — the Review group,
the gate-command row, the `/task` verify-list row, the CI group's per-class reproducer tables, and
the `ci.yml`-job row; plus the generic obligation to update `ai-docs/claude-tools-hierarchy.md` for
a new tool contract).

One tension worth naming, because a reader will otherwise re-open it: the `…Unchecked` AXIOM
requires a doc comment to name the guarantor, and a guarantor in another package cannot be named
under the ban. The spec settles this — KD-9 and § *Source conflicts* item 4 keep both sections
unchanged — and it is not live today, because the tree holds no such function
[measured `72d5bf7` · `ast-index search "Unchecked"` → `No results found`; `git ls-files '*.go' | xargs grep -n Unchecked` → no output].
The design records the reading rather than reopening it: where a future guarantor is cross-package,
the comment states the precondition and describes the guarantor without a package-qualified symbol.

## Decomposition

| # | Task | Files | Depends on |
|---|------|-------|------------|
| 1 | Comment extraction: the file-class router and one extractor per grammar (Go, shell, YAML, SQL, `Makefile`, `.gitignore`, `.env.example`), each returning marker-stripped text with a line number; add the `mvdan.cc/sh/v3` requirement via `go get` + `go mod tidy` | `internal/commentref/` (extractors + tests), `go.mod`, `go.sum` | — |
| 2 | The banned-class classifier (D4) and the exemption pass (D5), over extracted comments | `internal/commentref/` (classifier + tests) | 1 |
| 3 | The command: the input modes and exit codes D6 names, the `<file>:<line>: <class>: <text>` report, a testable `run` with a thin `main` | `cmd/commentrefs/` | 2 |
| 4 | Sweep `cmd/bot`, `internal/config`, `internal/backoff` to the gate's silence (D12) | `cmd/bot/*.go`, `internal/config/*.go`, `internal/backoff/*.go` | 3 |
| 5 | Sweep `internal/store` and its migrations, and `internal/testdb` | `internal/store/*.go`, `internal/store/migrations/*.sql`, `internal/testdb/*.go` | 3 |
| 6 | Sweep `internal/tg` and `internal/tgtest` | `internal/tg/*.go`, `internal/tgtest/*.go` | 3 |
| 7 | Sweep `internal/ingest` and `internal/scheduler` | `internal/ingest/*.go`, `internal/scheduler/*.go` | 3 |
| 8 | Sweep the build and runtime gated files; clean the workflow's block-scalar comments by hand (cleaned, not gated — KD-6); give `coverage-ratchet.sh` the D9 `--help` | `.golangci.yml`, `.github/workflows/ci.yml`, `Makefile`, `.gitignore`, `.githooks/coverage-ratchet.sh` | 3 |
| 9 | `.env.example`: strip the pointers, restate the file-level contract in its own words (D14) | `.env.example` | 3 |
| 10 | `config/balance.yaml`: English self-contained prose per leaf key, values untouched (D13) | `config/balance.yaml` | 3 |
| 11 | Wiring: `Makefile` target + `verify`, the shellcheck target's special case removed; the CI filter key, the new job, and the new guard-suite line; `.githooks/pre-commit` → symlink, `.githooks/pre-commit.sh` dispatching gate-then-ratchet (D7, D8) | `Makefile`, `.github/workflows/ci.yml`, `.githooks/pre-commit`, `.githooks/pre-commit.sh` | 4–10 |
| 12 | Rewrite `ai-docs/doc-convention.md` to the new rule (D15, AC13) | `ai-docs/doc-convention.md` | 11 |
| 13 | Sweep the harness shell scripts; move usage prose behind the D9 `--help`; add the `--help` conformance suite; extend the dispatch suite for the symlink and the two-gate dispatch | `.claude/**/scripts/*.sh`, `ai-docs/scripts/*.sh`, `ai-docs/scripts/test-script-help.sh`, `ai-docs/scripts/test-precommit-dispatch.sh` | 11 |
| 14 | Propagate the rule text across the instruction surface and the hook messages, per D15 and the `grep -rni` sweep; AC22's grammar sites; the tool-hierarchy and propagation-group rows for the new gate, job and suite | `AGENTS.md`, `ai-docs/*.md`, `.claude/agents/*.md`, `.claude/skills/**/*.md`, `.claude/settings.json` | 12, 13 |
| 15 | Rewrite the #68 body to the reformulated rule (AC23) | issue #68 (no tracked file) | 14 |

### Which subtask owns which acceptance criterion

| AC | Owned by |
|---|---|
| AC1 | subtasks 4–10 and 13 (the classes the gate decides, plus the review-judged symbol half of D4) |
| AC2 | subtask 11 (the dispatcher), asserted by subtask 13's dispatch cases |
| AC3 | subtask 11 (the filter key and the job) |
| AC4 | subtask 3 (the report format) |
| AC5 | subtask 1 (the Go and SQL extractors) |
| AC6 | subtasks 1–3 — the gate is run over its own package as a completion condition |
| AC7 | subtask 11 (the symlink and the dispatcher), asserted by subtask 13 |
| AC8 | subtask 11 makes it true; a verifier's condition over the tree, with no gate behind it (see `## Open questions`) |
| AC9 | subtask 11 (`verify` reaches the target), held green by every subtask's own gate run |
| AC10 | subtasks 4–7 — `golangci-lint run` with `revive` unchanged is what refuses a removed doc comment, so the criterion is gated, not merely reviewed |
| AC11 | subtask 10 |
| AC12 | subtask 9 |
| AC13 | subtask 12 |
| AC14 | subtask 14 |
| AC15 | subtasks 4–8 and 13, under the reading in `## Open questions` |
| AC16 | subtasks 4–10 and 13, review-judged against the diff (KD-13) |
| AC17, AC18, AC19, AC21 | subtask 8 for `coverage-ratchet.sh`, subtask 13 for the harness scripts and the conformance suite |
| AC20 | subtasks 8 and 13, gated by the `repo-path` class once the prose moves behind `--help` |
| AC22 | subtask 14, under the reading in D11 |
| AC23 | subtask 15 |

## Handoff plan

Grouping is required for every `M ≥ 1` (a), a group holds at most `10` consecutive subtasks (b), the
handoff destination is `/context-reset` per `.claude/skills/context-reset/SKILL.md` § *Compaction
recovery (re-entry)* (c), the terminal group's size lies in `1..=10` (d), every group is homogeneous
by change-type (e), the group count is minimized subject to the cap and the dependency order (f),
each group is marked with its implementor model and effort (g), and the count is within the default
maximum of `4` (h).

**Change-type assignment used here.** *Code*: `*.go`, `*.sql`, and the build and runtime artefacts
the rule does not name — `Makefile`, `.github/**`, `.golangci.yml`, `.gitignore`, `.env.example`,
`config/**`, `.githooks/**`. *Instructions/harness*: `*.md`, `.claude/**` (its scripts and
`settings.json` included), `ai-docs/**` (its scripts included), `AGENTS.md`.

- **Handoff into Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § *Compaction recovery (re-entry)* before the first subtask, as the every-group contract requires.
- **Group A** — model `sonnet`, effort `medium` (pinned) via the `code-writer` subagent, 1M-token
  window — subtasks 1–3 (code change-type: `*.go`, `go.mod`, `go.sum`). The gate, built and tested
  before anything depends on it.
- **Handoff after Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § *Compaction recovery (re-entry)*. Parent `/task` resumes in Group B with fresh context.
- **Group B** — model `sonnet`, effort `medium` (pinned) via the `code-writer` subagent, 1M-token
  window — subtasks 4–11 (code change-type: `*.go`, `*.sql`, `.env.example`, `config/**`,
  `Makefile`, `.github/**`, `.githooks/**`). The code-side sweep, then the wiring, in that order:
  wiring first would make the pre-commit gate refuse the very commits that clean the tree.
- **Handoff after Group B:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § *Compaction recovery (re-entry)*. Parent `/task` resumes in Group C with fresh context.
- **Group C** — model `inherit` (the orchestrator's), effort inherited from the orchestrator
  (typically xHigh) — NOT pinned — via the `general-purpose` subagent with no inline `model=`,
  1M-token window — subtasks 12–15 (instructions/harness change-type: `*.md`, `.claude/**`,
  `ai-docs/**`, `AGENTS.md`, and the tracking issue's body). Terminal group (4 subtasks; within the
  `1..=10` range).

Group A and Group B are the same change-type and adjacent; they are two groups only because their
combined size exceeds the cap of `10`, so the count is minimized. Three groups, within the default
maximum of `4`, so no user approval is needed for the count.

## Risks

- The gate's own source flags itself: a doc comment on an exported classifier item that spells out
  what it matches ("matches a path ending in .md") is a banned reference in a gated file. Mitigation:
  the class names and patterns live in string constants and test fixtures, never in prose, and the
  gate is run over its own package as part of the subtask's completion condition —
  `[derived → AC6]`.
- Group C's commits are made with the pre-commit gate already wired (it lands in Group B), so a
  half-cleaned harness script cannot be committed. That is the gate working, but it makes Group C's
  commit granularity per-file rather than per-batch. Mitigation: stated here so the implementor
  stages what it has cleaned — `[derived → AC2]`.
- A YAML comment in a position `go.yaml.in/yaml/v3` does not attach to any node would be a silent
  false negative — the failure mode `AGENTS.md` § *Patterns 2* names, where a clean instrument reads
  as a clean subject. Mitigation: the extraction tests carry a fixture per comment position (head,
  line, foot, between mappings, after the last node, inside and after a block scalar), each asserted
  present or absent by name — `[derived → the YAML extraction cases in § Test Design]`.
- `mvdan.cc/sh/v3` fails to parse a script that `bash` accepts, and the gate exits 2 on a file it
  cannot read. Mitigation: exit 2 is an instrument failure by design (D6), not a pass; the extraction
  suite parses every tracked `*.sh` of the tree and asserts no parse error —
  `[derived → the shell extraction cases in § Test Design]`.
- The dispatch suite goes green for the wrong reason once the dispatcher can legitimately skip
  (D8): every fixture there stages `README.md` only
  [measured `72d5bf7:ai-docs/scripts/test-precommit-dispatch.sh` § `commit_from` ·
  `grep -n 'README.md' ai-docs/scripts/test-precommit-dispatch.sh` → every `git add` in the file
  names `README.md` and nothing else], so the reference gate never runs and the suite
  would pass with the dispatcher's gate call deleted. Mitigation: the suite gains its own instrument
  case — a sandbox commit that stages a gated file carrying a banned reference must be refused —
  `[derived → AC7 and the dispatch cases in § Test Design]`.
- `gosec` refuses the `git` subprocess in `--staged` mode. Mitigation: fixed argument vector, no
  shell; if the finding stands, a specific `//nolint:gosec` with a reason, which is what the lint
  config requires
  [measured `72d5bf7:.golangci.yml:42-48` · `cat -n .golangci.yml` → `nolintlint: require-explanation: true, require-specific: true`].
- The coverage ratchet blocks the commit because the new package arrived under-covered. Mitigation:
  D2 keeps `main` thin and the logic in `internal/commentref`, and the subtasks are TDD'd — the
  legitimate exits from a ratchet block are stated in `AGENTS.md` § *Build & Test*, and `--no-verify`
  is not among them — `[derived → AC9]`.
- The sweep strips rationale that a later reader will want and that no gate will restore. This is
  the owner's decided cost, not a defect; it is bounded by the fact that a script's printed failure
  messages are not comments and are untouched, and by `config/**` and `.env.example` gaining prose
  instead — `[derived → AC11, AC12, AC15]`.
- AC15 read literally forbids the wiring in subtask 11, which rewrites non-comment lines in
  `Makefile`, `ci.yml` and the pre-commit dispatcher. Mitigation: the reading in
  `## Open questions`, settled before Step 9 rather than at it — `[derived → AC15]`.
- CI's Harness-guards job shellchecks only what its `find` reaches, which is not `.githooks/**`,
  while `AGENTS.md` § *Build & Test* says "shellcheck on every script". That claim is already
  inaccurate and the symlink makes it more load-bearing. Mitigation: surfaced as a scope-boundary
  item in `## Open questions` rather than silently widened —
  [measured `72d5bf7:.github/workflows/ci.yml:150-153` · `cat -n .github/workflows/ci.yml` →
  `find .claude ai-docs/scripts -name '*.sh' -print0 | xargs -0 -r shellcheck -s bash`].

## Test Design

All of it is new, so every claim here is `[derived → …]`. Go tests are table-driven subtests beside
the code, per `ai-docs/go-test-conventions.md`; the shell suites follow the shape of the guard
suites already in `ai-docs/scripts/`.

**Comment extraction — `internal/commentref` (subtask 1).**
Entry point: the per-grammar extractor and the file-class router.
Scenarios, one table per grammar, each case a source snippet as a Go string literal and the expected
`(line, text)` list:
- Go: a doc comment; a trailing comment; a block comment spanning lines, each line reported with its
  own number; `//` inside an interpreted string; `//` inside a raw string; a comment on the same
  line as a string containing `//`; a file whose last line is a comment.
- Shell: a standalone comment; a trailing comment; a `#` inside single and double quotes; `$#` and
  `${#a[@]}`; a `#`-leading line inside a quoted heredoc and inside an unquoted one; a comment after
  the heredoc terminator; the shebang; a comment at end of file; and a case that parses every
  tracked `*.sh` in the tree and asserts no parse error.
- YAML: head, line and foot comments; a comment between two mappings; a comment after the document's
  last node; a `#` inside a quoted scalar; `#`-leading lines inside a `|` block scalar asserted
  **absent**; a comment on the line introducing the block scalar asserted **present**.
- SQL: `--` at line start and mid-line; `--` inside a single-quoted string; a `/* */` block; a goose
  annotation.
- `Makefile`, `.gitignore`, `.env.example`: line-start and mid-line `#`; a `#` in a recipe line; an
  escaped `\#` in `.gitignore` asserted **absent**.
- Router: each gated path shape maps to its extractor; a path outside the gated set is reported as
  not gated rather than silently skipped.
Fixtures: string literals in the test file, plus `git ls-files` for the whole-tree parse case.
Instrument check: for each grammar, a case whose expected list is deliberately non-empty beside a
case whose expected list is empty, so a router returning nothing cannot pass the table.
`[derived → AC5 and AC1]`

**Classification — `internal/commentref` (subtask 2).**
Entry point: the classifier over an extracted comment.
Scenarios: one positive and one negative case per class of D4; a comment carrying more than one
class, reporting each; the directive exemptions of D5 with a clean directive and with a directive
whose reason text carries a banned reference; `TODO(#N)` passing while a bare `#N` in the same
comment is caught; a same-package qualified symbol passing and a sibling-package one caught, with
the `_test` package suffix stripped; a standard-library qualified symbol passing, asserted with the
reason, so a later widening has to change a named case rather than a regex quietly.
`[derived → AC1 and AC4]`

**Command — `cmd/commentrefs` (subtask 3).**
Entry point: `run(args, stdout, stderr) int`.
Scenarios: a clean file exits 0 and prints nothing; a dirty file exits 1 and prints file, line, class
and matched text; an unreadable path exits 2; explicit-paths mode; whole-tree mode; `--staged` mode
driven against a scratch repository where the index and the worktree **disagree**, asserting the
index blob is what was judged.
Fixtures: a scratch repository built with `t.TempDir()` and fixed `git` invocations.
`[derived → AC2 and AC4]`

**`--help` conformance — `ai-docs/scripts/test-script-help.sh` (subtask 13).**
Scenarios over the tracked tree: every `*.sh` whose comments carry usage prose answers `--help`;
every script that answers `--help` carries the D9 shape marker verbatim; `--help` exits 0 and prints
a non-empty block. Sandbox fixtures: a conforming script, a script with usage prose and no `--help`,
a script answering `--help` with a different shape, and a script whose `--help` runs its body first
— each asserted to be flagged, which is this suite's instrument check.
`[derived → AC17, AC18, AC19, AC21]`

**Pre-commit dispatch — `ai-docs/scripts/test-precommit-dispatch.sh` (subtask 13).**
Added to the existing cases: the tracked entry is recorded as a symlink whose target is a `.sh` file
in the same directory; a sandbox commit staging a gated file with a banned reference is refused; a
sandbox commit staging a clean gated file is not; the existing ratchet-removed instrument case still
refuses. The refused-dirty-file case paired with the accepted-clean-file case is what stops this
suite from passing with the dispatcher's gate call deleted — `[derived → AC7]`.

**Everything the sweep touches (subtasks 4–10, 13).**
No new test. The completion condition is the gate's silence over the subtask's paths (D12) plus the
existing suites staying green — `make verify`, and for the harness scripts their own regression
suites, which must behave identically after their comments change.

## Open questions

- **AC15's scope.** Read literally, "every line removed or rewritten in the diff is a comment line"
  forbids subtask 11, which rewrites `Makefile`, `ci.yml` and the pre-commit dispatcher — all of them
  required by Scope items 5 and 6 of the same spec. The design reads AC15 as a constraint on **the
  sweep**: within a file whose only reason to appear in the diff is the sweep, only comment lines
  change. Files that the gate wiring, the symlink or the `--help` migration touch are outside it, as
  the AC's own carve-out for `config/**`, `.env.example` and the `--help` scripts already
  acknowledges. Confirm before Step 9, since it is a verifier's criterion.
- **AC2/AC3 versus what any lexical gate can decide.** The gate refuses a banned reference of a
  class it decides (D4); the bare-symbol half of the class list is review-judged, exactly as AC16's
  narration half is. This is a narrowing of the two ACs as written, and it is not a matter of effort
  — a pattern that decided the rest would fire on `time.Duration` in contract prose. Confirm the
  reading, or accept a gate that cannot be made green.
- **CI shellchecks less than `AGENTS.md` claims.** The Harness-guards job's `find` reaches
  `.claude` and `ai-docs/scripts` only, so `.githooks/**` is shellchecked locally by `make
  shellcheck` and not by CI, while `AGENTS.md` § *Build & Test* says CI covers every script. Fixing
  the filter is a small change and squarely out of this task's scope; flagged rather than
  absorbed. The same paragraph is worth a decision on whether
  `ai-docs/claude-tools-hierarchy.md` § *Shell guards* should also gain the rows it is missing for
  suites that already exist.
- **Whether `.githooks/pre-commit`'s symlink target should be gated by a check.** AC8 states a
  condition over the tree — every tracked file with a shebang either ends in `.sh` or is a symlink
  to one — with no gate behind it. The design leaves it as a verifier's condition; a mechanical form
  is cheap and can be added later without disturbing this task's gate.
