# Comment reference ban — doc comments stay, outward references go

**Source:** issue #68, reformulated by the owner on 2026-09-08
**Date:** 2026-09-08
**Tracked in:** #68

A comment in a non-markdown file may say what a thing is, state its call contract, and carry
whatever the linter mandates. It may not point outward at a place that will move. The reason
is rot: the thing pointed at is edited, and the comment becomes a claim nothing checks. Doc
comments themselves are kept; `revive` keeps demanding them.

This supersedes the direction the #68 body currently states (delete every comment, including
doc comments). See `## Source conflicts`.

**How this document spells a reference.** The spec is a durable markdown artefact, so it obeys
the project's durable-reference convention, which is a *different* rule from the ban it
defines: a place named here is a **symbol** — package plus function, or a type, or a named
document section — resolved before it is written; a coordinate appears only as a historical
measurement and only carrying the commit it was taken at
[source: 76a7b41:ai-docs/learnings.md § entry whose Rule opens "A reference that is written
down to be read later names a **symbol**" · `grep -n 'names a \*\*symbol\*\*' ai-docs/learnings.md`].
The ban this spec defines governs comments **in code**, where the settled rule is stricter
still. They must not be conflated.

## Scope

1. **The rule.** In a file of the gated set (KD-1), a comment carries only: a short
   description, the call contract, and what the linter requires. It carries no reference of a
   banned class (`## Banned reference classes`). It also does not narrate what the code does
   step by step, or how it is implemented (KD-13).
2. **The sweep.** Every existing comment in the gated set is brought into compliance, wherever
   on its line the comment marker sits (KD-14). The reference is removed; where a sentence
   exists only to carry the reference, the sentence goes with it. Outside `config/**` and
   `.env.example` the fact the reference carried is simply lost — it is not relocated to a
   design document or to `ai-docs/**` (owner, round 0 answer 6).
3. **`config/**` gains documentation instead of losing it.** This is the owner's mid-round-1
   narrowing of answer 6. Every key in every comment-bearing file under `config/` and every
   directory nested under it ends up with self-contained prose that describes the key without
   pointing outward, at the completeness level a cleaned `.env.example` reaches per variable.
   Today the only such file is `config/balance.yaml`, whose per-key documentation is in most
   cases nothing but a `docs/DESIGN.md §…` pointer, so this is a net writing task
   [source: 76a7b41:config/balance.yaml · `cat -n config/balance.yaml`].
4. **`.env.example` is cleaned to the same rule and is the model for item 3.** Its per-key
   prose already meets the bar. Its header block loses its pointers but **keeps its warning**
   (KD-15): a variable the config loader does not read does not belong in the file. That
   sentence is a contract about the file, not a pointer, and it survives restated without the
   package name, the test path and the markdown path it currently leans on
   [source: 76a7b41:.env.example · `cat -n .env.example`].
5. **The machine gate.** A pre-commit check and a CI gate refuse a banned reference in a gated
   file. The gate catches references, never verbosity or narration (owner, round 0 answer 5);
   the narration half of item 1 is a review judgement (KD-13), and so is the bare unqualified name
   that no lexical rule separates from prose (§ Banned reference classes).
6. **`.githooks/pre-commit` becomes a symlink** to a `.sh` file in the same directory (owner,
   round 0 answer 4). It is the only tracked file that carries a `#!` shebang and does not end
   in `.sh` [source: 76a7b41 · `git ls-files | while read f; do head -c2 "$f" | grep -q '#!' && case "$f" in *.sh) ;; *) echo "$f";; esac; done`].
7. **Shell that does not live in a `.sh` file** — `run:` blocks in workflow files and hook
   bodies in `.claude/settings.json` — is cleaned by the same rules but is **not** gated. The
   doc-style document says so, so that a code writer and `self-review` look for it (owner,
   round 0 answer 3).
8. **Rule-text propagation, as a class:** every site whose claim this diff falsifies, per
   `AGENTS.md` § Propagation Rule step 4. The known members are listed under
   `## Technical constraints`; they illustrate the class, they do not bound it.
9. **Guard-script usage prose becomes `--help`.** Every tracked `*.sh` whose comments carry
   usage prose — a comment introducing the script's invocation grammar, typically naming the
   script's own path — moves that grammar out of the comments and into executable behaviour
   (KD-16, KD-17). A script whose comments carry no usage prose gains no flag. This is the part
   of the task that ships executable code rather than prose.
10. **The #68 body is rewritten** to state the reformulated rule ("давай переформулируем issue"
   — owner). A body that still prescribes deleting doc comments would mislead every later
   reader of the tracking issue.
