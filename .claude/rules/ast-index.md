# ast-index Rules

## Mandatory Search Rules

1. **ALWAYS use ast-index FIRST** for any code search task
2. **NEVER duplicate results** — if ast-index found usages/implementations, that IS the complete answer
3. **DO NOT run grep "for completeness"** after ast-index returns results
4. **Use grep/Search ONLY when:**
   - ast-index returns empty results
   - Searching for regex patterns (ast-index uses literal match)
   - Searching for string literals inside code (`"some text"`)
   - Searching in comments content
   - Searching a surface the index does not cover. **Measured on this tree with the installed binary: `.go`, `.sql` and `.sh` are indexed; `.yml`, `.md` and `go.mod` are not** — so the workflows, the instruction corpus and the module files are grep's, while migrations and the harness's own scripts are not. Re-measure the same way after a toolchain upgrade rather than trusting this line.

## Negative results are NOT evidence

A search that comes back **empty or short** for a construct that SHOULD exist is a
**search-method failure first, code-absence second.** Never conclude "X does not
exist" from a miss — re-run with a different method, or read the region.

| Cause of the false negative | Fix |
|---|---|
| Multi-line construct (a `gofmt`-split signature, a struct literal with tags, a chained builder call) | `rg -U` (multiline), or read the region |
| Hand-rolled identifier class — `[a-z_]*` excludes digits and capitals, and Go identifiers are `camelCase`/`PascalCase` with digits (`chatID`, `phase2Node`, `qR`) | `[A-Za-z0-9_]+`, or `ast-index symbol` / `ast-index outline`, which need no hand-written pattern |
| The symbol lives in generated code, or in a file your pattern's path filter excluded | Read the file list first (`ast-index file`), then the source |
| **The opposite, and it is the one that misleads:** a file behind a build tag **is** indexed. Measured: a symbol in a `//go:build integration` file answers `ast-index symbol` while `go build ./...` never compiles it | Before concluding a symbol is live, check the tag at the top of its file — the index says it exists, not that the default build sees it |
| Case-sensitive pattern over **prose** — instruction text, comments and headings capitalise mid-sentence words freely, so the emphatic occurrence is the one that escapes | `grep -rni` / `rg -i`; a clean sweep is evidence about your *pattern* until you have varied its case |
| An implementor searched through `implementations` — Go satisfies an interface implicitly, and **the index does not infer it**. Measured twice: on a clean probe where one type carried the whole method set, `implementations` returned **nothing**; on this tree it returned two *functions* from a same-named file rather than the implementing types | Read the interface's method set and search the method names (`ast-index symbol`), or read the type. Treat `implementations` on a Go interface as a hint to follow, never as the answer |

**MUST — a claim that an API, symbol, flag, or precedent does NOT exist requires a
raw read of the source (or `go doc <pkg>.<Symbol>`), never a search tool's silence.**
Prescribing a *replacement* off such a negative compounds it by inventing a second
nonexistent symbol.

## Positive results are NOT evidence either

**MUST — no search result is reportable until the same pattern has been run against a constructed
string it MUST match, and seen to match.** One control line per probe, **before** the conclusion —
not after being challenged. This binds a probe you write for yourself exactly as it binds a guard in
the repository.

**MUST — the control string is CONSTRUCTED, never borrowed from the artefact under test**, and least
of all from a line this change edits: a borrowed control fails silently in exactly the runs where the
edit worked, so its empty output is equally consistent with a working instrument. **NEVER** put
`2>/dev/null` on a grep whose emptiness is the verdict — a tool refusing to run and a genuine
absence produce the same empty stdout, and the redirect is what makes them indistinguishable. Prefer
several simple patterns over one long alternation: a regex engine has complexity limits, and its
failure mode is an error you have arranged not to see. An enumeration in prose has no canonical
order, so a literal phrase carrying one is a pattern for one variant, never for the claim.

**MUST — before reading a probe's result, assert that the probe LANDED where the instrument looks,
and report where that is.** A control proves the pattern RUNS; it never proves the pattern REACHED
the subject. So the probe reports two things or it reports nothing: the region the instrument
actually judges — read from its section parser, its glob, or its index walk, never assumed — and the
evidence that the mutation or the pattern is inside that region (print the mutated line, or diff it).
Never chain a probe behind its own mutation with `&&`, which converts a failed mutation into a
skipped verification that looks like nothing happened. A probe that relocates or reshapes the
artefact tests the harness, not the subject, and a degradation notice on stderr **voids** the probe
rather than decorating it. An instrument's silence becomes evidence only after one probe has been
seen to FAIL **and** one genuine case has been seen to PASS — the second half is the one skipped once
the first probe finally fails.

