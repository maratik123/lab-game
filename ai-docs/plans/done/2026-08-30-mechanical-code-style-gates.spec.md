# Mechanical code-style gates ported from a donor project

**Source:** user description (free-text entry)
**Date:** 2026-08-30
**Tracked in:** none — owner declined a tracking issue (round-2 approval); the repository has no issues, open or closed.

Port the **mechanical** half of the sibling project `<donor>`'s code-style
enforcement into lab-game: the linter delta, a stricter formatter, one Makefile
entry point that every runner (local, CI, hook) shares, and a hard file-size gate.
The prose half — idioms, rules expressed as guidance — is explicitly **not** part
of this task.

---

## Scope

1. **`.golangci.yml` — enable `asciicheck`, and nothing else.** The transferable
   delta from `<donor>/.golangci.yml` reduces to exactly one line
   [source: `<donor>/.golangci.yml:11`, under its own rationale comment at
   `:10` — `# ПРАВИЛО ПРОЕКТА: идентификаторы только ASCII.` ·
   `cat -n <donor>/.golangci.yml`]. Everything else the donor enables is
   either already active here or inapplicable, and its two `errcheck` loosenings
   were **rejected** by the owner (KD-11). Remove and disable nothing we already
   have, and add no exclusion. Row-by-row account: *Technical constraints § A*.

2. **`gofumpt`** — enable it in `.golangci.yml`'s `formatters` block and make the
   format gate actually enforce it (today's gate is plain `gofmt`, which is
   strictly weaker — see *Technical constraints § B*).

3. **`Makefile` with a `verify` target** — the single entry point that runs the
   full gate list of `AGENTS.md` § *Build & Test* (`AGENTS.md:42-53`):
   `go build ./...`, `go vet ./...`, `go test ./...`, `go test -race ./...`,
   `golangci-lint run`, a format check, `go mod tidy` + no-delta check,
   `actionlint` over the workflow files, `shellcheck` over every `*.sh`, plus the
   file-limit check of item 4. **CI is switched onto it** so that hook, CI and a
   local run cannot drift apart — keeping the existing `paths-filter` job matrix,
   with each Go/format/lint job invoking its own `make` sub-target (KD-10).

4. **File-limit gate — hard 1000 lines for a non-test `.go` file, hard 1500 for a
   `_test.go` file.** Minimal means only: a `revive` rule, an `awk` step in the
   Makefile, or a combination. **No new `cmd/` binary and no import of the donor's
   `projectlint` package.** Mechanism choice belongs to design; the enforceability
   limits discovered by probe are in *Technical constraints § C*.