11. **CI shellchecks every script it is documented to shellcheck.** `AGENTS.md` § Build & Test
   describes the Harness-guards job as running "shellcheck on every script and hook body"
   [source: fa9279f:AGENTS.md § Build & Test · `grep -n 'Harness guards' AGENTS.md`]. The job
   shellchecks what its own `find` expression reaches, and that expression names two harness
   directories, neither of them `.githooks/`, so no script under `.githooks/` is shellchecked by
   CI at all
   [source: fa9279f:.github/workflows/ci.yml § job `harness`, step "shellcheck every harness script" · `sed -n '/name: shellcheck every harness script/,/^      - name:/p' .github/workflows/ci.yml`];
   the `harness` path filter does not name that directory either, so a commit whose only changed
   paths are under `.githooks/**` matches no filter and runs no job at all. CI never invokes the
   `Makefile` shellcheck target, which does reach the directory
   [source: fa9279f:.github/workflows/ci.yml § its `run: make …` steps · `grep -n 'run: make ' .github/workflows/ci.yml`].
   This task adds a script under `.githooks/` (item 6) and puts `.githooks/**` in the gated set
   (KD-1), which makes the gap load-bearing, so the gap is closed here (owner, 2026-09-08:
   *«В скоуп — починить здесь»*).

## Out of scope

- Deleting doc comments wholesale, and dropping `revive` / `exported` / `package-comments`.
  That was the pre-reformulation direction and is now reversed.
- Markdown prose anywhere (`docs/**`, `ai-docs/**`, `.claude/**`, `AGENTS.md`). Markdown is
  excluded by the owner's own framing ("кроме md"), and it obeys the durable-reference
  convention instead. Rule *text* inside those files still changes under item 8 — that is
  propagation, not gating.
- `go.mod`. Every comment in it is an `// indirect` marker written by `go mod tidy`, and
  `AGENTS.md` § Dependency Versions forbids hand-editing the file
  [source: 76a7b41:go.mod · `grep -n '//' go.mod`].
- Machine-checking that a comment is brief or does not narrate implementation. That stays a
  review judgement (owner, round 0 answer 5; KD-13).
- Changing any balance value. `config/balance.yaml` gains prose; its numbers are untouched.
- Files that cannot carry a comment: `.claude/settings.json` as a JSON document (its hook body
  *strings* are covered by item 7), `go.sum`, `*.jsonl`, `*.gitkeep`,
  `ai-docs/coverage-ratchet.txt`.

## Deferred

| What | Why | Separate issue? |
|---|---|---|
| Machine-checking the positive `config/**` documentation requirement (every leaf key carries prose) | The owner stated the requirement, not an enforcement mode; adding a second gate unasked is scope widening (KD-8) | Yes, if the requirement is seen to rot |
| Machine-checking the narration half of the rule | Ruled out by the owner (answer 5); the criterion is review-judged (KD-13, AC16) | No |
| Machine-checking a bare unqualified name used as a pointer ("see such-and-such") | No lexical rule separates it from ordinary prose or from the same-package contract symbol the KD-9 carve-out keeps; the owner settled the gated symbol class as the package-qualified form (KD-9). Review-judged under AC16 | No |

## Key decisions