**MUST — a hit proves a STRING occurs; it NEVER proves a BEHAVIOUR exists.** That a script *handles*
a flag, that a gate *fires*, that a function *does* X — each is established by running the thing,
never by matching its name.

**MUST — read the cardinality of every input BEFORE the verdict**, for any check shaped as *intersect
two sets* / *diff against a baseline* / *grep a corpus*. An empty right-hand side makes `comm -12`
and `grep -f` report the clean answer for every possible left-hand side. Likewise an
enumeration that reached nothing reports "no findings": `git ls-files`, `git diff --cached` and any
index walk say nothing about a file not yet in that set, so run such a checker AFTER `git add`, and a
NEW checker against its own new files by explicit path.

**Vary the encoding before believing a clean sweep over PROSE** — digits against spelled numerals,
leading markers against trailing ones, case, and the multi-line form. A pattern is written against
the *typical* form of its target; prose is where one encoding hides the instances.

Each pattern fails on a VARIANT of the thing sought, never on the thing itself: a bracket inside a
bracket class, a trailing marker against a leading one, a spelled numeral against a digit, an
occurrence against a behaviour, a legitimate non-zero exit against an error.

## Why ast-index

ast-index is 17–69× faster than grep (1–10 ms vs 200 ms–3 s) and returns structured, accurate results.

## Command Reference

**`ast-index --help` is the authoritative list, and it grows between releases — this table is the subset the flows lean on, not a picture of the tool.** Every row below was run against this tree.

| Task | Command |
|------|---------|
| Open an unfamiliar area | `ast-index explore "<question or bag of names>"` — ranked symbols with their source and tests, in one shot |
| Universal search | `ast-index search "query"` |
| Find type/interface | `ast-index class "Session"` |
| Find symbol | `ast-index symbol "SymbolName"` |
| Resolve a file's indexed path | `ast-index file "post.go"` |
| File outline | `ast-index outline "internal/store/post.go"` — **the indexed path, not the base name**: a bare `post.go` answers `File not found`, which is why the row above exists |
| Definitions, imports and usages at once | `ast-index refs "SymbolName"` |
| Find usages | `ast-index usages "SymbolName"` |
| Call hierarchy | `ast-index call-tree "function" --depth 3` |
| Find callers | `ast-index callers "functionName"` |
| Where things live | `ast-index map` — one line per directory with its symbol kinds |
| What this branch touched | `ast-index changed` — **read the branch it names**: it prints the base it diffed against, and that base is not always the one you had in mind |
| Open markers | `ast-index todo` |
| Candidates for deletion | `ast-index unused-symbols` — **a question, never an answer**: a symbol nothing in the index calls shows up here, and the index does not see a SQL table reached from a string-built query, a shell function a hook or CI calls, or a symbol reached only through an interface value |

**The module commands do not apply here.** `deps`, `dependents`, `module-route`, `unused-deps` and `api` read a module graph this project has none of: a rebuild over the whole tree reports `0 modules, 0 deps`, and `deps` answers *"Module dependencies not indexed"*. Go package dependencies are `go list` / `go mod why`'s to answer (`AGENTS.md` § *Dependency Versions*).

## Go-Specific Commands

| Task | Command |
|------|---------|
| Find a struct | `ast-index class "Session"` |
| Find an interface | `ast-index class "Gate"` — an interface and a same-named struct both answer, each with its kind |
| Find implementors of an interface | **No command does this.** See the false-negative table: read the method set and search the method names |
| Find methods on a type | `ast-index outline "<indexed path>.go"` — a method is indexed as a bare function, so `symbol "(*Session)"` returns nothing (measured) |
| Find tests | `ast-index search "func Test"` |
| Find struct tags (db/json) | `rg -U 'db:"' --type go` — literal-with-quotes is a grep job |

## SQL migrations are indexed too

The goose migrations under `internal/store/migrations/` are in the index, so a schema question does not start with grep. Measured on this tree's own shapes: a `CREATE TABLE` is a class, a `CREATE INDEX` a property, a `CREATE FUNCTION` a function — and a **commented-out** statement produces a content match and no symbol, so a symbol hit here is a live definition.

| Task | Command |
|------|---------|
| Find a table | `ast-index class "posting"` |
| Find an index | `ast-index symbol "idx_posting_account"` |
| Find where a table is touched | `ast-index usages "posting"` — then read the call sites, because a query built as a string reaches no index |

## Index Management

- `ast-index rebuild` — Full reindex (run once after clone)
- `ast-index update` — After git pull/merge
- `ast-index stats` — Show index statistics

Both are automated by `SessionStart` and pre-commit hooks in `.claude/settings.json`.
