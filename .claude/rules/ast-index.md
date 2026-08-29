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