| Question | Decision |
|---|---|
| KD-1 — which file classes does the machine gate cover? | **Settled (round 1).** `*.go`, `*.sh`, `*.sql`, `*.yml`, `*.yaml`, `.gitignore`, `.env.example`, `Makefile`, `.githooks/**`. |
| KD-2 — do doc comments survive? | Yes. The reformulation keeps them; only their outward references go. |
| KD-3 — does `revive` stay enabled? | Yes, with `exported` and `package-comments` unchanged. The owner's rule explicitly preserves "обязательные вещи, которые требует линтер" [source: 76a7b41:.golangci.yml § linters.settings.revive · `sed -n '/^    revive:/,/^  exclusions:/p' .golangci.yml`]. |
| KD-4 — is a doc comment's leading identifier a banned symbol reference? | No. `revive`'s `exported` rule mandates the name-first opening; a comment naming its own subject is not pointing outward. |
| KD-5 — are machine-read directives exempt? | The directive itself is exempt in full: the `//go:` family, `-- +goose Up` / `-- +goose Down`, `//nolint:<linter>`, `# shellcheck disable=…`, `#!`. The human **reason text** a directive carries obeys the same rules, because it is prose and rots like prose. |
| KD-6 — what happens to comments inside YAML block scalars (`run: \|`, `filters: \|`)? | Cleaned by the same rules, not machine-gated. The gate covers YAML-level comments. Mechanical reading of the owner's round 0 answer 3. |
| KD-7 — where does the fact in a deleted reference go? | Nowhere, outside `config/**` and `.env.example` (answer 6). Inside those, it is replaced by self-contained prose (Scope items 3 and 4). |
| KD-8 — is the `config/**` documentation requirement gated? | No. Stated in the doc-style document and checked in review, consistent with the answered posture that the gate catches references and not quality. Recorded in `## Deferred`. |
| KD-9 — does the symbol ban cut into the call contract? | **Settled (round 1); narrowed by the owner on 2026-09-08: this module only** (*«Только свой модуль — гейтим»*). A symbol may be named when it *is* the contract — a returned sentinel error, the guarantor of a precondition — and only from the comment's own package. What is banned is a **package-qualified symbol of this Go module** named outside the comment's own package. A qualified symbol of the standard library or of a third-party module is not a reference for this rule: the ban's reason is rot, and a name that does not move when this tree is edited does not rot — `time.Duration`, `errors.Is`, `context.Context` stay as written. "See such-and-such" written as a bare unqualified name is outside the class too, for the reason given under § Banned reference classes, and is refused in review instead (AC16). `ai-docs/doc-convention.md` § DOC-3 and `ai-docs/go-api-naming.md` § The `…Unchecked` AXIOM survive unchanged. |
| KD-10 — which remaining classes count as a banned reference? | **Settled (round 2): all of them — "ничего наружу".** The key-decision anchor, the design-section number written without its file, the issue or pull-request number, the bare repository path and the URL are all banned, with an exemption for the `TODO(#<issue>)` form, which keeps its issue number. |
| KD-11 — does `ai-docs/doc-convention.md` survive? | Yes, rewritten. Doc comments survive, so their convention needs a live carrier; § DOC-4 inverts; § DOC-5 stands unchanged, its `TODO(#<issue>)` requirement exempted by KD-10; § DOC-3 stands per KD-9. |
| KD-12 — English or Russian in the rewritten `config/balance.yaml` prose? | English. `AGENTS.md` reserves Russian for `docs/**` and for conversation; `config/**` is neither, and the file currently carries Russian comments [source: 76a7b41:config/balance.yaml · `grep -nP '[\x{0400}-\x{04FF}]' config/balance.yaml`]. |
| KD-13 — how is "no implementation narration" verified? | By review, against the diff. The owner ruled out a machine check (answer 5), so the criterion is a stated condition over the tree that a reviewer judges (AC16) — not an unverified aspiration, and not a gate. |
| KD-14 — are trailing and inline comments in scope? | **Yes.** A comment is a comment wherever its marker sits on the line, and a gate anchored to line-start would pass a violation silently — the failure shape `AGENTS.md` § Build & Test names. The `exhaustive` entry in `.golangci.yml` carries a design-section reference in exactly that position [source: 76a7b41:.golangci.yml · `grep -nE '^[^#]*#.*§' .golangci.yml`]. The gate must therefore recognise a comment marker anywhere on the line while not mistaking one inside a Go or SQL string literal for a comment. |
| KD-15 — does `.env.example`'s header warning survive? | Yes, restated without its pointers. The header carries pointers — a package name, a test path, a markdown path, an issue number — and, separately, a **contract about the file** — that a variable the config loader does not read must not be added here. Answer 6 discards the fact a *reference* carried; this is not one. It survives also because the owner made this file the completeness model for `config/**`, and a model that dropped its own key invariant would teach the wrong thing. Listed under open questions as revisitable. |
| KD-16 — which scripts owe a `--help`, and does `check-citations.sh`? | Every tracked `*.sh` whose comments carry usage prose owes one; its complement owes nothing. The judgement the owner left open resolves **no**: `check-citations.sh` references no positional parameter, no `$@`, no `$#` and no `shift`, so the line the ban strips — *"Also runnable standalone"* plus its own path — states no invocation grammar, only a path, and nothing is left undocumented. The membership criterion, and the conditions under which a script in the complement is nonetheless documented, are in § Usage prose becomes `--help`. |
| KD-17 — what shape does `--help` take? | The design chooses one shape and applies it to every script in that set, because there is no precedent in the tree to copy (see § Usage prose becomes `--help`). The condition the shape must meet: the flag prints the invocation grammar the `# Usage:` prose used to carry, and exits 0. `coverage-ratchet.sh` already parses `--check`, so it has an argument-handling site to extend rather than introduce. |
| KD-18 — why is the CI shellcheck gap this task's to close? | Because this task creates a script under `.githooks/` (Scope item 6; Scope item 11 records what CI does not reach today) and puts `.githooks/**` in the gated set (KD-1): a directory CI does not reach becomes load-bearing here, and a guard whose CI coverage is absent is a gate that silently does not run. Settled by the owner on 2026-09-08: *«В скоуп — починить здесь»*. Which shape the fix takes — extending the job's `find` expression, or having CI invoke the `Makefile` target that already reaches every script — is the design's choice. |