5. **Doc-pointer sync, no growth.** `ai-docs/code-style.md` § *File size* carries
   the four-band scale and says which two bands are gated; `AGENTS.md`'s existing
   pointer lines are corrected where the new gates contradict them
   (`AGENTS.md:48`, `:49`, `:78`, `:103`, `:109` — **five** lines; they sat at
   `:47`/`:48`/`:77`/`:102`/`:108` until KD-18 inserted a sixth line above them).

   Two of the five need saying explicitly. `:103` — "format via `golangci-lint
   fmt` or `gofmt -w`, never by hand" — is a site the design's propagation sweep
   did **not** list; I found it in my own sweep (§ H). `:78` currently reads
   "**CI runs the same gates** … Each job is `paths-filter`-gated…"; the edit makes
   that sentence name the Makefile (KD-14). Both are **in-place replacements** —
   no line and no section is added — and the whole five-line set still lands on
   AC9's ceiling, measured in § H. `AGENTS.md` **must not grow** — see AC9.

6. **Propagate the two changed rules to every site that asserts the old one.**
   Authorised by the owner as a spec amendment (KD-12), because the `AGENTS.md`
   § *Propagation Rule* sweep reaches further than deliverables 1–5 did. Three
   families, all measured in § H:
   - **19 format-gate sites in 7 `.claude/**` files** still naming `gofmt -l .`
     (or "`gofmt` reports unformatted files") as *the format gate*. **Carve-out:**
     `allowed-tools:` lines and `.claude/settings.json`'s `permissions.allow` keep
     `gofmt` — the tool stays permitted, it merely stops being the gate.
   - **8 edit regions in `ai-docs/claude-tools-hierarchy.md`** (lines 17, 18, 23,
     the shell-guards table at 78–82, 84, 88, 94–97, 98 — enumerated and measured
     in § H). Not a judgement call: `AGENTS.md` § *Propagation Rule* names that file
     by row for any Tool/Subagent/Skill/Hook contract change, and KD-8 **and**
     Scope item 8 each change a hook contract.
   - **2 file-size band sites in 2 `.claude/agents/**` files**
     (`self-review.md:110`, `review-findings.md:86`), restated as the four-band
     ladder. **Each file's severity rubric stays intact** — `REJECT` in
     `self-review.md`, `major` in `review-findings.md`.

7. **`.claude/settings.json` — `permissions.allow` gains `Bash(make *)`.**
   Matching the file's existing granularity (`Bash(go *)`, `Bash(gofmt *)`,
   `Bash(git *)` — 17 entries today, verified with
   `jq -r '.permissions.allow[]?' .claude/settings.json`). Without it, `make verify`
   is an ASK on every invocation and **AC13 cannot be discharged in an unattended
   run**.

8. **Extend the `PreToolUse` piped-gate guard so it still covers this project's
   gates.** The 3rd `PreToolUse` entry in `.claude/settings.json` blocks a gate
   piped into `tail`/`head`, and `AGENTS.md:76` advertises it as the enforcement
   for that whole class. **After deliverables 2 and 3 it would stop covering the
   format gate, and it never covered `make`** — measured in § I. The alternation
   gains `golangci-lint[[:space:]]+(run|fmt)` and `make` **as a class** (KD-16).
   The two carve-outs — `--help`/`-h`, and `set -o pipefail` — are preserved
   **unchanged**. This is a Scope item rather than design-carried scope because
   without it the PR silently narrows enforcement, against KD-11's recorded
   posture that nothing this repo enforces today gets narrower (KD-15).

9. **Commit the guard's fixture matrix as a regression suite, and run it in CI.**
   The repo already runs two guard suites in the CI harness job
   (`.claude/skills/ai-audit/scripts/test-check-citations.sh`,
   `.claude/skills/task/scripts/test-append-task-run.sh`, at
   `.github/workflows/ci.yml:144-151`); this joins them, so a later edit to the
   regex that breaks a must-allow case fails CI instead of being discovered by a
   blocked agent. The suite asserts **both** directions (KD-17). Exact path and
   ownership are a design call; the spec requires only that it exists, that CI
   runs it, and that it passes `shellcheck -s bash`.

---

## Out of scope

Not to be done, not to be specced, not "while we are here":

| Item | Why |
|---|---|
| Coverage ratchet | Named out of scope by the owner. (`<donor>/Makefile:111-132` has one; it does not cross over.) |
| Porting prose rules and idioms from the donor | Separate task, owner-authored. |
| **Adding** a new agent, skill, or hook | Named out of scope by the owner, and it stays out: this change adds **no entry** to `.claude/settings.json`'s `hooks` block and creates no agent or skill — no new `.claude/agents/*.md`, and no new skill directory or `SKILL.md`. **Editing an existing** agent, skill or hook body is a different act and is explicitly *in* scope — KD-8 for the hook body, Scope items 6 and 7 for the rest, Scope item 8 for the guard regex. **A shell regression script is none of the three** (KD-17): Scope item 9's suite is a test fixture that joins the two guard suites CI already runs, exactly as `test-check-citations.sh` and `test-append-task-run.sh` are — it declares no `allowed-tools`, is never invoked as `/name`, and registers no hook event. It lands under `.claude/skills/**/scripts/` beside its siblings without making its parent a new skill. |
| Edits to `docs/DESIGN.md` | Named out of scope. |
| A bash code-line gate | `MaxShellCode = 10` exists in the source [source: `<donor>/internal/projectlint/projectlint.go:37` · `grep -n "MaxShellCode" <donor>/internal/projectlint/*.go`], but the owner's deliverable list names four items and a bash gate is not one of them. The 10-line ceiling still binds *this task's own shell* — see KD-7. |
| A markdown / prose size guard | the donor declines it on the record: "Сторожа на размер markdown НЕ ЗАВОДИМ" [source: `<donor>/docs/code-style.md:23-25` · `cat -n <donor>/docs/code-style.md`]. |
| the donor's `internal/query/` lint exclusion | Inapplicable: this repo has no `internal/` directory and no code generator. Verified — `find . -name '*.go' -not -path '*/.git/*'` returns exactly `cmd/bot/main.go`. |
| the donor's `errcheck` `exclude-functions: [fmt.Fprintf, fmt.Fprintln, fmt.Fprint]` (`<donor>/.golangci.yml:30-36`) | **Rejected by the owner, round 1 (KD-11).** It loosens a rule this repo already holds tighter: `ai-docs/code-style.md:20` — "Never discard: `_ = err` is a defect. If an error genuinely cannot be acted on, say why in a comment on the line that drops it." Do not re-propose. |
| the donor's `errcheck` exclusion `source: 'defer .*Close\(\)'` (`<donor>/.golangci.yml:43-45`) | **Rejected by the owner, round 1 (KD-11).** Same reason: a blanket source-regex exclusion removes exactly the per-site stated reason `ai-docs/code-style.md:20` requires. If a deferred `Close` genuinely cannot be checked, the channel is `//nolint:errcheck // <reason>`, which `nolintlint` already polices. Do not re-propose. |
| Removing the `python3` heredoc at `.github/workflows/ci.yml:169-182` | Pre-existing; the language decision (KD-7) constrains what *this* change adds, not what it must clean up. |
| `asciicheck`-equivalent coverage for SQL / TypeScript | the donor's `projectlint` does that for non-Go files; importing `projectlint` is forbidden here, and lab-game has neither surface yet. |

---

## Deferred

| What | Why | Separate issue needed? |
|---|---|---|
| Coverage ratchet | Owner-deferred; the donor's rationale for keeping it a separate target rather than a `make test` line is worth reading when it comes up (`<donor>/Makefile:111-126`). | Yes, when scheduled |
| Porting the donor's prose code-style rules | Owner will author it. | Owner-owned |
| A bash code-line gate (10 code lines, comments excluded) | The rule is real in the source but outside this task's four deliverables. | Optional |
| `.claude/agents/self-improve.md:255` prescribes `gofmt -- --check` | **Measured, not assumed:** `gofmt --help` lists only `-cpuprofile -d -e -l -r -s -w` — there is no `--check`. `gofmt --check` fails with `flag provided but not defined: -check` (exit 2), and the prescribed form `gofmt -- --check` fails with `lstat --check: no such file or directory` (exit 2), because `--` turns it into a path operand. So the step can never pass. Pre-existing defect found by the propagation sweep, in a file this task does not otherwise touch. Belongs to `/improve`. | Yes |
| `AGENTS.md:50` offers `go mod tidy && git diff --exit-code go.mod go.sum` | **Measured, not assumed:** in this repository `git diff --exit-code go.mod go.sum` exits **128** with `fatal: go.sum: no such path in the working tree`, because the module has no dependencies and `go.sum` does not exist — so the "hygiene gate" reports failure on a clean tree. Adding the `--` separator (`git diff --exit-code -- go.mod go.sum`) exits **0**; the repo already routes around it via `git status --porcelain` — the check now lives at `Makefile:52-59`, whose own comment records the move out of `.github/workflows/ci.yml`. Fixing `AGENTS.md:50` would *not* grow the file, but it is a distinct defect from this task's two rule changes. | Yes |

---

## Key decisions

| # | Question | Decision |
|---|---|---|
| KD-1 | What is the gated file-size threshold? | **1000 lines** for a non-test `.go` file, **1500** for a `_test.go` file. The 500 and 800 bands stay prose. Owner decision; full source-conflict record below. |
| KD-2 | What do the 500 / 800 bands mean, and where do they live? | 500 = *reasonable limit* — the target a split is carried to; 800 = *soft, plan the split now*. Both live in `ai-docs/code-style.md` prose, enforced by author and reviewer, never by a linter. [source: `<donor>/docs/code-style.md:29-30` · `cat -n <donor>/docs/code-style.md` — rows read `500 строк \| разумный предел` and `800 строк \| мягкий: пора думать о расщеплении`] |
| KD-3 | How are lines counted for the gate? | **Raw lines**, comments and blanks included — matching the source implementation, which counts `\n` bytes [source: `<donor>/internal/projectlint/projectlint.go:294` `func countLines(body []byte) int` · `grep -n "countLines" <donor>/internal/projectlint/projectlint.go`]. If `revive` is the mechanism, this means **not** passing `skipComments` / `skipBlankLines` — probe evidence in *Technical constraints § C*. |
| KD-4 | Which test-file names get the 1500 limit? | Files whose base name ends `_test.go`. The donor additionally matches `.test.` for TypeScript [source: `<donor>/internal/projectlint/projectlint.go:288` · `sed -n '286,292p' <donor>/internal/projectlint/projectlint.go`]; lab-game has no TypeScript, so `_test.go` alone. |
| KD-5 | Which linters cross over from the donor? | `asciicheck` only, among linters. The donor's `errcheck`/`staticcheck`/`govet`/`ineffassign`/`unused` (`.golangci.yml:15,19,20,21,22`) are already active here via `linters.default: standard` — verified: `golangci-lint help linters` prints exactly those five under "Enabled by default linters:". `nilerr`/`errorlint`/`bodyclose`/`gocritic` (`:16,17,25,27`) are already in our `.golangci.yml:14-37`. |
| KD-6 | What is the format gate command? | `golangci-lint fmt -d` — it exits **1** when any enabled formatter would rewrite a file, so it works as a check-mode gate without post-processing its output. Probe evidence in *Technical constraints § B*. A standalone `gofumpt` binary is not an option: `which gofumpt` reports it absent from PATH. |
| KD-7 | Which languages may this change introduce? | Go and bash only, bash capped at **10 code lines** per unit; python is barred from project artefacts (agents' own one-off tooling may use it). Owner decision; corroborated by the source [source: `<donor>/CLAUDE.md:99` "обвязка \| **bash**, и только минимальная \| всё, что длиннее десятка строк, переезжает в Go" and `:115` "**Go.** Не Python" · `grep -n -iE "bash\|python" <donor>/CLAUDE.md`]. This binds the Makefile recipes and any CI step this change writes. |
| KD-8 | Does the `PostToolUse` `gofmt -w` hook change? | **Yes.** The owner's stated reason for the Makefile is that hook, CI and a local run must not diverge, and the divergence is demonstrated (§ B): the hook currently runs plain `gofmt -w`, which leaves files that the new gate rejects. Its body becomes a `gofumpt`-equivalent through `golangci-lint fmt`. Editing an existing hook body is not "a new hook"; it does trigger the `AGENTS.md` § *Propagation Rule* obligation to re-verify per `ai-docs/hook-verification.md` and keeps the CI hook-body `shellcheck` step honest. |
| KD-9 | Does `Makefile` need a paths-filter entry? | **Yes — and KD-9 is why it now has one.** Before this change the `go` filter listed `**/*.go`, `go.mod`, `go.sum`, `.golangci.yml` and `.github/workflows/**`, and **no `Makefile`**: a Makefile-only change would have skipped every Go job, and `AGENTS.md:78` states that a job that did not run is not a passing job. The entry exists today at **`ci.yml:43`**. No line range is cited for the old state deliberately — this decision removed that state, so no range in the live file shows it. |
| KD-10 | How does CI adopt the Makefile? | **Per-job `make` sub-targets** — owner answer, round 1. The `paths-filter` job matrix stays exactly as it is (`changes` · Format · Build · Test · Lint · Harness guards · Actionlint), each job keeping its `if: needs.changes.outputs.* == 'true'` guard; the Go/format/lint jobs' inline `run:` commands become invocations of `make` sub-targets. `make verify` is the local aggregate of exactly those sub-targets. Parallelism and the `ci.yml:38-51` filter contract survive; the **Harness-guards job stays inline**, so its guards stay outside the Makefile — no Makefile-owned gate is routed through `make` there. (KD-10 decided the job's *relationship to `make`*, not that its contents are frozen: Scope item 9 appends one `bash` line to its existing `guard regression suites` step — the job keeps exactly seven steps — and AC8 states the boundary.) |
| KD-11 | Do the donor's two `errcheck` loosenings cross over? | **Neither** — owner answer, round 1. `asciicheck` is the whole crossover; `errcheck` keeps its current strictness, matching `ai-docs/code-style.md:20` exactly. **Nothing this repo enforces today gets narrower.** Both rejected settings are recorded in *Out of scope* with their reason so they are not re-proposed later. |
| KD-12 | How broad is the propagation of the two changed rules? | **Full** — every site that asserts the old rule, authorised **through a spec amendment** rather than left to the design. Owner answer, round 3, verbatim: *"Full, но через spec-amendment."* The reason the owner gave the amendment route: a design must not carry scope the spec never granted. Scope items 6 and 7 are that grant; § H is the measured site list. |
| KD-13 | Does the harness get a new permission entry? | **Yes — `Bash(make *)`** in `.claude/settings.json`'s `permissions.allow`. Owner answer, round 3. Scoped to match the file's existing per-tool granularity, not broadened. |
| KD-14 | Does `AGENTS.md:78` get edited so `AGENTS.md` names the Makefile? | **Yes — "Add it back."** Owner answer, round 4. The design had dropped this edit when it reconciled to the amended spec, correctly: Scope item 5 enumerated four lines and `:77` was not among them, so carrying it would have been design-side scope the spec never granted. The cost the design named, and the owner's reason for reinstating it: **`AGENTS.md` is the one file every agent loads on every invocation, and a Makefile no agent can discover there is a Makefile that will not be used** — precisely the hook/CI/local divergence this task exists to close. The edit is an in-place rewrite of the existing sentence, adding no line and no section; its byte cost is measured in § H. |
| KD-15 | The guard stops covering this project's gates — fix now or defer? | **Fix in this PR**, as a Scope item with ACs rather than design-carried scope. Owner decision, round 5. The regression is silent and self-inflicted: deliverable 2 renames the format gate to a command the regex does not match, and deliverable 3 introduces `make`, which it never matched. Shipping the PR without this would narrow enforcement while KD-11 records that **nothing this repo enforces today gets narrower** — the same posture, applied to the harness instead of to `errcheck`. |
| KD-16 | Does `make` match as a class, or by target enumeration? | **As a class** (`make` + `\b`), not `make[[:space:]]+(verify\|lint\|…)`. Owner decision, round 5. **The rationale is the load-bearing part: the two candidates fail in opposite directions.** Enumeration fails by **silent under-blocking** — precisely the harm the hook exists to prevent — because the `make[[:space:]]+(verify\|lint\|…)` anchor binds only when a target name follows `make` immediately; I measured six shapes that all run a real gate and all leak (§ I) — **five** where a flag intervenes (`-s`, `-B`, `-C .`, `-j4`, `-f Makefile`), and one, **bare `make`**, that names no target for the anchor to bind to at all. Class-matching fails by **loud over-blocking** of a command that runs no gate. A guard whose failure mode is a visible refusal is strictly safer than one whose failure mode is a green record of a red gate. **Why `make` may be class-matched while `go` stays enumerated:** every target in the designed Makefile is a gate — the design refused a non-gate `fmt` target as drift bait — whereas `go list` and `go doc` are legitimately piped and must stay allowed. |
| KD-17 | Where do the fixtures live? | **In a committed regression suite that CI runs**, not only in an AC-time matrix. Owner decision, round 5. A matrix checked once at merge cannot fail a later regex edit; a committed suite can. It joins the two guard suites already in the CI harness job. Recorded in the *Out of scope* carve-out row so it does not read as contradicting "no new agent, skill, or hook": a test script is none of the three. |
| KD-18 | `make file-limits` is a new gate command — how is leg 1 of its propagation satisfied? | **Raise AC9's ceiling and add the line plainly**, rather than contort the wording to stay byte-neutral. Owner decision. `ai-docs/propagation-groups.md:16` makes adding a gate command a **three-legged** propagation: `AGENTS.md` § *Build & Test* **AND** every skill's `allowed-tools` line that grants it **AND** `.claude/skills/task/reference.md` § *Gate checklist*. Self-review round 3 found only the third leg closed. Leg 1 is this decision: `make verify` became the first line of the § *Build & Test* code block, comment column 57 preserved, costing **87 B** (86 + newline) and taking `AGENTS.md` from 34 984 to **35 071** [measured: `wc -c AGENTS.md`]. **The consequence, recorded plainly rather than engineered around:** the file now sits in the **35 000–39 999** early-warning band, which `AGENTS.md`'s own size table calls a `minor` — "extraction pass **owned by the next `/ai-audit` run**; NOT a criterion of any `/task`" [verified verbatim at `AGENTS.md:73`]. It stays far below the 40 000 hard cap. **Leg 2 is closed too, in this PR.** It briefly stood at seven of eight: `.claude/skills/task/SKILL.md` was withheld because the design's risk row said "nothing else may enter that file in this PR" as a byte-budget guard. The owner approved a **Design Amendment** lifting that guard for exactly this one grant, and it shipped. Measured at the branch head: `task/SKILL.md` is **39 552 B** — the grant cost exactly **13 B**, leaving **448 B** under the 40 000 cap — and the set of skills granting `Bash(make *)` is now **identical** to the eight granting `Bash(golangci-lint *)` (verified by `diff` of the two `grep -rl` file lists over `.claude/skills/*/SKILL.md` → empty). All three legs of `propagation-groups.md:16` are therefore satisfied. **No AC asserts this**: the grant landed under an owner-approved amendment, and writing a criterion afterwards to match work already done inverts the order. |

---

## Technical constraints

### A. The `.golangci.yml` delta, line by line

Present in `<donor>/.golangci.yml`, absent from `.golangci.yml`
[both read with `cat -n`]:

| Item | donor source | Applicable here? |
|---|---|---|
| `asciicheck` | `.golangci.yml:11`, under the comment `# ПРАВИЛО ПРОЕКТА: идентификаторы только ASCII.` (`:10`); corroborated by `<donor>/CLAUDE.md:366`, which maps "идентификаторы ASCII (Go)" to `golangci-lint` / `asciicheck` | **Yes — and it is the entire crossover.** Checks identifiers only, so the Russian `docs/**` corpus is untouched. Verified green on the current tree (§ G). |
| `settings.errcheck.exclude-functions: [fmt.Fprintf, fmt.Fprintln, fmt.Fprint]` | `.golangci.yml:30-36`; rationale comment at `:31-32` — output to stderr is not checked, because if stderr is broken there is nowhere left to report it | **No — rejected (KD-11).** It loosens `errcheck` relative to `ai-docs/code-style.md:20` ("Never discard: `_ = err` is a defect"). |
| exclusion `linters: [errcheck]` + `source: 'defer .*Close\(\)'` | `.golangci.yml:43-45`; rationale at `:40-42` — in a deferred call there is nowhere to put the error, and the exclusion is deliberately narrow | **No — rejected (KD-11).** Same reason as the row above. |
| exclusion `path: internal/query/` for `errcheck`/`gocritic`/`staticcheck`/`unused` | `.golangci.yml:48-53` | **No.** No such path; no generator. |

