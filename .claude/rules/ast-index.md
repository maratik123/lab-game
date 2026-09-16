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
   - Searching non-Go files (SQL migrations, YAML workflows, Markdown)

## Negative results are NOT evidence

A search that comes back **empty or short** for a construct that SHOULD exist is a
**search-method failure first, code-absence second.** Never conclude "X does not
exist" from a miss — re-run with a different method, or read the region.

| Cause of the false negative | Fix |
|---|---|
| Multi-line construct (a `gofmt`-split signature, a struct literal with tags, a chained builder call) | `rg -U` (multiline), or read the region |
| Hand-rolled identifier class — `[a-z_]*` excludes digits and capitals, and Go identifiers are `camelCase`/`PascalCase` with digits (`chatID`, `phase2Node`, `qR`) | `[A-Za-z0-9_]+`, or `ast-index symbol` / `ast-index outline`, which need no hand-written pattern |
| The symbol lives behind a build tag, in generated code, or in a file your pattern's path filter excluded | Read the file list first (`ast-index file`), then the source |
| Case-sensitive pattern over **prose** — instruction text, comments and headings capitalise mid-sentence words freely, so the emphatic occurrence is the one that escapes | `grep -rni` / `rg -i`; a clean sweep is evidence about your *pattern* until you have varied its case |
| An interface method searched as a declaration — Go interfaces are satisfied implicitly, so there is no `implements` keyword to find | `ast-index implementations "<Interface>"`, or search for the method name across types |

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
`2>/dev/null` on a probe whose emptiness is the verdict — a tool refusing to run and a genuine
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

| Task | Command | Time |
|------|---------|------|
| Universal search | `ast-index search "query"` | ~10 ms |
| Find type/interface | `ast-index class "SessionStore"` | ~1 ms |
| Find symbol | `ast-index symbol "SymbolName"` | ~1 ms |
| Find usages | `ast-index usages "SymbolName"` | ~8 ms |
| Find implementations | `ast-index implementations "Poster"` | ~5 ms |
| Call hierarchy | `ast-index call-tree "function" --depth 3` | ~1 s |
| Find callers | `ast-index callers "functionName"` | ~1 s |
| Package deps | `ast-index deps "package-name"` | ~10 ms |
| File outline | `ast-index outline "store.go"` | ~1 ms |

## Go-Specific Commands

| Task | Command |
|------|---------|
| Find a struct | `ast-index class "Session"` |
| Find an interface | `ast-index class "Notifier"` |
| Find implementors of an interface | `ast-index implementations "Notifier"` |
| Find methods on a type | `ast-index symbol "(*Session)"` or `ast-index outline "<file>.go"` |
| Find tests | `ast-index search "func Test"` |
| Find struct tags (db/json) | `rg -U 'db:"' --type go` — literal-with-quotes is a grep job |

## Index Management

- `ast-index rebuild` — Full reindex (run once after clone)
- `ast-index update` — After git pull/merge
- `ast-index stats` — Show index statistics

Both are automated by `SessionStart` and pre-commit hooks in `.claude/settings.json`.