## Technical constraints

### Banned reference classes

Settled. A comment names nothing outside itself that moves when this tree moves (owner, round 2:
*"ничего наружу"*, narrowed for the symbol class on 2026-09-08 to this module's own packages —
KD-9):

| Banned class | Example in the tree |
|---|---|
| A reference to a place in a file that carries no revision — a line number, with or without its path | a bare `<file>:<line>` locator |
| A path to a markdown file | `docs/DESIGN.md §2.2.4`, `ai-docs/go-test-conventions.md` |
| A spec or design acceptance-criterion id | `AC6`, `AC9` |
| A package-qualified symbol of **this** Go module named outside the comment's own package — the form `<pkg>.<Ident>`, where `<pkg>` names a package of this module | `store.Post` in a comment of package `internal/scheduler` [source: fa9279f:internal/scheduler/schedule.go § Schedule · `grep -n 'store\.Post' internal/scheduler/schedule.go`] |
| A key-decision anchor | `// (design D4, D9). A non-nil return means ctx was cancelled mid-attempt` |
| A design-section number written without its file | `DESIGN §3.5`, `§16.5` |
| An issue or pull-request number outside the `TODO(#…)` form | `# lab-game balance constants (issue #18, …)` |
| A repository path carrying neither a line number nor a revision | `# Usage: bash .claude/skills/ai-audit/scripts/test-check-citations.sh` |
| A URL | none occurs in the gated set today |

The exemptions, each already settled and none of them widened by *"ничего наружу"*:

- **The comment's own subject** (KD-4) — `revive` mandates the name-first opening.
- **A same-package symbol that *is* the contract** (KD-9) — a returned sentinel error, the
  guarantor of a precondition. `config.Load`'s doc comment, which names `ErrMissing`,
  `ErrInvalidValue`, `ErrUnknownKey` and `ErrUnreadable`, all of package `internal/config`,
  survives as written [source: 76a7b41 · `ast-index outline "internal/config/config.go"`].
- **A machine-read directive** (KD-5), its human reason text excepted.
- **The `TODO(#<issue>)` form** (KD-10) — the issue number is exempt *in that form only*. A
  bare issue number elsewhere in a comment stays banned.