Already covered here, do not re-add: `errcheck` `:15`, `staticcheck` `:19`, `govet` `:20`,
`ineffassign` `:21`, `unused` `:22` (via `linters.default: standard`); `nilerr` `:16`,
`errorlint` `:17`, `bodyclose` `:25`, `gocritic` `:27` (explicit in our `.golangci.yml:14-37`).

Our config currently enables **26** linters [`golangci-lint linters`, section
"Enabled by your configuration linters:"]. Nothing in that list may be removed or
disabled by this change, and **no exclusion may be added**: after this change the
`exclusions.rules` block still holds exactly the one `_test.go` rule it holds
today (`.golangci.yml:50-57`).

### B. `gofumpt` is strictly stronger than the gate this repo ran before — demonstrated

Probe in the scratchpad, on a file that is gofmt-clean but has a blank line after
`func B() {`:

```
$ gofmt -l .                     # → no output, exit 0
$ golangci-lint fmt -d           # → exit 1, prints the diff removing that blank line
```

Consequences, all of them facts rather than predictions. **They are written in
the past tense because this PR fixed every one of them** — the baseline is the
evidence for the change, not a description of the tree today:

- The CI `Format` job **ran** `gofmt -l .`, and would have passed code the new
  gate rejects. No line number is cited: the job now runs `make fmt-check`, and
  the stronger present fact is that **`gofmt` appears nowhere in `ci.yml` at all**
  [measured: `grep -c gofmt .github/workflows/ci.yml` → `0`].
