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
still. The two must not be conflated.

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
   the narration half of item 1 is a review judgement (KD-13).
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
9. **Guard-script usage prose becomes `--help`.** Thirteen tracked `*.sh` files carry a
   `# Usage:` comment naming their own path. The prose leaves the comments and becomes
   executable behaviour (KD-16, KD-17). A script that carries no usage prose gains no flag.
   This is the one part of the task that ships executable code rather than prose.
10. **The #68 body is rewritten** to state the reformulated rule ("давай переформулируем issue"
   — owner). A body that still prescribes deleting doc comments would mislead every later
   reader of the tracking issue.

## Out of scope

- Deleting doc comments wholesale, and dropping `revive` / `exported` / `package-comments`.
  That was the pre-reformulation direction and is now reversed.
- Markdown prose anywhere (`docs/**`, `ai-docs/**`, `.claude/**`, `AGENTS.md`). Markdown is
  excluded by the owner's own framing ("кроме md"), and it obeys the durable-reference
  convention instead. Rule *text* inside those files still changes under item 8 — that is
  propagation, not gating.
- `go.mod`. Every comment in it is an `// indirect` marker written by `go mod tidy`, and
  `AGENTS.md` § Dependency Versions forbids hand-editing the file
  [source: 76a7b41:go.mod · `grep -oE '//[a-z ]*' go.mod | sort | uniq -c`].
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
| KD-9 — does the symbol ban cut into the call contract? | **Settled (round 1): own package only.** A symbol may be named when it *is* the contract — a returned sentinel error, the guarantor of a precondition — and only from the comment's own package. "See such-and-such" and any cross-package symbol are banned. `ai-docs/doc-convention.md` § DOC-3 and `ai-docs/go-api-naming.md` § The `…Unchecked` AXIOM survive unchanged. |
| KD-10 — which remaining classes count as a banned reference? | **Settled (round 2): all of them — "ничего наружу".** The key-decision anchor, the design-section number written without its file, the issue or pull-request number, the bare repository path and the URL are all banned. One exemption: the `TODO(#<issue>)` form keeps its issue number. |
| KD-11 — does `ai-docs/doc-convention.md` survive? | Yes, rewritten. Doc comments survive, so their convention needs a live carrier; § DOC-4 inverts; § DOC-5 stands unchanged, its `TODO(#<issue>)` requirement exempted by KD-10; § DOC-3 stands per KD-9. |
| KD-12 — English or Russian in the rewritten `config/balance.yaml` prose? | English. `AGENTS.md` reserves Russian for `docs/**` and for conversation; `config/**` is neither, and the file currently carries Russian comments [source: 76a7b41:config/balance.yaml · `grep -nP '[\x{0400}-\x{04FF}]' config/balance.yaml`]. |
| KD-13 — how is "no implementation narration" verified? | By review, against the diff. The owner ruled out a machine check (answer 5), so the criterion is a stated condition over the tree that a reviewer judges (AC16) — not an unverified aspiration, and not a gate. |
| KD-14 — are trailing and inline comments in scope? | **Yes.** A comment is a comment wherever its marker sits on the line, and a gate anchored to line-start would pass a violation silently — the failure shape `AGENTS.md` § Build & Test names. The `exhaustive` entry in `.golangci.yml` carries a design-section reference in exactly that position [source: 76a7b41:.golangci.yml · `grep -nE '^[^#]*#.*§' .golangci.yml`]. The gate must therefore recognise a comment marker anywhere on the line while not mistaking one inside a Go or SQL string literal for a comment. |
| KD-15 — does `.env.example`'s header warning survive? | Yes, restated without its pointers. The header carries two distinct things: pointers (a package name, a test path, a markdown path, an issue number) and a **contract about the file** — that a variable the config loader does not read must not be added here. Answer 6 discards the fact a *reference* carried; this is not one. It survives also because the owner made this file the completeness model for `config/**`, and a model that dropped its own key invariant would teach the wrong thing. Recorded in `## Open questions

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