A reference from one key of a config file to a sibling key of the same file — `.env.example`'s
*"strictly below LAB_GAME_TG_ATTEMPT_TIMEOUT"* — is not outward and is part of the key's
contract, so it stays. It is named here because it looks like a reference and a sweeper will
ask.

**KD-9 is not overturned by *"ничего наружу"***. That answer settled the classes the question
put to the owner — key-decision anchor, section number, issue number, bare path, URL. The
same-package contract symbol was settled the round before with its own carve-out, and a later
reader must not collapse them.

**What the narrowing leaves outside the symbol class, stated so that nothing is left implied.**
The class above is written in the one form a lexical gate can decide, and the gate decides it in
full — which is why AC2 and AC3 do not narrow for it:

- **A qualified symbol from outside this module is not a member.** `time.Duration`, `errors.Is`,
  `context.Context`, `telego.GetUpdatesParams`, and the bare `GetUpdatesParams.Limit` by which
  `.env.example` names that type's field, all survive the sweep as written. Membership turns on
  `<pkg>` naming a package of *this* module, and that set of names is read off the tree the gate
  runs against, never inherited from this document.
- **A bare, unqualified name is not a member.** `Session`, `Type.Field`, or a "see such-and-such"
  with no package qualifier cannot be separated by any lexical rule from ordinary prose, or from
  the same-package contract symbol the KD-9 carve-out keeps. It stays banned as prose that points
  the reader elsewhere, and it is judged in review against the diff, exactly as the narration half
  is (KD-13, AC16). No gate is asked to decide it, and no criterion claims one does.
- **A gated file that has no Go package** — `.env.example`, a `Makefile`, a `*.sh`, a `*.yml`, a
  `*.sql` — has no own package for the carve-out to compare against, so a package-qualified symbol
  of this module in its comments is always outside the comment's own package, and banned.

### Usage prose becomes `--help`

The bare-path class removes a guard script's `# Usage:` line, which names the script's own
path. The owner's substitution is that the information becomes executable: *"guard-скрипты
обязаны вместо usage иметь `--help`, а если usage отсутствует, то и `--help` не нужен"*. A
renamed script's `--help` still works; its `# Usage:` line silently lies.

The set that owes a `--help` is every tracked file matching `*.sh` whose comments carry usage
prose — a comment introducing the script's invocation grammar, whether written on one line or as
a heading with the grammar on the lines below it. Its complement is every tracked `*.sh` whose
comments carry none. The sweep and the verifier each derive both sets from the tree they run
against; neither inherits a list from this document, because a script added, renamed, or given a
usage line after this was drafted belongs in the set on the same criterion.

**No script in the tree answers `--help`, so there is no shape to copy.** Where `--help` occurs
at all it is fixture data and a carve-out pattern asserted to survive in a hook body; invoked
with the flag, that script ignores it and runs its whole suite
[source: a386fe7 · `bash .claude/skills/ai-audit/scripts/test-piped-gate-guard.sh --help`, which
prints `piped-gate guard: all fixtures behave as specified` and exits 0]. The design therefore
chooses one shape and applies it to every script in the set (KD-17).

No script in the complement is left undocumented by owing nothing. A script belongs there on
grounds a verifier reads off the script itself: it has no invocation grammar to relocate,
referencing no positional parameter, no `$@`, no `$#` and no `shift`; or it already surfaces
that grammar as behaviour rather than as prose, printing a usage string at runtime on a wrong
call.

That criterion is stated in prose, not in a table cell, for the reason recorded under § Banned
reference classes: a markdown cell forces `\|` escaping, and in an extended regular expression
`\|` is a literal pipe rather than alternation, so an escaped alternation matches nothing and
silently reports every script as taking no arguments.

### The symlink is measured, not assumed

On git 2.55.0 with `core.hooksPath = .githooks`, a real `git commit` runs a `pre-commit` that
is a symlink to a sibling `.sh` file, and a non-zero exit from it still refuses the commit.
The success direction and the failure direction were each exercised in a scratch repository; git records the symlink as mode
`120000` [source: 76a7b41 · scratch repository, `git init` + `ln -s pre-commit.sh .githooks/pre-commit` + `git commit`, marker asserted present on the success path and the commit asserted refused on the failure path].