- The `PostToolUse` `Write|Edit` hook **ran** `gofmt -w "$f"` on every written
  `.go` file, so it would have kept re-introducing violations — the divergence
  KD-8 closes. It now runs `golangci-lint fmt` [measured:
  `jq -r '.hooks.PostToolUse[].hooks[].command' .claude/settings.json`].
- `AGENTS.md:48` **described** `golangci-lint fmt` as "apply formatters (gofmt +
  goimports)" and `AGENTS.md:49` **offered** `gofmt -l .` as the format check.
  Both were false once `gofumpt` was enabled, and both were corrected in place as
  existing-pointer fixes, not new content (§ H's byte table).
- `golangci-lint formatters` currently lists `gofumpt` under "Disabled by your
  configuration formatters:"; it is bundled, so nothing needs installing.

### C. What `revive`'s `file-length-limit` can and cannot express — four probes

Run against a two-file scratch module with `golangci-lint run ./...`
(golangci-lint 2.13.1, go1.26.4):

| Probe | Config | Result |
|---|---|---|
| 1 | `max: 5`, `skipComments: true`, `skipBlankLines: true` | Fires on the test file only; message reads `file length is 7 lines` for a 10-line file — **the skips change the number being compared**. |
| 2 | `max: 5`, no skip arguments | Fires on both files; a 7-line file reports `file length is 7 lines` — **raw line counting**, which is what KD-3 wants. |
| 3 | probe 2 plus `exclusions.rules: [{path: _test\.go, linters: [revive], text: file-length-limit}]` | The `_test.go` finding disappears, the non-test finding remains — **per-path suppression of one revive rule works**. |
| 4 | two `file-length-limit` entries with `max: 5` and `max: 9` | Only `max: 9` is applied, silently — **a rule cannot be instantiated twice with two limits**. |

Therefore a **pure-`revive`** solution can enforce one tier only. Two tiers
(1000 non-test / 1500 test) require either an `awk`-style step covering both, or
`revive` at 1000 with `_test.go` excluded plus a second mechanism for 1500.
Design chooses; a proposal that asks `revive` alone for both numbers is
unimplementable.

### D. Makefile mechanics

- Each recipe line runs in its own shell, and `make`'s default shell is `/bin/sh`,
  where `set -o pipefail` is not portable. A gate whose exit status is
  load-bearing must therefore not be piped, or the Makefile must set
  `SHELL := /bin/bash` — see `AGENTS.md:76` ("A zero exit status is evidence about
  the LAST pipeline stage").
- No recipe may swallow a failure (`|| true`, a trailing `@`-silenced pipeline).
- KD-7 caps any recipe's shell at 10 code lines.
- Gate logs belong at the gitignored names `/gate.log` or `*.gate.log`
  [source: `.gitignore:12-14` · `cat .gitignore`]; `/bot` and `/bin/` are
  already ignored build output.

### E. Tool availability

| Tool | Local | ubuntu-latest runner |
|---|---|---|
| `golangci-lint` 2.13.1 | yes | installed today by `golangci/golangci-lint-action@v9` pinned to `v2.13.1` (`ci.yml:106-109`) |
| `shellcheck` | `/usr/bin/shellcheck` | preinstalled |
| `actionlint` | `~/go/bin/actionlint` | **not** preinstalled — CI uses `reviewdog/action-actionlint@v1` (`ci.yml:191-194`) |
| `gofumpt` standalone | **absent** (`which gofumpt` → not found) | n/a — reached through `golangci-lint` |
| `make`, `awk` | `/usr/bin/make`, `/usr/bin/awk` | preinstalled |

Under KD-10 this costs nothing for `actionlint`: the Actionlint job keeps
`reviewdog/action-actionlint@v1` and is not one of the jobs that shells out to
`make`; `make actionlint` stays the local path only.

**The Lint job is the one that needs care.** `golangci/golangci-lint-action@v9`
runs `golangci-lint` itself, so a job whose `run:` step is `make lint` needs the
pinned binary on `PATH` first. The action supports exactly that — its inputs
include `install-only` ("Only install golangci-lint. It does not run
golangci-lint.") and `install-mode` ("It can be 'binary', 'goinstall', or
'none'") [source: `golangci/golangci-lint-action` `action.yml:21-27` at ref `v9` ·
`gh api "repos/golangci/golangci-lint-action/contents/action.yml?ref=v9" --jq '.content' | base64 -d`].
Design picks between that and a hand-rolled install; the `version: v2.13.1` pin at
`ci.yml:108` must survive either way, so that CI and this machine lint with the
same binary.

### F. Current repo inventory

- Exactly one Go file: `cmd/bot/main.go`. `go.mod` declares
  `module github.com/maratik123/lab-game`, `go 1.26`, and zero requirements.
- Five `*.sh` files, all under `.claude/skills/**/scripts/`.
- No `Makefile` exists yet (`ls -la Makefile` → no such file).
- `AGENTS.md` was **34 986 bytes** when this spec was first written. It is
  **35 071** today (`wc -c AGENTS.md`), because KD-18 added a `make verify` line to
  § *Build & Test*. It therefore now sits **inside** the 35 000–39 999
  early-warning band that `.github/workflows/ci.yml:152-165` enforces — an
  owner-approved consequence, not a regression. See KD-18 and AC9.

### G. The baseline is already green under both new settings

Probed on the real repository: `.golangci.yml` temporarily patched with
`asciicheck` added to `linters.enable` and `gofumpt` added to
`formatters.enable`, then restored (`git diff --stat -- .golangci.yml` empty
afterwards).

```
golangci-lint run ./...   → exit 0, "0 issues."
golangci-lint fmt -d      → exit 0, no diff
```

So deliverables 1 and 2 cause **no code churn**: `cmd/bot/main.go` already
satisfies `asciicheck` and `gofumpt`. A design that budgets work for fixing new
findings is budgeting for none — the value of both is preventive.

### H. The propagation sweep — measured, not transcribed

The site list below came to this spec as the design's claim. I re-ran the sweep
myself with `grep -rn "gofmt" .claude/ ai-docs/ AGENTS.md CLAUDE.md .github/` and
`grep -rniE "1500|excluding .*_test|excl\./incl" .claude/ ai-docs/ AGENTS.md`, and
read each cited line. **All 25 of the design's claimed sites resolve to lines that
really do assert the old rule** — the counts 19 / 4 / 2 are confirmed, not
adjusted.

**Family 1 — 19 format-gate sites in 7 files.** Each names `gofmt -l .`, or
`gofmt reports unformatted files`, as *the format gate*:

| File | Lines | Count |
|---|---|---|
| `.claude/skills/main-ci-failed/SKILL.md` | 154, 168, 263, 315, 390 | 5 |
| `.claude/skills/pr-ci-failed/SKILL.md` | 160, 174, 271, 357 | 4 |
| `.claude/skills/task/reference.md` | 201, 267, 302 | 3 |
| `.claude/agents/code-writer.md` | 65, 67, 84 | 3 |
| `.claude/skills/pr-commented/SKILL.md` | 204, 325 | 2 |
| `.claude/skills/task/SKILL.md` | 188 | 1 |
| `.claude/skills/project-review/SKILL.md` | 106 | 1 |

**Family 2 — 8 regions in `ai-docs/claude-tools-hierarchy.md`.** It was 4 when
this table was first written; Scope items 8 and 9 made four more stale. I counted
them on the live 107-line file rather than adopting the design's figure, and
arrive at the same **8**:

| # | Region | Why it goes stale |
|---|---|---|
| 1 | line **17** | The piped-gate guard row enumerates what it blocks — "`go build/test/vet` / `golangci-lint run` / `gofmt`". Scope item 8 adds `golangci-lint fmt` and `make`, so the enumeration becomes incomplete. |
| 2 | line **18** | The `PostToolUse` `gofmt` hook row, whose contract KD-8 changes. |
| 3 | line **23** | "All nine bodies pass `shellcheck -s bash`; the blocking ones were exercised … on 2026-08-29" — wrong count (§ H note 2 below), and **two** hook bodies now change, so the date needs re-verifying. |
| 4 | lines **78–82** | The shell-guards table. Scope item 9's suite needs a row beside `check-citations.sh`, `test-check-citations.sh`, `append-task-run.sh`, `test-append-task-run.sh` and `cleanup-progress.sh`. |
| 5 | line **84** | "**Both** suites must pass `shellcheck -s bash` and run green before `git add`" — becomes three. |
| 6 | line **88** | The `permissions.allow` tool inventory, which Scope item 7 changes. |
| 7 | lines **94–97** | The CI job table — Format/Build/Test/Lint rows, all of which change shape under KD-10. |
| 8 | line **98** | The Harness-guards CI row says "both guard suites"; it becomes three. The job gains **no step** — the third suite is one more `bash` line inside the existing `guard regression suites` step. |

**One conditional ninth, for the design to resolve:** line **62** describes
`/ai-audit` as shipping "two shell guards — `check-citations.sh` and its
regression test". That stays true **only if** Scope item 9's suite lands outside
`ai-audit/scripts/`. If the design puts it there, line 62 becomes a ninth region.
Path choice is the design's (Scope item 9); this spec only flags the dependency.

**Family 3 — 2 file-size band sites:** `.claude/agents/self-review.md:110`
(`REJECT` rubric) and `.claude/agents/review-findings.md:86` (`major` rubric).
Both currently read "hard limit (1000 lines excluding `_test.go` content / 1500
including)" and "soft limit (500 / 800)" — the excl./incl.-tests framing of
*Source conflict 2*. A repo-wide sweep found **no third site**: the only other
occurrences are `AGENTS.md:109` and `ai-docs/code-style.md:62`, already covered by
Scope item 5.

#### Three things my sweep found that the design's list does not carry

1. **`AGENTS.md:103` is a 20th format site.** It reads "format via
   `golangci-lint fmt` or `gofmt -w`, never by hand". That is the *apply* form, not
   the gate form, so it fell outside the design's `gofmt -l .` pattern — but
   `gofmt -w` under a gofumpt-enabled config produces exactly the files the new
   gate rejects, which is KD-8's divergence in prose. Now in Scope item 5.
2. **`ai-docs/claude-tools-hierarchy.md:23` says "All nine bodies"; there are
   ten.** Measured: `jq -r '.hooks[][].hooks[].command' .claude/settings.json`
   yields 10 lines (SessionStart 2, PreToolUse 3, PostToolUse 4, Stop 1).
   Pre-existing drift, inside a region this task already edits — correcting it is
   free and in-region, so it is folded into AC17 rather than deferred.
3. **`.claude/skills/task/SKILL.md` is 39 529 bytes — 471 below the 40 000 hard
   cap** that `.github/workflows/ci.yml:152-165` enforces over the 37 files its
   `find` matches. Replacing one `gofmt -l .` with `golangci-lint fmt -d` costs
   ~10 bytes, so it fits — but nothing else may be added to that file in this PR.
   Headroom for the rest: `pr-ci-failed` 25 290, `main-ci-failed` 24 689,
   `task/reference.md` 33 478, `project-review` 9 881, `pr-commented` 25 076,
   `code-writer` 12 604, `self-review` 25 978, `review-findings` 14 260 bytes.
   `ai-docs/claude-tools-hierarchy.md` (11 145 bytes) is **outside** that CI
   `find` and outside the `ai-docs/{…}.md` list in the AGENTS.md AXIOM, so it
   carries no cap.

#### The `AGENTS.md` byte budget across all five lines — measured

**Implemented, and re-derived against the commit itself.** These are no longer
predictions: I diffed `git show 37a37f8^:AGENTS.md` against the live file, so each
figure is the actual before/after of the five in-place replacements. The file was
**34 986 bytes** before and is **34 984** after, with the line count unchanged at
**292** — no line added or removed, so every downstream `AGENTS.md:<line>` pointer
still resolves.

| Line | Before | After | Delta |
|---|---|---|---|
| 48 (was 47) | 94 | 87 | −7 |
| 49 (was 48) | 100 | 96 | −4 |
| 78 (was 77) | 412 | 426 | **+14** |
| 103 (was 102) | 120 | 106 | −14 |
| 109 (was 108) | 145 | 154 | +9 |
| **Five-line total** | | | **−2** → **34 984** |
| **+ KD-18's sixth line** | — | 86 | **+87** (86 B + newline) |
| **Running total** | | | → **35 071** |

**Line 77 carries two substitutions, not one — which is where earlier versions of
this table went wrong.** It read `+16`, then `+15`; the measured truth is `+14`. A
character-level diff of the line resolves it:

| # | Substitution | Bytes |
|---|---|---|
| 1 | the parenthetical gains `` , Go via `make` `` | **+15** |
| 2 | `both guard suites` → `the guard suites` | **−1** |

The second is a **Scope item 9 consequence landing inside line 77**: once that
item's regression suite joins the two already running, "both" is simply false, so
the count word had to go. Six substitutions across the five lines in total
(1 + 1 + 2 + 1 + 1). **My re-derivation agrees with the design
(`…design.md:131`) in every digit**, and with the live file.

**AC9's bound moved, deliberately and once — see KD-18.** It is now `≤ 35 100`, and
35 071 satisfies it with 29 bytes to spare. AC9 is a **bound,
not a target** — it is not restated as an equality and not tightened, per
`AGENTS.md` § *Communication* ("A bound is not a target… tightening a rule nobody
asked to tighten is unapproved scope exactly as loosening one is"). The
operational note still binds anyone editing these lines later: two of the five
grow (`:78` +14, `:109` +9) and three shrink to pay for them, so **the five-line
set only balances as a set** — any rewording of a growing line needs its byte
arithmetic re-run *before* the edit lands.

**All five line numbers shifted +1 when KD-18 inserted the sixth line at 41.**
Every `AGENTS.md:<line>` citation in this spec has been re-resolved against the
live file; a pre-KD-18 reference to `:47`/`:48`/`:77`/`:102`/`:108` maps to
`:48`/`:49`/`:78`/`:103`/`:109`. The byte figures are unaffected: a line that
moves does not change size.

**An earlier version of this section said a `make verify` menu entry "was never
affordable — ~87 B overruns the ceiling on its own". The measurement was right
and the conclusion is now obsolete:** the line costs exactly **87 B** (86 + the
newline), and KD-18 raised the ceiling to fit it rather than contorting the
wording to avoid it.

#### Two bookkeeping notes, surfaced rather than silently fixed

1. **AC9 has two halves, and only one of them was ever frozen — now resolved.**
   Its parenthetical had drifted out of step with Scope item 5: it listed three
   lines while Scope item 5 grew to five (`:103` joined in round 3, `:78` in
   round 4). The **binding half of AC9 is the ceiling**, `wc -c AGENTS.md`
   — `≤ 34 986` then, `≤ 35 100` since KD-18 — satisfied by the table above. The round-4 amendment said
   *do not change AC9*; at `round == round_cap` I could not ask whether that bar
   covered the enumeration half as well, so I left the row untouched and flagged
   the contradiction rather than resolving it unilaterally. The owner's answer:
   the bar was aimed at the **bound** — the ceiling must not be tightened,
   loosened, or rewritten as an equality — and not at the line list. **AC9's
   parenthetical now names the same five lines as Scope item 5**; the ceiling,
   its number, and the "gains no new section, table, or bullet" clause are
   byte-identical to before.
2. **This spec is 61 893 bytes, which is not a cap violation.** The 40 000-char
   instruction-file cap covers `AGENTS.md`, `CLAUDE.md`, `.claude/rules`,
   `.claude/agents`, `.claude/skills` (the CI step's own `find`, verified: it
   matches 0 paths under `ai-docs/plans`) plus the five named
   `ai-docs/{code-style,doc-convention,context,agent-writing-style,corrections-log}.md`
   pages. `ai-docs/plans/**` is in neither set — specs and designs are task
   artefacts, not per-invocation instruction files.

#### Sites deliberately NOT changed — do not "fix" these

| Site | Why it stays |
|---|---|
| `allowed-tools:` lines in `bugfix`, `context-reset`, `dependabot-pr`, `main-ci-failed`, `pr-ci-failed`, `pr-commented`, `project-review`, `task` SKILL.md | Permission grants, not gate assertions. `gofmt` stays a permitted tool. |
| `.claude/settings.json:115` `"Bash(gofmt *)"` | Same carve-out. |
| `.claude/rules/ast-index.md:23` ("a `gofmt`-split signature") | Unrelated sense — line-splitting, not formatting gates. |
| `.claude/skills/bugfix/SKILL.md:216`, `:236` (`golangci-lint fmt`) | The *apply* form, which picks up `gofumpt` automatically once it is enabled. It gets stronger with no edit. |
| the `gofmt` alternative **inside** the piped-gate regex (`.claude/settings.json:54`) | Not the regex as a whole — Scope item 8 **does** edit it. What must not happen is *deleting* the `gofmt` branch: `gofmt -l .` stops being this project's gate, but the guard still has to catch it, because agents and imported harness prose reach for it by habit. Scope item 8 only **adds** alternatives (`golangci-lint fmt`, `make`); it removes none, and AC21 keeps `gofmt -l . \| tail -5` in the must-block set as regression cover. |

### I. The piped-gate guard — measured, both candidates, both directions

The live regex, read from `.claude/settings.json` rather than retyped
(`jq -r '.hooks.PreToolUse[].hooks[].command'`, third entry):

```
(^|[ ;&|`(])(go[[:space:]]+(build|test|vet)|golangci-lint[[:space:]]+run|gofmt)\b[^;]*\|[[:space:]]*(tail|head)\b
```

with two carve-outs that exempt a command outright: `(^|[[:space:]])(--help|-h)([[:space:]]|$)`
and `(^|[;&|[:space:]])set[[:space:]]+-o[[:space:]]+pipefail`. The proposed class
form changes exactly one group:

```
… |golangci-lint[[:space:]]+(run|fmt)|gofmt|make)\b …
```

Verdict = BLOCKED when the main regex matches **and** neither carve-out applies.
All three columns below are my own runs, not carried figures.

| Command | LIVE | ENUM | CLASS |
|---|---|---|---|
| `golangci-lint fmt -d \| tail -5` | ALLOWED | BLOCKED | **BLOCKED** |
| `make verify \| tail -5` | ALLOWED | BLOCKED | **BLOCKED** |
| `make lint \| head -30` | ALLOWED | BLOCKED | **BLOCKED** |
| `make test \| tail -5` | ALLOWED | BLOCKED | **BLOCKED** |
| `make \| tail -5` (bare = default target) | ALLOWED | **ALLOWED** | **BLOCKED** |
| `make -s verify \| tail -5` | ALLOWED | **ALLOWED** | **BLOCKED** |
| `make -B lint \| tail -5` | ALLOWED | **ALLOWED** | **BLOCKED** |
| `make -C . verify \| tail -5` | ALLOWED | **ALLOWED** | **BLOCKED** |
| `make -j4 test \| tail -5` | ALLOWED | **ALLOWED** | **BLOCKED** |
| `make -f Makefile verify \| tail -5` | ALLOWED | **ALLOWED** | **BLOCKED** |
| `go test ./... \| tail -5` | BLOCKED | BLOCKED | BLOCKED |
| `gofmt -l . \| tail -5` | BLOCKED | BLOCKED | BLOCKED |
| `golangci-lint run \| tail -5` | BLOCKED | BLOCKED | BLOCKED |
| `go build ./... \| head -20` | BLOCKED | BLOCKED | BLOCKED |
| `go vet ./... \| tail -3` | BLOCKED | BLOCKED | BLOCKED |
| `set -o pipefail; make verify \| tail -5` | ALLOWED | ALLOWED | ALLOWED |
| `make --help \| head` | ALLOWED | ALLOWED | ALLOWED |
| `go list ./... \| head -20` | ALLOWED | ALLOWED | ALLOWED |
| `golangci-lint linters \| head -20` | ALLOWED | ALLOWED | ALLOWED |
| `git log --oneline \| head -20` | ALLOWED | ALLOWED | ALLOWED |
| `go test ./... > gate.log 2>&1 && echo GATE-RED` | ALLOWED | ALLOWED | ALLOWED |
| `grep -E "^(FAIL\|ok)" gate.log \| head -5` | ALLOWED | ALLOWED | ALLOWED |
| `cmake --build . \| tail -5` | ALLOWED | ALLOWED | ALLOWED |
| `echo makezero \| tail -1` | ALLOWED | ALLOWED | ALLOWED |

The six **ALLOWED-under-ENUM** rows are the leak KD-16 turns on: every one runs a
real gate, and every one defeats the `make[[:space:]]+<target>` anchor — but **not
all for the same reason**, and the split is five-plus-one. **Five** carry a flag
between `make` and the target (`-s`, `-B`, `-C .`, `-j4`, `-f Makefile`), so what
follows the anchor's `[[:space:]]+` is a flag rather than a target name. The
**sixth is bare `make`**, which leaks because it names **no target at all** — the
default target runs, and there is nothing for the anchor to bind to; § I's table
labels that row "bare = default target" for exactly this reason. The distinction
sharpens the mechanism without touching KD-16's decision: bare `make` leaks under
enumeration either way. The last two rows are mine, added to confirm the class form does not fire
on a word merely *containing* `make` — `\b` after the alternation and the
`(^|[ ;&|`(])` prefix together rule out `cmake` and `makezero`.

#### The one disagreement, resolved in favour of the measurement

The amendment's draft must-ALLOW set listed **`make -n verify | head`**. It does
not belong there: **the class form BLOCKS it**, as my run above's methodology
confirms and as the amendment itself suspected. I record the measured verdict
rather than the aspirational one — this is the accepted, *known* false positive
that KD-16's rationale already names, not an allowance. It is a dry run that
executes nothing and fails loudly, which is the cheap side of the asymmetry.
**It therefore appears in AC23 as a documented false positive, and NOT in AC22's
must-allow set**, where asserting it would make the suite unsatisfiable.

#### Why the `-n` carve-out was rejected — measured, and worse than expected

Sparing `make -n` by adding a `-n` carve-out fails badly, because the carve-outs
are **whole-command greps**: any `-n` anywhere in the command exempts all of it.

| Command | as designed | with a `-n` carve-out |
|---|---|---|
| `make -n verify \| head` | BLOCKED | ALLOWED |
| `golangci-lint run \| grep -n foo \| tail -5` | BLOCKED | **ALLOWED** |
| `go test ./... \| grep -n FAIL \| tail -5` | BLOCKED | **ALLOWED** |
| `make verify \| tail -n 5` | BLOCKED | **ALLOWED** |

The last row is the decisive one and goes beyond the hazard the amendment named:
**`tail -n 5` is the canonical spelling of `tail -5`**, so a `-n` carve-out would
exempt the single most common piped-gate shape there is — the guard would be
disabled by the very idiom it exists to catch. Rejected.

---

## Source conflicts

### Conflict 1 — the donor's own hard file-size threshold: 500 vs 1000

Two sites of the named source disagree.

- **Site A — the linter.** `<donor>/internal/projectlint/projectlint.go:30-31`
  [`sed -n '25,40p' <donor>/internal/projectlint/projectlint.go`]:

  ```go
  const (
  	MaxLines     = 500
  	MaxTestLines = 1500
  ```

  and `:286-291` [`sed -n '286,292p' …`] applies `MaxTestLines` when the base name
  ends `_test.go`, otherwise `MaxLines`. The preceding comment (`:27-29`) itself
  labels 500 differently from the constant's use: "Порог — РАЗУМНЫЙ предел, а не
  жёсткий" — *a reasonable limit, not a hard one* — while the code enforces it as
  the only limit there is.

- **Site B — the prose.** `<donor>/docs/code-style.md:27-32`
  [`cat -n <donor>/docs/code-style.md`], verbatim:

  ```
  | порог | что значит |
  |---|---|
  | 500 строк | разумный предел |
  | 800 строк | мягкий: пора думать о расщеплении |
  | 1000 строк | жёсткий |
  | 1500 строк | жёсткий для файлов с тестами |
  ```

  `:39-40` reinforces it: "если файл перерос 1000 строк, он делится по доменной
  границе" — and `:48-52` states the split is carried to 500, "не до жёсткого".

**Resolution: in favour of the prose (Site B).** The gate is 1000 / 1500; 500 and
800 remain ungated prose bands.

**Who chose it:** the product owner, stated in this task's description and already
recorded in this repository's history — commit `a870415`
("style: restore the four-band file-size scale, gate only the hard limits"),
whose message reads *"The linter enforces 1000/1500; 500 (reasonable) and 800
(plan the split) stay in prose because a judgement a linter enforces stops being
one."* [`git log -1 --format='%s%n%b' a870415`]. That commit lives on the
abandoned branch `chore/2026-08-29-codestyle-from-<donor>` and is **not** an
ancestor of `main` (`git merge-base --is-ancestor a870415 main` → non-zero), so
its content is history, not a live rule — it is cited here as the record of the
decision, not as an implementation to copy. Its approach (a Go package under
`internal/harness/rules/`) is in fact forbidden by this task's deliverable 4.

This is settled input, **not** an open question.

### Conflict 2 — lab-game's own docs describe the bands as an excl./incl.-tests pair

- `AGENTS.md:109` [`awk 'NR==108' AGENTS.md`]: "**File size:** soft 500/800, hard
  1000/1500 lines (excl./incl. tests); exemptions and the don't-over-split
  counter-rule are in `code-style.md`."
- `ai-docs/code-style.md:62` [`cat -n ai-docs/code-style.md`]: "Soft 500 / hard
  1000 lines excluding `_test.go` content (800 / 1500 including it)."

Both read the four numbers as two pairs split by *whether test content counts*.
The scale they came from is a **single four-band ladder on one axis**, with only
the top two gated and 1500 belonging specifically to `_test.go` files (Site B
above, plus KD-1/KD-4). Resolution: bring both sentences onto the four-band
reading — Scope item 5. No new AGENTS.md prose; the line is rewritten in place
within its existing budget (AC9).

---

## Acceptance Criteria

| # | Criterion |
|---|-----------|
| AC1 | `.golangci.yml` enables `asciicheck` and changes nothing else in `linters`: `golangci-lint linters` lists **27** enabled linters — the 26 enabled before this change plus `asciicheck`. The `exclusions.rules` block is unchanged from its pre-change content (the single `_test.go` rule at `.golangci.yml:50-57`), and neither rejected donor `errcheck` setting appears anywhere in the file (KD-11). |
| AC2 | `.golangci.yml`'s `formatters.enable` includes `gofumpt` alongside `gofmt` and `goimports`; `golangci-lint formatters` lists `gofumpt` under "Enabled by your configuration formatters:". |
| AC3 | The format gate rejects a file that plain `gofmt` accepts: on a fixture that is `gofmt -l`-clean but gofumpt-dirty, the repository's format gate exits non-zero. |
| AC4 | A tracked `Makefile` exists with a `.PHONY` `verify` target that runs, each with a load-bearing exit status: `go build ./...`; `go vet ./...`; `go test ./...`; `go test -race ./...`; `golangci-lint run`; the format check; `go mod tidy` followed by a no-delta check on `go.mod`/`go.sum`; `actionlint` over `.github/workflows/*.yml`; `shellcheck` over every `*.sh` in the tree; the AC6 file-limit check. `make verify` exits 0 on the branch head. |
| AC5 | No step of `make verify` can report green while its gate is red: no recipe pipes a gate into another command without `pipefail` in force, and no recipe ends in `\|\| true`. Demonstrated for the **new** gate (AC6) by a temporary over-limit fixture that turns `make verify` red and is then removed. |
| AC6 | The file-limit gate is exactly two-tier: a non-test `.go` file of **1001** raw lines fails and one of **1000** passes; a `_test.go` file of **1501** fails, **1500** passes, and **1001** passes (test files are governed by 1500, never by 1000). Counting is raw lines, comments and blanks included (KD-3). |
| AC7 | The gate is implemented without a new `cmd/` binary, without any new Go package, and without importing or vendoring the donor's `projectlint`. |
| AC8 | `.github/workflows/ci.yml` keeps its `paths-filter` job matrix: the jobs `changes`, `format`, `build`, `test`, `lint`, `harness` and `actionlint` all still exist, each non-`changes` job still guarded by `if: needs.changes.outputs.* == 'true'`. Every gate the Makefile owns is invoked in CI **through** `make` — no gate's command text exists independently in both files. The `harness` job keeps everything KD-10 decided for it: it stays inline, keeps its own `harness` paths-filter, routes **no** Makefile-owned gate through `make`, and **keeps exactly seven steps** — none added, removed, or reordered. The **one** permitted edit inside those steps is a **single `bash` line appended to the existing `guard regression suites` step**, invoking Scope item 9's suite after the two that already run there, so all three guard suites stay grouped in the one step where the existing two live. It lands in this job because it has nowhere else to go — `harness` is the only job whose filter matches `.claude/**` (verified: `go` matches `**/*.go`, `go.mod`, `go.sum`, `.golangci.yml`, `Makefile`, `.github/workflows/**`; `workflows` matches `.github/workflows/**`), so a `.claude/settings.json`-only edit fires no other job. `Makefile` is present in the `go` paths-filter at `ci.yml:38-43`. `actionlint .github/workflows/ci.yml` passes. |
| AC9 | `wc -c AGENTS.md` after the change is **≤ 35 100** — raised from `≤ 34 986` by KD-18, and a **bound, not a target**: the measured result is **35 071**, and the criterion is not restated as an equality. AGENTS.md gains no new section, table, or bullet — KD-18's addition is a single line *inside* the existing § *Build & Test* code block, and the other five changes are in-place replacements of existing pointer lines (`:48`, `:49`, `:78`, `:103`, `:109` as needed). |
| AC10 | `ai-docs/code-style.md` § *File size* states all four bands with their enforcer: 500 reasonable (prose), 800 plan-the-split (prose), 1000 hard non-test (gated), 1500 hard `_test.go` (gated). `grep -rni "excl" AGENTS.md ai-docs/code-style.md` returns no line that still describes the four numbers as an excluding-tests / including-tests pair. |
| AC11 | The `PostToolUse` `Write\|Edit` formatting hook no longer diverges from the format gate: after it runs on a gofumpt-dirty `.go` file, the format gate is green. `jq -r '.hooks[][].hooks[].command' .claude/settings.json` still parses, each body still passes `shellcheck -s bash`, and the hook is re-verified per `ai-docs/hook-verification.md`. |
| AC12 | This change adds no python to any tracked project artefact and no shell unit longer than 10 code lines. The pre-existing `python3` heredoc at `.github/workflows/ci.yml:169-182` is untouched. |
| AC13 | `make verify` is green on the branch head, and every gate it runs is green when run standalone. |
| AC14 | `make verify` runs exactly the gate list of Scope item 3 — no more, no less — and every CI job that reaches a gate through `make` uses one of `verify`'s own sub-targets, so a local `make verify` and a full CI run cannot disagree about what any gate's command is. The two gates CI still reaches by another route (`reviewdog/action-actionlint@v1`, and the Harness-guards job's inline `shellcheck`) are named as deliberate exceptions in a Makefile comment (KD-10). |
| AC15 | All **19** format-gate sites of § H Family 1 assert the new gate: `grep -rn "gofmt -l" .claude/ --include='*.md'` returns nothing, and `grep -rn "gofmt reports unformatted files" .claude/ --include='*.md'` returns nothing. Each edited line still reads as a gate instruction in its surrounding step, not as a dangling command. **The `--include='*.md'` scoping is load-bearing, not cosmetic — do not "tighten" it away.** AC15's claim is that no instruction-file *prose* still directs an agent to check format with `gofmt -l .`. The scoping deliberately excludes exactly one file, `.claude/skills/ai-audit/scripts/test-piped-gate-guard.sh`, whose line 81 carries the fixture `BLOCK⟶gofmt -l . \| tail -5` that **AC21 requires it to carry** as regression cover (KD-16). A fixture asserting the guard still *blocks* that command is the opposite of prose recommending it. Unscoped, AC15 and AC21 cannot both hold — satisfying AC21 breaks AC15's literal grep. The design's equivalent sweep (§ *Test Design* → *Documentation checks*) already carries `--include='*.md'` for the same reason. |
| AC16 | `AGENTS.md:103` no longer offers `gofmt -w` as a way to format Go source. The carve-out holds: `grep -rn "Bash(gofmt \*)" .claude/` still finds every `allowed-tools:` line and the `permissions.allow` entry — the tool stays permitted. |
| AC17 | `ai-docs/claude-tools-hierarchy.md` is updated at all **eight** regions enumerated in § H Family 2: the piped-gate guard row (17) enumerates what the extended regex blocks, `golangci-lint fmt` and `make` included; the `PostToolUse` hook row (18) describes that hook's new body; the verification line (23) names the correct body count — **ten**, verified by `jq -r '.hooks[][].hooks[].command' .claude/settings.json \| grep -c .` — and a re-verification date from this change; the shell-guards table (78–82) has a row for Scope item 9's suite; line 84 and the Harness-guards CI row (98) say **three** suites, not "both"; the `permissions.allow` inventory (88) lists `make`; the CI job table (94–97) shows each job invoking its `make` sub-target. If the suite lands under `ai-audit/scripts/`, line 62's "two shell guards" is a ninth region and is updated too. |
| AC18 | `.claude/agents/self-review.md:110` and `.claude/agents/review-findings.md:86` state the four-band ladder (500 reasonable · 800 plan-the-split · 1000 hard non-test · 1500 hard `_test.go`) instead of an excl./incl.-tests pair, and **each keeps its own severity rubric** — `REJECT` in `self-review.md`, `major` in `review-findings.md`. `grep -n "excluding" .claude/agents/self-review.md .claude/agents/review-findings.md` returns no file-size line. |
| AC19 | `jq -r '.permissions.allow[]?' .claude/settings.json` includes `Bash(make *)`, and the file still parses (`jq . .claude/settings.json > /dev/null`). No other `permissions` entry is added, removed, or broadened. |
| AC20 | No instruction file crosses its cap: for every file in the CI cap set, `wc -c` is < 40 000, and `.claude/skills/task/SKILL.md` specifically stays < 40 000 (it starts at 39 529). `AGENTS.md` still satisfies AC9's ≤ 35 100 (KD-18). |
| AC21 | **Must BLOCK.** The extended guard blocks every one of these (regex matches, neither carve-out applies): `golangci-lint fmt -d \| tail -5`; `make verify \| tail -5`; `make lint \| head -30`; `make test \| tail -5`; `make \| tail -5`; `make -s verify \| tail -5`; `make -B lint \| tail -5`; `make -C . verify \| tail -5`; `make -j4 test \| tail -5`; `make -f Makefile verify \| tail -5`; and, as regression cover for what the live regex already blocks, `go test ./... \| tail -5`, `gofmt -l . \| tail -5`, `golangci-lint run \| tail -5`, `go build ./... \| head -20`, `go vet ./... \| tail -3`. |
| AC22 | **Must ALLOW.** The extended guard leaves every one of these untouched: `set -o pipefail; make verify \| tail -5`; `make --help \| head`; `go list ./... \| head -20`; `golangci-lint linters \| head -20`; `git log --oneline \| head -20`; `go test ./... > gate.log 2>&1 && echo GATE-RED`; `grep -E "^(FAIL\|ok)" gate.log \| head -5`; `cmake --build . \| tail -5`; `echo makezero \| tail -1`; `make verify` (no pipe). **Both directions are required** — AC21 alone is satisfiable by a regex that matches everything. |
| AC23 | The two carve-outs are **byte-identical** to their pre-change text: `(^\|[[:space:]])(--help\|-h)([[:space:]]\|$)` and `(^\|[;&\|[:space:]])set[[:space:]]+-o[[:space:]]+pipefail`. No `-n` carve-out is added (§ I). The one known false positive — `make -n verify \| head`, which the class form **BLOCKS** — is recorded as such in the guard's own comment or the regression suite, stating that it is a dry run that executes nothing and fails loudly. It is deliberately **absent** from AC22. |
| AC24 | A committed regression suite asserts **both** AC21 and AC22 and is run by the CI harness job alongside the two existing guard suites; it fails non-zero when any must-block case is allowed **or** any must-allow case is blocked (demonstrated once by inverting a single fixture, which is then restored). It passes `shellcheck -s bash`, and `.github/workflows/ci.yml` invokes it. Its fixtures are derived from the guard's live regex, not a copy that can drift silently — or, if copied, the suite fails when the copy and `.claude/settings.json` disagree. |

---

## Open questions

- **Exemptions the gate cannot grant.** `ai-docs/code-style.md:62` promises an
  exemption for a generated file or one long `switch`/state machine. Neither a
  `revive` rule nor an `awk` step has a per-file exemption channel other than
  `//nolint:revive // <reason>` (which `nolintlint` already polices) or a path
  exclusion. Design should pick one and say so in the prose; not blocking.
- **`make verify` runtime.** With one `.go` file it is trivial today. If the
  aggregate ever becomes slow enough that people stop running it, split a fast
  subset out — a judgement call for whoever first feels it, not a decision now.
