# Dependency versions — live-lookup reference

Recipes behind the AXIOM in `AGENTS.md` § *Dependency Versions*. **Every claim about live state is queried, never remembered** — and the command must reach the category you are asking about, or its exit 0 answers a different question than yours.

## Lookup table

| Category | Question | Command |
|---|---|---|
| Module versions | "What is the current version of X?" | `go list -m -versions <module>` (add `@latest` form: `go list -m -json <module>@latest`) |
| Module versions, offline-ish | Same, straight from the proxy | `curl -sS https://proxy.golang.org/<escaped-module>/@v/list` — the module path is case-escaped (`!` before each capital) |
| Dep-graph membership | "Is X a dependency here?" | `grep <module> go.mod` for a **direct** requirement **AND** `go mod why -m <module>` for transitive reach |
| Why a module is here | "Who pulls X in?" | `go mod why -m <module>`; `go mod graph \| grep <module>` for the full edge list |
| Tool behaviour | "Does `<tool>` support `--flag`?" | `<tool> --help` or run it. **Never** from memory |
| VCS state | "Is this file tracked / ignored / committed?" | `git ls-files --error-unmatch <path>` (tracked), `git check-ignore -v <path>` (ignored), `git log -1 -- <path>` (committed). `git status` is **blind to ignored files** — empty output is never proof of absence |
| Upstream issue | "Is this bug fixed?" | `gh issue view <N> --json state,comments` — the body is frozen at filing time; the **closing comment** carries the resolution |

## Changing a dependency

```bash
go get github.com/example/mod@v1.4.2   # or @latest, or -u
go mod tidy
go build ./...
git diff go.mod go.sum                 # confirm the delta is only the intended edges
```

- **Never hand-edit a version in `go.mod`.** The tool keeps `go.sum` consistent; a hand edit does not.
- `go mod tidy` also prunes and adds transitive lines. Read the diff before staging — an unrelated bump riding along is exactly what the review is for.
- Go pins exact versions; there is no range syntax to loosen, so a bump is always an explicit, reviewable diff.

## Failure mode — a filter that prints a non-answer and exits 0

A `jq` filter over an error body prints `null` and exits **0**. A `grep` over the wrong file exits 1 and *looks* like "not a dependency". Both are fact-shaped non-answers.

> **If a lookup returns nothing, re-run it without the filter and read the raw output before concluding anything.** Absence of evidence from a command that never reached the category is not evidence of absence — the same principle `.claude/rules/ast-index.md` states for code search.

## Standard library first

A new dependency needs a stated reason in the design document. The load-bearing ones are already fixed in `docs/DESIGN.md` §11 and [`key-decisions.md`](key-decisions.md): `telego` (low-level only), `pgx`, a migration tool, and a self-written scheduler rather than a job framework.