### Known members of the propagation class

Not a bound on the class (Scope item 8) — the sites already identified:

- `ai-docs/doc-convention.md` § DOC-4 — Design citations: mandates exactly what the ban
  forbids. § DOC-5 — What not to write and § DOC-3 — Contract sections stand unchanged, under
  KD-10 and KD-9 respectively.
- `ai-docs/go-api-naming.md` § The `…Unchecked` AXIOM: stands unchanged under KD-9.
- `AGENTS.md` — § API Naming restates that AXIOM; § Code Style's Documentation bullet; the
  panic-justification rule in § Go Test Conventions; Boundary rule 2's file list.
- `.golangci.yml` — comments on the `exhaustive` and `revive` entries carry references, including
  a trailing one (KD-14).
- `Makefile` — the `shellcheck` target special-cases `.githooks/pre-commit` on its own line
  because the file has no `.sh` extension; the symlink makes that line redundant or a
  deliberate double-check [source: 76a7b41:Makefile § shellcheck target · `sed -n '/^shellcheck:/,/^$/p' Makefile`].
- `.github/workflows/ci.yml` — the `paths-filter` block, whose own comment warns that a gate
  not named there silently stops running; a new gate needs its filter entry and its job, and
  `.githooks/**` needs a filter entry of its own (Scope item 11). The Harness-guards job's
  shellcheck step is the other site: its `find` expression is what decides which scripts CI
  checks.
- `.claude/skills/task/scripts/test-precommit-dispatch.sh` — the regression suite for the hook
  whose shape Scope item 6 changes.
- `.claude/settings.json` — the `--no-verify` guard and the panic gate mention the hook and the
  doc-comment justification.
- Every skill or agent file that documents how a guard script is invoked, since Scope item 9
  moves that grammar behind `--help`.
- Harness surfaces that instruct an agent to write or check a doc comment.

### Conventions the gate inherits

- Every gate this project owns is reachable through `Makefile`. The Go gates are invoked by CI
  from that target; the harness guards are not — CI runs them directly, and its shellcheck step
  carries an expression of its own instead of calling the target, which is how the `.githooks/**`
  gap of Scope item 11 opened. A gate this task adds is invoked from the same `Makefile` target
  locally and in CI, so a local run and a CI run cannot disagree about the command.
- Guard scripts in this repository are shell, live beside a regression suite that CI runs, and
  are covered by `make shellcheck`. `AGENTS.md` requires tests for any file with ~50+ lines of
  substantial logic.
- The gate is itself a file of the gated set, so it must satisfy the rule it enforces. The
  guard scripts already in the tree explain themselves by naming the acceptance criterion they
  lock, which is the style the new gate cannot use.

## Source conflicts

Each quotation below was read at `76a7b41` and is located by its document section, not by a
line number.

1. **Issue #68's persisted body vs. the owner's reformulation.** The body says: *"Every comment
   in `.go`, `.sh` and `.sql` goes, **including the doc comments the linter currently
   demands**"*, and prescribes dropping `revive` and deleting `ai-docs/doc-convention.md`. The
   owner's message of 2026-09-08 says: *"давай переформулируем issue: doc-комменты оставляем,
   но они не должны иметь ссылок…"* **Resolution:** the reformulation governs; the owner chose
   it explicitly and asked for the issue to be reformulated. Scope item 10 rewrites the body
   [source: 76a7b41:ai-docs/plans/2026-09-08-comment-reference-ban.spec.md.state.md § gh_issue · `sed -n '/^  body: |/,/^  comments:/p' ai-docs/plans/2026-09-08-comment-reference-ban.spec.md.state.md`].

2. **`ai-docs/doc-convention.md` § DOC-4 — Design citations vs. the ban.** The section says:
   *"Anything implementing a designed mechanic cites its section, so a reader can find the
   reasoning without hunting"*, and closes: *"Cite the section number, never a line number: the
   design document is edited, and `§2.2.4` survives what `:118` does not."* The convention
   mandates the citation the ban forbids — and justifies it with the same rot argument the ban
   uses, having already ruled out the unanchored line reference for that reason.
   **Resolution:** the ban governs; § DOC-4 inverts. The `§`-form does not survive either:
   KD-10 bans the design-section number written without its file
   [source: 76a7b41:ai-docs/doc-convention.md § DOC-4 — Design citations · `sed -n '/^## DOC-4/,/^## DOC-5/p' ai-docs/doc-convention.md`].

3. **`ai-docs/doc-convention.md` § DOC-5 — What not to write vs. the issue-number class.** The
   section says: *"No `TODO` without an issue reference: `// TODO(#<issue>): …`. A `TODO`
   without an owner is a lie about future work."* **Resolution:** § DOC-5 stands unchanged. The
   owner's round-2 answer bans the issue-number class but exempts the `TODO(#<issue>)` form
   specifically, so a `TODO` keeps its owner. No `TODO(#` occurs in the gated set today, so
   nothing in the sweep turns on it
   [source: 76a7b41 · `git ls-files '*.go' '*.sh' '*.sql' '*.yml' '*.yaml' '.gitignore' '.env.example' 'Makefile' | xargs grep -l 'TODO(#'`]
   [source: 76a7b41:ai-docs/doc-convention.md § DOC-5 — What not to write · `sed -n '/^## DOC-5/,/^## DOC-6/p' ai-docs/doc-convention.md`].

4. **The symbol ban vs. the contracts that mandate naming a symbol — resolved in round 1.**
   `ai-docs/go-api-naming.md` § The `…Unchecked` AXIOM requires that *"its doc comment MUST
   state (a) the precondition, and (b) who guarantees it"*, and rules a variant *"whose doc
   comment does not name the guarantor"* to be **Wrong** — *"the comment is the contract"*.
   `ai-docs/doc-convention.md` § DOC-3 — Contract sections requires that *"A function returning
   a sentinel error names it"*. **Resolution:** the owner's round-1 answer carves out the
   same-package symbol that *is* the contract, so those sections stand unchanged (KD-9). The
   owner's narrowing of 2026-09-08 shrinks the conflict further — the banned symbol class reaches
   only this module's own packages — without changing that resolution
   [source: 76a7b41:ai-docs/go-api-naming.md § The `…Unchecked` AXIOM · `sed -n '/^## The/,/^## Naming/p' ai-docs/go-api-naming.md`;
   76a7b41:ai-docs/doc-convention.md § DOC-3 — Contract sections · `sed -n '/^## DOC-3/,/^## DOC-4/p' ai-docs/doc-convention.md`].

5. **Round 0 answer 6 vs. the mid-round-1 constraint.** Answer 6, asked with
   `config/balance.yaml` as the named example: *"просто теряем, не жалко"*. The later message:
   *"Документацию в balance.yaml сделать самодостаточной и такой же полной, как в почищенном
   `.env.example` (это правило пропагируется на всю директорию config и все вложенные в нее
   директории)"*. **Resolution:** the owner narrowed his own earlier answer. Answer 6 governs
   everywhere outside `config/**`; inside it the outward reference still goes, but the file
   gains self-contained prose. Scope items 2 and 3, extended to `.env.example`'s file-level
   contract by KD-15.

## Acceptance Criteria

| # | Criterion |
|---|-----------|
| AC1 | No comment in a file of the gated set carries a reference of a banned class, wherever the comment marker sits on its line, except for the exemptions listed under § Banned reference classes. |
| AC2 | Staging a gated file whose comment carries a banned reference causes the pre-commit check to refuse the commit; staging a compliant one does not. |
| AC3 | The CI gate refuses a tree in which a gated file's comment carries a banned reference, and the workflow's path filter names every path class the gate covers, so the gate's job runs for a change to any of them. |
| AC4 | The gate reports the offending file, the line, and which banned class matched. |
| AC5 | The gate does not treat a comment marker occurring inside a Go or SQL string literal as a comment. |
| AC6 | The gate's own script and fixtures satisfy the rule the gate enforces. |
| AC7 | `.githooks/pre-commit` is a symbolic link whose target is a `.sh` file in the same directory, recorded in the index as a symlink; a real `git commit` still reaches the gates it dispatches to, and a non-zero exit from one of them still refuses the commit. |
| AC8 | Every tracked file beginning with a `#!` shebang either ends in `.sh` or is a symbolic link to a file that does. |
| AC9 | `make verify` is green and reaches the new gate; `golangci-lint run` is green with `revive`'s `exported` and `package-comments` rules still enabled. |
| AC10 | No doc comment demanded by `revive`'s `exported` or `package-comments` rules was removed: every exported item and every package still carries one. |
| AC11 | Every key of every comment-bearing file under `config/` and its nested directories carries prose that describes the key without pointing outward, and that prose is in English. |
| AC12 | `.env.example` documents each variable with self-contained prose, carries no outward pointer, and still states which variables do not belong in the file; its key set still matches the loader's consulted key set exactly, in both directions. |
| AC13 | The doc-style document states the rule, names the banned classes and the exemptions, records that the narration half and the bare-unqualified-name half are review-judged and why, and records that shell in workflow `run:` blocks and in `.claude/settings.json` hook bodies obeys the rule without a machine gate. |
| AC14 | No instruction file still mandates a comment reference of a banned class. History surfaces — `ai-docs/learnings.md`, `ai-docs/harness-gaps.md`, `ai-docs/plans/**` — keep whatever they say. |
| AC15 | The sweep changes comments and nothing else: in a file whose only reason to appear in the diff is the comment sweep, every line removed or rewritten is a comment line, and no statement, key, or value moves. A file that also carries a change this task requires for another reason is outside this criterion — `config/**`, `.env.example`, the scripts that gain `--help`, the files that wire the gate and its CI job, `.githooks/pre-commit` and its symlink target, and the files that close the shellcheck gap of Scope item 11. Behaviour preservation in those files is judged by the criteria that own their changes. |
| AC16 | No comment surviving the sweep narrates what the code does step by step or how it is implemented, and none points the reader elsewhere by a bare unqualified name ("see such-and-such"); each states what the thing is, its call contract, or what the linter requires. Judged in review against the diff — the owner ruled out a machine check for the narration dimension (KD-13), and no lexical rule can decide the bare name (§ Banned reference classes). |
| AC17 | Every tracked `*.sh` file that carried usage prose answers `--help` by printing the invocation grammar that prose carried, and exits 0. |
| AC18 | No tracked `*.sh` file that carried no usage prose answers `--help`, and none acquired argument handling it did not have. |
| AC19 | The `--help` handling is the same shape in every script that has it. |
| AC20 | No comment in any of those scripts still carries usage prose or the script's own path. |
| AC21 | Every script that gained `--help` still performs its original job unchanged when invoked without it, and `make shellcheck` is clean. |
| AC22 | Every instruction-file site that told a reader how to invoke one of those scripts now points at `--help` or is removed. |
| AC23 | The #68 body states the reformulated rule and no longer prescribes deleting doc comments or dropping `revive`. |
| AC24 | CI shellchecks every tracked `*.sh` file in the repository, those under `.githooks/` included, and a commit whose only changed paths are under `.githooks/**` runs the job that shellchecks them. |

## Open questions

- **Enforcement of the `config/**` documentation requirement** (KD-8). Defaulted to
  review-only. A mechanical form exists — every leaf key carries a non-empty comment above it —
  and can be added later without disturbing this task's gate.
- **`.env.example`'s file-level warning** (KD-15). Resolved to *survives, restated*. If the
  owner would rather the file carry nothing but per-key prose, the warning's content moves to
  the doc-style document and the file loses it.
- **Whether a script's `--help` output should itself be covered by a regression suite.** The
  guard suites CI already runs are the obvious home, and `AGENTS.md` requires tests for a file
  with ~50+ lines of substantial logic; whether a one-line flag clears that bar is the design's
  call, not a question for the owner.
