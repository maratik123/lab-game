# Design: Comment reference ban — doc comments stay, outward references go

**Issue:** #68
**Date:** 2026-09-08
**Revision:** round 5 — a Design Amendment raised at `/task` Step 11, and nothing else: the
owner's implementation-time ruling that the YAML extractor reads a single plain document and refuses
every other stream shape is written into § D1a, § D6, § Test Design, § Risks and subtask 1's row. No
other decision is reopened. Round 4 — the round-3 GO notes folded in, each re-measured rather than
transcribed: the `.githooks/**` router branch (D6, subtask 1, § Test Design), the duplication
argument for the
per-script `--help` block (D9), the pin on D8's `cp -r` probe, the source the `config/**` prose is
written from (D13), and D3's CI evidence re-answered on the channel the question is actually about.
Round 3 settled the YAML extractor's line numbers (D1a), the package survey for the four
scanner grammars (D1b), the re-measured `mvdan.cc/sh/v3` footprint (D3), and the cross-group
`--help` shape obligation (D9). Round 2 reconciled the design against the spec as amended at
`/task` Step 7: KD-9's narrowing to this module's own packages, AC15's carve-out written into the
criterion, and the new Scope item 11 / KD-18 / AC24 on CI's shellcheck coverage.

## Approach

The spec asks for work that only looks like one task: a **rule**, a **machine gate** for the
mechanically-decidable half of it, a **sweep** of every comment in the gated set, and the
**propagation** of the rule text into every instruction file whose claim the sweep falsifies. The
design keeps them separable and orders them so that the gate exists before the sweep it scopes.

**The ordering, and it is this round's main correction.** The gate is built first; the code-side
sweep follows; the **pre-commit dispatcher** may land with it, because it judges only the *staged*
paths of one commit; and the **runner wiring** — `verify`'s prerequisite and the CI job, which judge
the whole tree — lands **last, after the harness shell sweep as well**. Every tracked `*.sh` carries
a comment line of a banned class today: the complement of the matching set is empty
[measured `4772d33` ·
`comm -23 <(git ls-files '*.sh' | sort) <(git ls-files '*.sh' | xargs grep -lE '^[[:space:]]*#.*(Usage:|\.md|AC[0-9]|#[0-9]|§|ai-docs/|\.claude/|\.githooks/)' | sort)`
→ no output, both operands non-empty]. Those scripts are swept in the instructions/harness group, so
a `verify` that reached the gate before that group would be red across a group boundary, with CI red
on every push in between. Round 1 wired the runners after the *code-side* sweep and said so in this
paragraph; the code-side sweep does not reach `*.sh` outside `.githooks/`, and the sentence was
wrong.

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
  [measured `72d5bf7:ai-docs/scripts/test-doc-edit-guard.sh` · `sed -n '60,90p' ai-docs/scripts/test-doc-edit-guard.sh` →
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
| YAML | `go.yaml.in/yaml/v3` node comments (already a direct requirement) [measured `72d5bf7:go.mod` · `grep yaml go.mod` → `go.yaml.in/yaml/v3 v3.0.5`], **plus D1a's accepted source shape and line reconciliation** | it decides *which* `#` runs are comments — the hard half. It reports nothing from `ci.yml`'s `filters: \|` block scalar or from a `#` inside a quoted scalar, which is precisely the KD-6 boundary [measured `a18467f` · yaml.v3 probe over the tracked `*.yml`/`*.yaml` set plus synthetic fixtures → the `#`-leading lines inside `ci.yml`'s `filters: \|` block and inside a fixture's block scalar absent from the node comments, present in a naive line scan]. It does **not** answer *where* the comment is — D1a does |
| SQL, `Makefile`, `.gitignore`, `.env.example` | small lexical scanners in the same package, after the survey in D1b | for each of the four, every surveyed package's public API either discards comments entirely or cannot be reached from this module; the one package that does answer the question (SQL) costs cgo. D1b records what was evaluated, why each lost, and the escape hatch |

**D1a — the YAML extractor reads one plain document, and reconciles that document's line numbers
rather than reading them off the node.** Two rules, applied in that order.

**The accepted source shape, decided before anything is decoded.** The extractor reads a source that
is a **single plain document**: no unindented document marker (`---`, `...`) and no directive (a line
whose first token begins `%`). Every other stream shape is refused by a **named sentinel error**
before the decoder is reached, naming the offending line and its leading token, and the command turns
that refusal into exit 2 (D6). The marker is recognised as the line's **first whitespace-separated
token at column zero** — never by the separator that follows it, which YAML allows to be a space or a
tab and which the extractor therefore does not enumerate. A marker may appear only unindented, so a
column-zero match cannot be block-scalar content. A source the parser yields no node for is read
directly, line by line: such a source holds no key, therefore no block scalar, so every `#`-leading
line in it is a comment and there is no node walk to do
`[derived → the YAML refusal cases in § Test Design]`.

**Why the contract narrows instead of the grammar growing.** The alternative is a hand-written YAML
document grammar living inside this gate, and every attempt to enumerate that grammar shipped a fresh
hole in the guard's primary case — deciding which `#` runs in a YAML file are comments. A decoder
loop missed a document the parser yields no node for; the hand-split that replaced it missed `...`,
`--- <content>` and `%YAML`; the token check that replaced *that* missed a tab after the marker,
reopening the silent partial read the whole exercise was closing. The owner's ruling at `/task`
Step 11 was **fail-closed**: stop extending the enumeration, narrow what the extractor promises.
Narrowing deletes the grammar rather than lengthening it — an extractor that never sees a second
document never has to decide where the first one ends — and it converts every shape outside the
promise from a silent partial read into a loud refusal, which is the direction D1a already takes for
a comment that does not reconcile.

**What the narrowing costs, and where the escape is.** A legal, readable, parseable YAML file that
carries a marker or a directive — a `---`-headed workflow, a `docker-compose` file — turns
`make comment-refs`, and with it `verify` and the CI job, **red with an instrument failure rather
than a finding**, the moment it joins the gated set. What bounds the cost is that the refusal is
legible where it fires, since it names the line and the token, and that this paragraph is the second
place to look. The escapes, both deliberate: keep such a file a single plain document, or widen the
extractor back — which means re-introducing the document-boundary handling this narrowing removed,
with a case per marker form **and per separator**, and is a design decision rather than a Step-9 fix.
Nothing in the tree is affected today: no tracked YAML carries an unindented marker or a directive
[measured `8371fb4` · `git ls-files '*.yml' '*.yaml' | xargs grep -nE '^(---|\.\.\.|%)'` → no output,
over a non-empty file list].

**The line reconciliation.** `go.yaml.in/yaml/v3` never reports a comment's own line. A comment
arrives as `HeadComment`, `LineComment` or `FootComment` on the **node it attaches to**, and only
`LineComment` shares that node's line. Measured on this toolchain: a file header arrives as
`HeadComment` on the node *below* it; a comment block above a mapping key arrives on that key's
node; and a `FootComment` arrives on a node whose own line is *above* the comment, because that
node is a mapping key whose subtree ends higher up
[measured `a18467f` · yaml.v3 probe over the tracked `*.yml`/`*.yaml` set and synthetic
fixtures → every reported `HeadComment` and `FootComment` sits on a node line that is not the
comment's own; only `LineComment` shares it]. Taking `n.Line` as the finding's line would make
AC4's report wrong on nearly every YAML comment in the tree, and a presence/absence-only test table
cannot see it.

The extractor therefore recovers each comment's real line by **matching the retained text back
against the source**, nearest-first, in the direction the field name fixes:

- `LineComment` → the finding is on `n.Line`; the extractor asserts the source line ends with the
  returned text.
- `HeadComment` → the block is the run of `K` consecutive lines (`K` = the lines of the returned
  text) nearest **above** `n.Line` whose whitespace-trimmed content equals the returned lines, one
  for one. It is not `n.Line - K`: yaml.v3 attaches a head comment across a blank line, contrary to
  its own field doc, and `config/balance.yaml` is exactly that shape today.
- `FootComment` → the same run, nearest **below** `n.Line`.
- A comment that does not reconcile is an **instrument failure — exit 2** (D6), never a guess and
  never a silent drop. That is what keeps a future yaml.v3 change from turning into wrong line
  numbers instead of a red gate.

Measured over the whole tracked YAML set plus the fixtures, this reconciles every comment yaml.v3
reports, with no unreconciled case, and a naive `n.Line - K` offset does not
[measured `a18467f` · reconciliation probe over every tracked `*.yml`/`*.yaml` →
`UNRECONCILED: none` with the nearest-match search; the same probe with the offset rule →
`HEAD unreconciled` on `config/balance.yaml`, whose header is separated from `world:` by a blank
line]. The rejected alternative was to drop yaml.v3 and take both the text and the line from a
lexical scan bounded by parser-derived block-scalar spans: `yaml.Node` carries `Line` and `Column`
but **no end position**
[measured `a18467f` · `go doc go.yaml.in/yaml/v3.Node` → the fields are `Kind, Style, Tag, Value,
Anchor, Alias, Content, HeadComment, LineComment, FootComment, Line, Column`], so the span of a
folded block scalar would itself have to be re-derived by hand — hand-rolling the one part of YAML
the parser was chosen for.

**D1b — the package survey for the scanner grammars.** `AGENTS.md` § *Dependency Versions*
refuses "it's only 20 lines" and "better to write our own" as arguments, and sets the bar at a
rejected-alternatives comparison naming the escape hatch (the model being KD-4's scheduler). Here it
is; the conclusion is still hand-roll, but now it is argued from what the packages do.

| Grammar | Evaluated | Why it lost |
|---|---|---|
| SQL | `github.com/pressly/goose/v3` — **already a direct dependency**, and the parser that reads these very migrations | Its SQL parser is an `internal/` package, unreachable from this module [measured `a18467f` · `find $(go env GOMODCACHE)/github.com/pressly/goose/v3@v3.27.3 -type d -name '*sqlparser*'` → `…/internal/sqlparser`]. Nothing in goose's public API returns comment positions |
| SQL | `github.com/pganalyze/pg_query_go/v6` | **It answers the question exactly** — `Scan` returns `ScanToken{Start, End, Token}` and the token set carries `SQL_COMMENT` and `C_COMMENT` [measured `a18467f` · `grep -n 'SQL_COMMENT\|C_COMMENT' pg_query.pb.go` → `Token_SQL_COMMENT` and `Token_C_COMMENT` among the `Token` constants; `grep 'type ScanToken struct' -A 8` → the fields `Start int32`, `End int32`, `Token Token`]. It loses on cost, not capability: it is **cgo**, vendoring the Postgres C parser sources into the module [measured `a18467f` · `grep -rl 'import "C"' <module>` → `parser/parser.go`; `grep -rn '#cgo' <module>` → `#cgo CFLAGS: -Iinclude -Iinclude/postgres …` on that file]. Adopting it makes every build and every CI job that compiles this module depend on a C toolchain, to answer a two-marker lexical question about goose migrations. **This is the named escape hatch:** if the SQL scanner is ever wrong on a real migration, this is the drop-in, at that price |
| SQL | `github.com/auxten/postgresql-parser` (pure Go, a CockroachDB fork) | Its public API is `Parse(sql) (Statements, error)`; the scanner discards `--` and `/* */`, and its `COMMENT` identifier is the SQL `COMMENT ON` keyword, not a lexical token [measured `a18467f` · `grep -hn '^func [A-Z]' pkg/sql/parser/parse.go` → `Parse`, `ParseOne`, `ParseQualifiedTableName`, … , none returning comments]. API cannot express the requirement |
| SQL | `github.com/xwb1989/sqlparser` | No tagged release on the proxy, and a MySQL dialect [measured `a18467f` · `go list -m -versions github.com/xwb1989/sqlparser` → the module path alone, no versions] |
| `.gitignore` | `github.com/go-git/go-git/v5` `plumbing/format/gitignore` | Its API is `ReadPatterns` / `ParsePattern` → `Pattern` values; comment lines are skipped and never returned, and no position travels with a pattern [measured `a18467f` · `grep -hn '^func [A-Z]\|^type [A-Z]' <pkg>/*.go` → `ReadPatterns`, `LoadGlobalPatterns`, `LoadSystemPatterns`, `NewMatcher`, `ParsePattern`, `Matcher`, `Pattern`, `MatchResult`]. API cannot express the requirement, and adopting it would pull go-git in for a question it does not answer |
| `.gitignore` | `github.com/sabhiram/go-gitignore`, `github.com/denormal/go-gitignore` | No tagged release on the proxy for either [measured `a18467f` · `go list -m -versions <each>` → the module path alone, no versions]; both are matchers, with the same discards-comments API shape |
| `Makefile` | `4d63.com/makefile`, `github.com/leighmcculloch/go-makefile` | No tagged release on the proxy for either [measured `a18467f` · `go list -m -versions <each>` → the module path alone, no versions], and both expose a target list rather than comment spans. API cannot express the requirement |
| `.env.example` | `github.com/joho/godotenv` — **already a direct dependency**, and the parser that reads this very file (D14) | Its whole public API returns `map[string]string` and `error` [measured `a18467f` · `go doc github.com/joho/godotenv` → `Parse`, `Read`, `Unmarshal`, `UnmarshalBytes`, `Load`, `Overload`, `Marshal`, `Write`, `Exec`]; comments are consumed and discarded with no position. API cannot express the requirement. It nonetheless stays the **grammar of record**: the scanner's rule for where an unquoted value ends is taken from godotenv's own parser, because godotenv is what reads this file in this tree (§ Test Design) |

Those scanners are therefore hand-rolled, and the argument is the one `AGENTS.md` admits — *the
API cannot express the requirement* — for every grammar but SQL, where the argument is a named cost
with a named escape hatch. Each scanner is one marker plus one quoting rule, each is exercised by
its own table in § Test Design, and each is a case where the surveyed package would have had to be
wrapped in a lexical scan anyway.

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

Latest release `v3.14.1`. It is the parser behind `shfmt`, and it is what the requirement in D1
names. Added with `go get` then `go mod tidy`, never by hand-editing `go.mod`, per `AGENTS.md`
§ *Dependency Versions*.

**What it actually costs, measured with a real import rather than a bare `go get`.** Round 2 read
`go get`'s own summary line and reported "a `go.sum` naming only that module"; that command, with
nothing importing the package, records an `// indirect` requirement and answers a different question
than the one asked. Re-measured on a scratch module pinned at this repo's `go` directive, importing
`mvdan.cc/sh/v3/syntax` and tidied, the delta the implementor and the
`go mod tidy && git diff --exit-code go.mod go.sum` gate will actually see is:

- **`go.mod` gains the direct requirement and no transitive `// indirect` line** — the substance of
  round 2's claim survives;
- **`go.sum` gains full `h1:`/`go.mod` entries for the dependency's own test dependencies** —
  `github.com/go-quicktest/qt`, `github.com/google/go-cmp`, `github.com/kr/pretty`,
  `github.com/kr/text`, `github.com/rogpeppe/go-internal` — which `go mod tidy` keeps, because it
  records what `go test all` needs;
- **the `go` directive normalises from `go 1.26` to `go 1.26.0`**, because `mvdan.cc/sh/v3`'s own
  `go.mod` declares `go 1.26.0`. This is not a `go get` artefact: the same command against an
  unrelated module leaves `go 1.26` alone

[measured `a18467f` · scratch module `module shprobe / go 1.26` importing `mvdan.cc/sh/v3/syntax`,
`go get mvdan.cc/sh/v3@v3.14.1 && go mod tidy` → `go: upgraded go 1.26 => 1.26.0`,
`go: added mvdan.cc/sh/v3 v3.14.1`; resulting `go.mod` → `go 1.26.0` plus
`require mvdan.cc/sh/v3 v3.14.1` and nothing else; resulting `go.sum` → the modules named above beside
`mvdan.cc/sh/v3`; control: the same `go get` for `github.com/google/go-cmp@v0.7.0` on an identical
scratch module → `go 1.26` unchanged; `grep -m1 '^go ' <modcache>/…/v3.14.1.mod` → `go 1.26.0`].

Two consequences the wiring must not be surprised by. The `go.mod`/`go.sum` diff of subtask 1 is not
confined to the `require` line, and it is correct — a reviewer seeing the `go.sum` growth should not
"clean it up". And the CI half of the directive change is answered by the workflow, not by this
machine's `go version`: the workflow sets `GOTOOLCHAIN: local` for every job, and each Go job
resolves its toolchain from the very directive the change edits
[measured `bba84c6:.github/workflows/ci.yml` · `grep -n 'GOTOOLCHAIN' .github/workflows/ci.yml` →
`GOTOOLCHAIN: local` in the workflow-level `env:` block ·
`grep -n 'go-version-file' .github/workflows/ci.yml` → `go-version-file: go.mod` on each
`actions/setup-go@v7` step]. Two consequences follow. `GOTOOLCHAIN=local` means **no toolchain
download is ever attempted** during a job, so the directive must be satisfied by whatever
`setup-go` installed; and what `setup-go` installs is read from that same directive, so the two
move together rather than against each other. `actions/setup-go`'s own documentation says it falls
back to the `go` directive and matches by semver, and does **not** state how it resolves a
three-part value [https://github.com/actions/setup-go, fetched 2026-09-08]. That ambiguity is not
load-bearing here: both readings of `1.26.0` — that exact patch, or the newest of the line —
satisfy a `go 1.26.0` directive, and a value `setup-go` could not resolve at all fails its own step
loudly instead of silently selecting a wrong toolchain.

Round 3 discharged this with `go version` on the development machine. That is an adjacent channel:
it answers what a local build does, not what `setup-go` resolves, and the evidence above is
recorded in its place so the citation names the channel it answers.

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

**Membership is read off the tree the gate runs against, never off this document.** The set of
`<pkg>` names is the set of package names in the module at run time, so a package added later is
covered without an edit here — the property the spec asks for by name.

**A gated file with no Go package** — `.env.example`, a `Makefile`, a `*.sh`, a `*.yml`, a `*.sql` —
has nothing that could be "the comment's own package", so every qualifier naming a package of this
module is outside it and is flagged. This is the spec's own reading, and it is the motivating case:
`.env.example`'s header names this module's config loader with a package qualifier.

**A qualifier that names a package of this module *and* a third-party module at once.** `backoff`
is both [measured `4772d33` · `ls internal/` → `backoff` among the entries;
`grep -n 'cenkalti/backoff' go.mod` → `github.com/cenkalti/backoff/v4 v4.3.0 // indirect`]. The
classifier resolves a qualifier against this module's package names only, so it flags `backoff.X`
whichever package the writer meant — including the third-party one, which KD-9 would otherwise
leave alone. The direction is deliberate: over-flagging costs a comment rewrite, under-flagging is
the rot the ban exists to stop. A named classification case carries the decision so that a later
reader does not quietly "fix" it into an import-set lookup (§ Open questions).

The gate does **not** decide the rest of the symbol class — a bare `Type.Field`, or a
package-qualified symbol from the standard library or a third-party module whose name collides with
nothing here. A pattern that caught those would fire on `time.Duration`, `errors.Is` and
`context.Context` in contract prose, which are the most common qualified names in this tree's
comments
[measured `72d5bf7` · a probe over `git ls-files '*.go'` extracting `<lower>.<Upper>` occurrences from
comment text ranked `telego`, `errors`, `backoff`, `time`, `http`, `context` as the leading
qualifiers].

**This is not a narrowing of any acceptance criterion, and round 1's claim that it was has been
settled the other way.** The spec's amended § *Banned reference classes* puts the bare unqualified
name and the outside-module qualifier **outside the banned class itself**, and says in terms that
the gate decides the class in full, which is why AC2 and AC3 do not narrow. What is left for a
reader is AC16's own territory: a bare "see such-and-such", and narration.

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

Exit 0 = no finding. Exit 1 = at least one finding. Exit 2 = the gate could not **decide** the
file — an unreadable file, a parse error, no git worktree, a `.githooks/**` member the router cannot
classify (below), **or a source whose shape an extractor's contract excludes**, of which the one
class today is a YAML source that is not a single plain document (D1a). That last cause is why the
exit is stated as *could not decide* rather than *could not read*: a file can be readable, parseable
and inside a worktree and still sit outside what its extractor promises to place comments in, and
the fail-closed ruling behind D1a puts that case here rather than in exit 0 — where it would be a
gated file passing clean with its comments never looked at. Every cause is an instrument failure
that must not read as a clean tree, the shape `check-citations.sh` already models for its own
high-water mark
[measured `72d5bf7:.claude/skills/ai-audit/scripts/check-citations.sh` ·
`grep -n 'INSTRUMENT reading\|Instrument failure' .claude/skills/ai-audit/scripts/check-citations.sh` →
`# The high-water mark is an INSTRUMENT reading.` and `Instrument failure, not a citation finding`].

**`.githooks/**` is gated as a directory, so the router keys it on content, not on an extension.**
Every other class KD-1 names is keyed by an extension or by a whole file name; `.githooks/**` is the
one keyed by a path prefix, and its members are not all `*.sh`
[measured `bba84c6:ai-docs/plans/2026-09-08-comment-reference-ban.spec.md` § KD-1 ·
`grep -n 'KD-1 — which file classes' ai-docs/plans/2026-09-08-comment-reference-ban.spec.md` → the
class list is the source extensions, the whole file names `.gitignore`, `.env.example` and
`Makefile`, and `.githooks/**`]. Today the directory holds
`coverage-ratchet.sh` beside an extension-less, shebang-bearing `pre-commit`
[measured `bba84c6` · `git ls-files -s .githooks` → `100755 … .githooks/coverage-ratchet.sh` and
`100755 … .githooks/pre-commit`], and after subtask 10 that same tracked entry is a symlink whose
blob is the target path rather than a script `[derived → AC7]`. Neither default a router could fall
into is acceptable: "absent from the extension table ⇒ not gated" silently drops a class KD-1 gates,
which is the exact silent-violation shape this gate exists to stop, and "absent from the extension
table ⇒ exit 2" turns `verify` red the moment subtask 15 wires it. The rule is therefore explicit:

- a path under `.githooks/**` that ends in `.sh` **or** whose content begins with a `#!` shebang
  goes to the shell extractor — in `--staged` mode the content read is the index blob, as for every
  other mode-`--staged` decision above;
- a **symlink entry**, wherever in the gated set it appears, is **skipped, and the skip is
  reported** rather than silent. A link carries no comment of its own, and its target is itself a
  tracked `*.sh` scanned under its own path `[derived → AC7]`, so nothing is dropped; skipping it is also what stops
  one body being reported twice, once under each name. The router recognises the link by its index
  mode in `--staged` mode — a staged symlink is recorded as mode `120000`
  [measured `bba84c6` · scratch repository, `ln -s real.sh link && git add link && git ls-files -s`
  → `120000 … link` beside `100755 … real.sh`] — and by `lstat` in worktree mode;
- anything else under `.githooks/**` is an **instrument failure — exit 2**, because the gated set
  names a file the gate cannot decide, and loud beats silent at that boundary. Nothing is in that
  branch today or after the change — every shebang-bearing tracked file outside `.githooks/` already
  ends in `.sh`
  [measured `bba84c6` ·
  `git ls-files | while read -r f; do head -c2 "$f" | grep -q '#!' && echo "$f"; done` →
  the `ai-docs/scripts/`, `.claude/skills/*/scripts/` and `.githooks/` scripts, with
  `.githooks/pre-commit` the only extension-less member] — and AC8 is what keeps it that way
  `[derived → AC8]`.

`git` is invoked with a fixed argument vector and no shell. `gosec` may still flag the subprocess;
if it does, the suppression is a specific `//nolint:gosec` with a stated reason, which is what
`nolintlint`'s `require-specific` and `require-explanation` settings demand, and the config's only
`gosec` exclusion is for test files
[measured `4772d33:.golangci.yml` · `sed -n '39,57p' .golangci.yml` →
`nolintlint: require-explanation: true, require-specific: true`, and `exclusions.rules` naming
`gosec` only under `path: _test\.go`].

Who calls the command, with which argument vector, and what happens when the toolchain is absent is
D16.

### D7 — Wiring: `Makefile`, CI, pre-commit — in two landings, not one

The wiring splits by what it judges, and the split is what keeps the tree green across the whole
task (§ Approach):

**Landing one, with the code-side sweep.** `Makefile` gains a `comment-refs` target running the
command over the whole tracked set, and `.githooks/pre-commit` becomes a symbolic link to a new
`.githooks/pre-commit.sh` that runs the reference gate over the staged set before `exec`ing the
ratchet (D8, D16). Both judge only what a commit stages, so neither can be made red by a file no
commit touches. Having the target exist early also gives the instructions/harness group a real
command to cite and to run over its own sweep.

**Landing two, last of all.** `verify` gains `comment-refs` in its prerequisite list (AC9), and
`.github/workflows/ci.yml` gains a **new filter key** naming every path class the gate covers — the
Go, shell, SQL, YAML, `Makefile`, `.gitignore`, `.env.example` and `.githooks/**` classes — and a
**new job** gated on it that sets up Go and runs `make comment-refs` (AC3). Neither existing filter
key covers the set: the `go` key does not name `**/*.sh`, the `harness` key does not name `**/*.go`,
and **neither names `.githooks/**`**
[measured `4772d33:.github/workflows/ci.yml` · `sed -n '37,61p' .github/workflows/ci.yml`].
No skill's `allowed-tools` line needs editing for the new target — the grants are wildcard
[measured `72d5bf7` · `grep -rno "Bash(make[^)]*)" .claude/skills/*/SKILL.md .claude/agents/*.md` →
`Bash(make *)` in `task/SKILL.md`, `pr-ci-failed/SKILL.md`, `project-review/SKILL.md` and their siblings].

### D8 — The pre-commit dispatcher, and why it must be able to do nothing

Today `.githooks/pre-commit` is a regular file that `exec`s the ratchet
[measured `72d5bf7:.githooks/pre-commit` · `git ls-files -s .githooks/pre-commit` → `100755 c78cd82… .githooks/pre-commit`].
After the change the tracked entry is a symlink and the content lives in `pre-commit.sh`, which
keeps resolving its own paths from the worktree root — the property the existing dispatch suite
locks
[measured `72d5bf7:ai-docs/scripts/test-precommit-dispatch.sh` · `cat -n ai-docs/scripts/test-precommit-dispatch.sh` →
`[ -x .githooks/pre-commit ]` and `grep -q 'git rev-parse --show-toplevel' .githooks/pre-commit`].
Both assertions survive a symlink, because `grep` and `[ -x ]` follow one.

That suite builds a throwaway repository by copying `.githooks` into a sandbox, and `cp -r`
preserves a symlink on this toolchain. This is external-tool behaviour rather than a fact about a
file in the tree, so the pin fixes the checkout the probe ran against, not a coordinate inside it
[measured `bba84c6` · scratch probe under the session scratchpad: a directory holding `real.sh` and
a symlink `link -> real.sh`, `cp -r src dst` → `ls -l dst` lists `link -> real.sh`; the same
directory staged in a throwaway repository → `git ls-files -s` reports mode `120000` for `link`].
So the sandbox inherits the new shape, and the tracked entry subtask 10 creates is recorded as a
link rather than as a second copy of the script. But that sandbox has no Go module
and stages no gated path, and a dispatcher that unconditionally ran the gate there would refuse
every fixture commit and turn the suite red. The dispatcher therefore has named skip conditions and
prints each one — D16 states them, and states the seam that lets the suite exercise a real refusal
anyway.

### D9 — One `--help` shape, applied identically (KD-17, AC19)

**The property AC19 is about is that nothing happens before `--help` is answered**: no file
written, no `git` invoked, no measurement taken. Round 1 prescribed a *position* — immediately after
the `set` line — and then carved out the one script that cannot take it, which made the rule
contradict its own exception. The property replaces the position:

- a script with no argument handling yet gains the `case` below, immediately after the `set` line;
- a script that already parses an argument extends that existing site — `coverage-ratchet.sh` reads
  a mode there
  [measured `4772d33:.githooks/coverage-ratchet.sh` · `grep -n '^mode=' .githooks/coverage-ratchet.sh`
  → `mode=${1:-raise}`], preceded only by constant assignments, which are not side effects.

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
[measured `72d5bf7:ai-docs/scripts/doc-edit-guard.sh` · `sed -n '1,45p' ai-docs/scripts/doc-edit-guard.sh` →
`usage() { sed -n '2,32p' "$0" | sed 's/^# \{0,1\}//'; exit 1; }`].

The fixed line `  -h|--help) usage; exit 0 ;;` is the shape marker the checker of D10 greps for.

**Why the block is copied into each script instead of sourced from a shared helper.** The
duplication is far past the `≥ 3`-site threshold at which this subagent's charter demands a shared
unit rather than copy-paste, so the argument is owed rather than assumed. Call sites: the block
lands in every script of the D10 set, and that set — derived at sweep time from the merge base, not
listed here — is a subset of the tracked `*.sh` that carry usage prose in a comment today,
`≥20` (verified `git ls-files '*.sh' | xargs grep -lE '^[[:space:]]*#.*([Uu]sage|[Rr]unnable)'`,
spanning `ai-docs/scripts/`, `.claude/skills/*/scripts/` and `.githooks/`).

The alternative — a sourced library, `ai-docs/scripts/lib/usage.sh` or similar — loses on a
measured property of this corpus, not on "minimal surface" or "no new file". **No script here
resolves its own location.** Every tracked `*.sh` that resolves anything resolves the *repository
root*, through `git rev-parse --show-toplevel`; none uses `BASH_SOURCE` or `dirname "$0"`, and none
sources a sibling file at all
[measured `bba84c6` · `git ls-files '*.sh' | xargs grep -ln 'BASH_SOURCE\|dirname "\$0"'` → no
output · `git ls-files '*.sh' | xargs grep -nE '^[[:space:]]*(\.|source)[[:space:]]+'` → no output ·
the same scan for `show-toplevel` → the `ai-docs/scripts/` guards, both skill suites and
`.githooks/coverage-ratchet.sh`]. A library would therefore have to be reached either from the repo
root — which makes every guard consult `git` before it can answer `--help`, destroying the one
property AC19 is about, that nothing happens first (D9 names "no `git` invoked" among the things
that must not) — or from a new `BASH_SOURCE` prologue in every script, which is the same
duplication moved one level down, plus a resolution failure mode the corpus does not have today. It also breaks the one place a guard already runs outside this tree:
`test-precommit-dispatch.sh` copies `.githooks/` into a throwaway repository and runs the hook
there, where a sibling library outside that directory does not exist
[measured `bba84c6:ai-docs/scripts/test-precommit-dispatch.sh` ·
`grep -n 'cp -r' ai-docs/scripts/test-precommit-dispatch.sh` →
`cp -r "$repo_root/.githooks" "$sut/.githooks"` and the same copy into `"$control"`]. And the
scripts answer to callers with different cwd contracts — `Makefile`, CI `run:` steps, a
`.claude/settings.json` hook body, and the pre-commit hook
[measured `bba84c6` · `grep -n 'coverage-ratchet' Makefile` → `.githooks/coverage-ratchet.sh
--check` · `grep -n 'bash ai-docs/scripts' .github/workflows/ci.yml` → the Harness-guards steps ·
`grep -n 'check-review-register' .claude/settings.json` → a hook body running
`script=ai-docs/scripts/check-review-register.sh` by repo-root-relative path].

What is shared is only the dispatch: `usage()`'s body states a different invocation grammar in every
script `[derived → AC17, AC18]`. The drift the charter warns about is answered by gating rather than
by trust — D10's checker greps the fixed marker line over the tracked tree, and subtask 12's
completion condition is byte-identity of that line across every script that answers `--help`
`[derived → AC19]`.

**AC19 spans a group boundary, so the shape is pinned to an artefact rather than to a memory.**
`coverage-ratchet.sh` takes this shape in subtask 8 (Group A, code change-type — `.githooks/**`);
every other script takes it in subtask 12 (Group B, instructions/harness). Two implementors on two
models writing "the same shape" from a description is how shapes drift. The obligation is therefore
directional: **subtask 12 copies the block subtask 8 wrote, verbatim**, reading it out of the tree
rather than out of this document, and subtask 12's completion condition includes that the marker
line is byte-identical across every script that answers `--help`, `coverage-ratchet.sh` included.
Subtask 13's checker then holds that forward for every later script `[derived → AC19]`.

### D10 — The `--help` membership set is derived twice, from two different trees

The set that owes a `--help` is "every tracked `*.sh` whose comments **carried** usage prose", and
after the sweep no comment carries any, so the criterion is unavailable in the post-sweep tree
`[derived → AC20]`. Therefore:

- the sweep and the Step-9 verifier each derive the set from the **merge base with `main`**
  (`git show <base>:<path>`), never from a list in this document, exactly as the spec requires.
  AC17 and AC18 are established there, at sweep time, and are re-derivable by the same recipe;
- what survives forward is a **checker plus its regression suite**, the pair every guard in this
  repository is built as
  [measured `4772d33` · `ls ai-docs/scripts` → `check-ac-shape.sh` beside `test-ac-shape.sh` and
  `check-spec-shape.sh` beside `test-spec-shape.sh`; `ls .claude/skills/ai-audit/scripts` →
  `check-citations.sh` beside `test-check-citations.sh`]:
  - `ai-docs/scripts/check-script-shape.sh` asserts, over the tracked tree, the properties that hold
    of any later tree: every script answering `--help` carries the D9 shape marker; `--help` exits 0,
    prints a non-empty block and runs nothing else; and — AC8 — every tracked file beginning with a
    `#!` shebang either ends in `.sh` or is a symbolic link to one.
  - `ai-docs/scripts/test-script-shape.sh` is its regression suite, built from sandbox fixtures that
    must each be flagged.

**AC8 is folded into the checker rather than left ungated.** Round 1 left it as a verifier's
condition and listed the gap as an open question; it is one pass over `git ls-files`, over a tree the
checker already walks, and an ungated condition over the tree is the thing that rots first.

That pair is also the design's answer to the spec's third open question. `AGENTS.md` sets the bar at
"any file with ~50+ lines of substantial logic"
[measured `72d5bf7:AGENTS.md` § *Workflow* · `grep -n '50+ lines' AGENTS.md` →
``- Any file with ~50+ lines of substantial logic MUST have tests (`_test.go` beside it).``]; the
checker clears it not by the flag's size but by the set's — its conditions span every script in the
tree and rot the moment one is added.

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
subtask's paths, and the review-judged classes (D4, AC16) are clear in the diff". No per-file site
tally is written into this document: a tally is true for one commit, the implementor and the verifier
measure it anyway, and the gate is a better contract than a number, because it is re-runnable. The
truncating-gate caveat in this subagent's own rules — where "N sites" is a floor because the gate
stopped printing — is answered by the gate printing every finding and capping nothing
`[derived → the command scenarios in § Test Design]`.

**No gated path may fall between two subtasks.** The completion condition of the *last* sweep
subtask of each group is the gate's silence over **the whole gated set minus the paths a later
subtask owns**, not merely over the paths its own row names. A gated file that no row names — the
Files columns are a plan, not an inventory — is therefore swept by whichever subtask's silence
condition first reaches it, instead of surviving to the wiring subtask and turning `verify` red.

Where a sentence exists only to carry a removed reference, the sentence goes with it; outside
`config/**` and `.env.example` the fact is lost, by the owner's answer. Note that the *failure
messages* these guard scripts print are not comments and are therefore untouched — a good deal of
the rationale the sweep strips from `check-ac-shape.sh`'s header survives in the message it prints
on a hit
[measured `4772d33:ai-docs/scripts/check-ac-shape.sh` · `sed -n '100,122p' ai-docs/scripts/check-ac-shape.sh`
→ the `MSG` heredoc printed to stderr, which names the learnings-log date and the spec-writer rule
the header also names].

### D13 — `config/**` gains prose (AC11)

`config/balance.yaml` is today the only comment-bearing file under `config/` — the directory's only
other tracked entry cannot carry a comment
[measured `72d5bf7` · `git ls-files config/` → `config/balance.yaml`, `config/world/.gitkeep`] — and
its per-key documentation is largely a design-section pointer, in Russian
[measured `72d5bf7:config/balance.yaml` · `cat -n config/balance.yaml`]. Every **leaf** key ends
up with English prose that says what the key controls and what changing it does, at the
completeness `.env.example` reaches per variable; a group key gains prose when the group needs an
introduction. The placeholder status of the numbers is a fact about the file and survives, restated
without the issue number and without the design-section pointer that currently carries it. No value
changes.

**Where that prose comes from, since this document is not the source.** The pointer being deleted
is, for most keys, the only carrier in the tree of what the key means, so the order of operations is
part of the subtask: for each leaf key the implementor **reads the `docs/DESIGN.md` section the
key's own pointer names, and reads it before stripping that pointer**, then writes the English prose
from what that section says the key controls; the pointer is removed in the same edit that replaces
it, never in an earlier pass. For a leaf key that carries no pointer today — several do not — the
source is the section covering that key's mechanic, and the file-level frame is `docs/DESIGN.md`
§16.5, which is what the header's own pointer names
[measured `bba84c6:config/balance.yaml` · `grep -nE '#.*(DESIGN|§)' config/balance.yaml` → the
per-key comments are `docs/DESIGN.md §…` pointers, some of them carrying a clause of prose beside
the pointer, and the header's pointer is §16.5 · `cat -n config/balance.yaml` → leaf keys including
`cap`, `step_cost` and `backpack_ttl` carry no comment at all]. `docs/DESIGN.md` is Russian and the replacement is English (KD-12), so this is a
translation of substance into self-contained prose, not a transcription of wording — and the
subtask is routed to the code group by its change-type, so the reading is stated here rather than
assumed `[derived → AC11]`.

### D14 — `.env.example` (AC12)

Its per-variable prose already meets the bar. What goes is the header's package name, its test path,
its markdown path and its issue number
[measured `72d5bf7:.env.example` · `cat -n .env.example`], the per-key `design D10` / `D13` /
`D15` anchors, and the bare `internal/…` paths that head three of its sections. What stays, restated
in its own words (KD-15), is the file's contract: a variable the configuration loader does not read
does not belong here.

Two things stay that round 1 listed among the deletions or left ambiguous, both corrected by the
spec's narrowing:

- **`GetUpdatesParams.Limit` survives as written.** It names a field of a third-party type, and the
  amended spec puts it outside the banned symbol class by name
  [measured `4772d33:.env.example` § `LAB_GAME_INGEST_BATCH_LIMIT` · `grep -n 'GetUpdatesParams' .env.example`
  → `# GetUpdatesParams.Limit. The Bot API accepts values between 1 and 100.`]. Round 1 listed it as
  a symbol the gate catches; that was written before the narrowing.
- **The cross-key constraint on the long-poll window survives**, because it names a sibling key of
  the same file, which is not outward.

The key set must still match the loader's, which is asserted by an existing test that parses this
file with the same library that would parse a real `.env`
[measured `4772d33:internal/config/disjoint_test.go` · `grep -n 'godotenv.Read' internal/config/disjoint_test.go`
→ `m, err := godotenv.Read(repoRootPath(t, ".env.example"))`].

### D15 — Rule text and propagation (AC13, AC14)

`ai-docs/doc-convention.md` is rewritten, not retired (KD-11): § DOC-4 inverts from "cite the
section" to the ban, carrying the banned-class table, the exemptions, the record that the narration
half and the bare-unqualified-name half are review-judged (KD-13, AC16), and the record that shell
in workflow `run:` blocks and in `.claude/settings.json` hook bodies obeys the rule without a gate
(Scope item 7). § DOC-3 and § DOC-5 stand.

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
(`.claude/skills/task/reference.md` § Step 2, `AGENTS.md` § *Build & Test*), the `AGENTS.md`
§ *Build & Test* claim about what CI shellchecks, which D17 makes true rather than edits away, and
the declared sync groups a new gate command and a new CI job trip (`ai-docs/propagation-groups.md` —
the Review group, the gate-command row, the `/task` verify-list row, the CI group's per-class
reproducer tables, and the `ci.yml`-job row; plus the generic obligation to update
`ai-docs/claude-tools-hierarchy.md` for a new tool contract).

One tension worth naming, because a reader will otherwise re-open it: the `…Unchecked` AXIOM
requires a doc comment to name the guarantor, and a guarantor in another package of this module
cannot be named under the ban. The spec settles this — KD-9 and § *Source conflicts* item 4 keep both
sections unchanged — and it is not live today, because the tree holds no such function
[measured `72d5bf7` · `ast-index search "Unchecked"` → `No results found`; `git ls-files '*.go' | xargs grep -n Unchecked` → no output].
The design records the reading rather than reopening it: where a future guarantor is cross-package,
the comment states the precondition and describes the guarantor without a package-qualified symbol.

### D16 — How the dispatcher invokes the gate, and the seam that lets its suite see a real refusal

Round 1 left the invocation unstated and the guard conditions unsatisfiable in the one place that
asserts them. Both are settled here.

**The invocation.** `.githooks/pre-commit.sh` runs `go run ./cmd/commentrefs --staged` from the
worktree root it already resolves. Not a `$PATH` lookup and not a pre-built binary: a stale binary is
a gate reporting on code that is not the code being committed, and `go run ./cmd/…` is the form this
project already documents for its own commands
[measured `4772d33:AGENTS.md` § *Build & Test* · `grep -n 'go run ./cmd/bot' AGENTS.md` →
`go run ./cmd/bot   # run the bot …`].

**Three named skip conditions, each printed to stderr**, in the loud-skip direction the ratchet
already takes for a missing toolchain: no staged path of the gated set; no `go.mod` at the worktree
root; no `go` on `$PATH`. A skip prints its reason and returns 0. A gate that ran returns its own
status, and the dispatcher exits with it *before* reaching the ratchet — exit 2 (instrument failure,
D6) refuses the commit exactly as exit 1 does, because an instrument that could not decide is not a
clean tree.

**The seam.** The dispatch suite's sandbox becomes a repository that satisfies the first two
conditions on its own terms: it writes an **untracked** `go.mod` and an **untracked**
`cmd/commentrefs/main.go` whose exit status a fixture file controls and which records its argument
vector in a marker file. Untracked matters twice — nothing about the stub is ever staged, so the
coverage ratchet keeps taking its "nothing that can move coverage is staged" exit and the suite stays
fast, and the fixtures stage a `*.sh` probe file, which is gated by KD-1 and is not a ratchet
trigger. No production override exists: an environment variable that replaced the gate command would
be a gate an agent can switch off, which `AGENTS.md` refuses on the same grounds as `--no-verify`.

**What this proves, and what it does not.** It exercises the dispatcher's real invocation form, its
three skip conditions, and its exit-code propagation in both directions. It does **not** prove the
gate's verdict — that is `cmd/commentrefs`'s own Go tests over a scratch repository (§ Test Design).
Saying so is the point: round 1's version proved neither, silently, which is the green-instrument
shape `AGENTS.md` § *Patterns* 2 names.

The sandbox needs a Go toolchain. Locally there is one; CI's Harness-guards job sets none up — its
steps run straight from the checkout
[measured `4772d33:.github/workflows/ci.yml` · `sed -n '129,155p' .github/workflows/ci.yml` →
the job's `steps:` begin with `uses: actions/checkout@v7` and the next entry is the shellcheck step].
So the wiring subtask adds `actions/setup-go@v7` with `go-version-file: go.mod` to that job: a case
that skips in CI is a case that does not run, and a job that did not run is not a passing job.

### D17 — CI shellchecks every tracked script (KD-18, AC24)

KD-18 leaves the shape to the design. Two were open; the design takes the second.

- **The `Makefile` target already reaches the whole tree.** It prunes `.git` and `tmp` and
  shellchecks every `*.sh` beneath the root, `.githooks/` included
  [measured `4772d33:Makefile` § shellcheck target · `sed -n '/^shellcheck:/,/^$/p' Makefile` →
  `find . -path ./.git -prune -o -path ./tmp -prune -o -name '*.sh' -exec shellcheck -s bash {} +`
  followed by `shellcheck -s bash .githooks/pre-commit`]. CI's own step reaches two directories
  [measured `4772d33:.github/workflows/ci.yml` · `grep -n "find .claude ai-docs/scripts" .github/workflows/ci.yml`
  → `find .claude ai-docs/scripts -name '*.sh' -print0 | xargs -0 -r shellcheck -s bash`].
- **So the Harness-guards job runs `make shellcheck`** instead of carrying its own expression.
  Extending the CI expression would leave two spellings of one gate — the drift the `Makefile`'s own
  header names as the reason CI invokes sub-targets
  [measured `4772d33:Makefile` · `sed -n '1,15p' Makefile` → "CI never runs `verify` — it
  invokes the same sub-targets from its paths-filtered jobs, so a local run and a CI run cannot
  disagree about what any gate's command is"] — and the gap being closed is precisely what the second
  spelling caused.
- **The header's own carve-out goes with it.** That same block records shellcheck as a gate "CI
  reaches by another route", because the Harness-guards job "keeps its inline step". After this
  change actionlint is the only such gate. The `Makefile` header is a comment in a gated file that
  the sweep rewrites anyway, so restating it belongs to the wiring subtask rather than to a separate
  propagation row.
- **The target's second line goes too.** `shellcheck -s bash .githooks/pre-commit` exists because
  that file has no `.sh` extension. After the symlink, `find` reaches the content through
  `pre-commit.sh`, and `-name '*.sh'` does not match the link's own name, so the line becomes a
  re-check of a file already checked.
- **The filter.** `.githooks/**` joins the `harness` filter key, which is the second half of AC24 —
  a commit touching only that directory currently matches no filter and runs no job at all. `Makefile`
  joins it as well, because the job's command now lives in that file; a filter key may name a path
  that another key also names, as `.github/workflows/**` already is
  [measured `4772d33:.github/workflows/ci.yml` · `sed -n '37,61p' .github/workflows/ci.yml` →
  `.github/workflows/**` under both the `go` and the `workflows` keys].

## Decomposition

| # | Task | Files | Depends on |
|---|------|-------|------------|
| 1 | Comment extraction: the file-class router and one extractor per grammar (Go, shell, YAML, SQL, `Makefile`, `.gitignore`, `.env.example`), each returning marker-stripped text with a line number — the YAML one refusing any source that is not a single plain document and reconciling the rest, both per D1a, since yaml.v3 reports the node's line and not the comment's; the router keys `.githooks/**` on content per D6, since that class alone is not extension-keyed — `.sh`-or-shebang to the shell extractor, a symlink entry skipped with the skip reported, anything else exit 2; add the `mvdan.cc/sh/v3` requirement via `go get` + `go mod tidy`, expecting the `go.sum` growth and the `go`-directive normalisation D3 measures | `internal/commentref/` (extractors + tests), `go.mod`, `go.sum` | — |
| 2 | The banned-class classifier (D4) and the exemption pass (D5), over extracted comments | `internal/commentref/` (classifier + tests) | 1 |
| 3 | The command: the input modes and exit codes D6 names, the `<file>:<line>: <class>: <text>` report, a testable `run` with a thin `main` | `cmd/commentrefs/` | 2 |
| 4 | Sweep `cmd/bot`, `internal/config`, `internal/backoff` to the gate's silence (D12) | `cmd/bot/*.go`, `internal/config/*.go`, `internal/backoff/*.go` | 3 |
| 5 | Sweep `internal/store` and its migrations, and `internal/testdb` | `internal/store/*.go`, `internal/store/migrations/*.sql`, `internal/testdb/*.go` | 3 |
| 6 | Sweep `internal/tg` and `internal/tgtest` | `internal/tg/*.go`, `internal/tgtest/*.go` | 3 |
| 7 | Sweep `internal/ingest` and `internal/scheduler` | `internal/ingest/*.go`, `internal/scheduler/*.go` | 3 |
| 8 | Sweep the build and runtime gated files; clean the workflow's block-scalar comments by hand (cleaned, not gated — KD-6); give `coverage-ratchet.sh` the D9 `--help` | `.golangci.yml`, `.github/workflows/ci.yml`, `.github/dependabot.yml`, `Makefile`, `.gitignore`, `.githooks/coverage-ratchet.sh` | 3 |
| 9 | The two files that gain prose rather than lose it: `.env.example` strips its pointers and restates its file-level contract (D14); `config/balance.yaml` gets English self-contained prose per leaf key, values untouched (D13) | `.env.example`, `config/balance.yaml` | 3 |
| 10 | Landing one of the wiring (D7): `Makefile` gains the `comment-refs` target — **not** yet a `verify` prerequisite; `.githooks/pre-commit` becomes a symlink to a new `.githooks/pre-commit.sh` dispatching gate-then-ratchet with the D16 invocation and skip conditions | `Makefile`, `.githooks/pre-commit`, `.githooks/pre-commit.sh` | 4–9 |
| 11 | Rewrite `ai-docs/doc-convention.md` to the new rule (D15, AC13) | `ai-docs/doc-convention.md` | 10 |
| 12 | Sweep the harness shell scripts to the gate's silence; move usage prose behind the D9 `--help`, **copying the block subtask 8 wrote verbatim out of the tree** — completion includes the marker line being byte-identical across every script that answers `--help`, `coverage-ratchet.sh` included (D9) | `ai-docs/scripts/*.sh`, `.claude/skills/*/scripts/*.sh` | 10 |
| 13 | The script-shape checker and its regression suite (D10, AC8); extend the dispatch suite with the symlink case and the D16 dispatch cases | `ai-docs/scripts/check-script-shape.sh`, `ai-docs/scripts/test-script-shape.sh`, `ai-docs/scripts/test-precommit-dispatch.sh` | 12 |
| 14 | Propagate the rule text across the instruction surface and the hook messages, per D15 and the `grep -rni` sweep; AC22's grammar sites; the tool-hierarchy and propagation-group rows for the new gate, job, checker and suite; rewrite the #68 body to the reformulated rule (AC23) | `AGENTS.md`, `ai-docs/*.md`, `.claude/agents/*.md`, `.claude/skills/**/*.md`, `.claude/settings.json`, issue #68 | 11, 12, 13 |
| 15 | Landing two of the wiring (D7, D17): `verify` gains `comment-refs`; the CI filter key and the `comment-refs` job; the Harness-guards job runs `make shellcheck`, gains `actions/setup-go@v7`, gains the new checker's step and the new suite's line; `.githooks/**` and `Makefile` join the `harness` filter; the `Makefile`'s `pre-commit` special case and its shellcheck carve-out note go | `Makefile`, `.github/workflows/ci.yml` | 14 |

### Which subtask owns which acceptance criterion

| AC | Owned by |
|---|---|
| AC1 | subtasks 4–9 and 12 for the classes the gate decides; the bare-name half is review-judged with AC16 |
| AC2 | subtask 10 (the dispatcher and its invocation, D16), asserted by subtask 13's dispatch cases |
| AC3 | subtask 15 (the filter key and the job) |
| AC4 | subtask 3 (the report format) |
| AC5 | subtask 1 (the Go and SQL extractors) |
| AC6 | subtasks 1–3 for the Go gate, subtask 13 for the new shell guards — each run over its own files as a completion condition |
| AC7 | subtask 10 (the symlink and the dispatcher), asserted by subtask 13 |
| AC8 | subtask 10 makes it true; subtask 13's checker gates it (D10) |
| AC9 | subtask 15 (`verify` reaches the target), held green by every subtask's own gate run |
| AC10 | subtasks 4–7 — `golangci-lint run` with `revive` unchanged is what refuses a removed doc comment, so the criterion is gated, not merely reviewed |
| AC11 | subtask 9 |
| AC12 | subtask 9 |
| AC13 | subtask 11 |
| AC14 | subtask 14 |
| AC15 | subtasks 4–9 and 12, under the carve-out the criterion itself now carries |
| AC16 | subtasks 4–9 and 12, review-judged against the diff (KD-13) |
| AC17 | subtask 8 for `coverage-ratchet.sh` and subtask 12 for the harness scripts, both derived from the merge base (D10) |
| AC18 | subtask 12, derived from the merge base (D10) |
| AC19 | subtasks 8 and 12 write the shape; subtask 13's checker holds it forward |
| AC20 | subtasks 8 and 12, gated by the `repo-path` class once the prose moves behind `--help` |
| AC21 | subtasks 8 and 12; `make shellcheck` clean is part of each one's completion condition |
| AC22 | subtask 14, under the reading in D11 |
| AC23 | subtask 14 |
| AC24 | subtask 15 (D17) |

## Handoff plan

Grouping is required for every `M ≥ 1` (a), a group holds at most `10` consecutive subtasks (b), the
handoff destination is `/context-reset` per `.claude/skills/context-reset/SKILL.md` § *Compaction
recovery (re-entry)* (c), the terminal group's size lies in `1..=10` (d), every group is homogeneous
by change-type (e), the group count is minimized subject to the cap and the dependency order (f),
each group is marked with its implementor model and effort (g), and the count is within the default
maximum of `4` (h).

**Change-type assignment used here.** *Code*: `*.go`, `*.sql`, `go.mod` / `go.sum`, and the build
and runtime artefacts — `Makefile`, `.github/**`, `.golangci.yml`, `.gitignore`, `.env.example`,
`config/**`, `.githooks/**`. *Instructions/harness*: `*.md`, `.claude/**` (its scripts and
`settings.json` included), `ai-docs/**` (its scripts included), `AGENTS.md`, and the tracking issue's
body. The directory rule for the last two is (e)'s own: it names `.claude/**` and `ai-docs/**` as
instructions/harness whatever a file's extension inside them is.

- **Handoff into Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § *Compaction recovery (re-entry)* before the first subtask, as the every-group contract requires.
- **Group A** — model `sonnet`, effort `medium` (pinned) via the `code-writer` subagent, 1M-token
  window — subtasks 1–10 (code change-type). The gate, the code-side sweep, the `comment-refs`
  target and the pre-commit dispatcher. At the size cap of `10`.
  **Its own completion condition for subtask 10**, whose durable regression does not exist until
  Group B: the dispatcher is exercised in a scratch repository in both directions — a staged gated
  file carrying a banned reference refused, a clean one accepted, and each of D16's three skip
  conditions printed — and the probe recorded in the progress file. Group B converts that probe into
  the suite; the group does not close on "it looks right".
- **Handoff after Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § *Compaction recovery (re-entry)*. Parent `/task` resumes in Group B with fresh context.
- **Group B** — model `inherit` (the orchestrator's), effort inherited from the orchestrator
  (typically xHigh) — NOT pinned — via the `general-purpose` subagent with no inline `model=`,
  1M-token window — subtasks 11–14 (instructions/harness change-type). The doc-convention rewrite,
  the harness shell sweep and its `--help` migration, the checker and the suites, the propagation
  sweep and the issue body.
- **Handoff after Group B:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § *Compaction recovery (re-entry)*. Parent `/task` resumes in Group C with fresh context.
- **Group C** — model `sonnet`, effort `medium` (pinned) via the `code-writer` subagent, 1M-token
  window — subtask 15 (code change-type). Terminal group (1 subtask; within the `1..=10` range).

**Why the count is three and cannot be two.** Subtasks 1–10 are one group already at the cap, so
nothing more fits in it. Subtasks 11–14 are a change-type switch. Subtask 15 switches back, and it
cannot be folded into Group A: it is what makes the whole tree the gate's subject, and every tracked
`*.sh` is still dirty until subtask 12 (§ Approach). It cannot join Group B either — different
change-type. Three groups, within the default maximum of `4`, so no user approval is needed for the
count.

## Risks

- The gate's own source flags itself: a doc comment on an exported classifier item that spells out
  what it matches ("matches a path ending in .md") is a banned reference in a gated file. Mitigation:
  the class names and patterns live in string constants and test fixtures, never in prose, and the
  gate is run over its own package as part of the subtask's completion condition —
  `[derived → AC6]`.
- A qualifier that names a package of this module and a third-party module at once is flagged for
  both readings (D4). Mitigation: the direction is conservative — the escape is to write the sentence
  without the qualifier — and a named classification case records the decision so it is changed
  deliberately or not at all — `[derived → the classification cases in § Test Design]`.
- Group B's commits are made with the pre-commit gate already wired (it lands in subtask 10), so a
  half-cleaned harness script cannot be committed. That is the gate working, but it makes Group B's
  commit granularity per-file rather than per-batch. Mitigation: stated here so the implementor
  stages what it has cleaned — `[derived → AC2]`.
- A YAML comment in a position `go.yaml.in/yaml/v3` does not attach to any node would be a silent
  false negative — the failure mode `AGENTS.md` § *Patterns 2* names, where a clean instrument reads
  as a clean subject. Mitigation: the extraction tests carry a fixture per comment position (head,
  line, foot, between mappings, after the last node, inside and after a block scalar), each asserted
  present or absent **and, when present, at an exact line** —
  `[derived → the YAML extraction cases in § Test Design]`.
- The sibling of that risk, and the one round 2 missed: a YAML comment reported at the **wrong
  line**. yaml.v3 reports the node's line, not the comment's, so a report can be confidently wrong
  rather than absent, and a presence-only test table passes either way (D1a). Mitigation: the
  reconciliation is a nearest-match search over the source text with a mismatch escalating to
  exit 2, and the extraction cases assert exact lines including the three shapes an offset rule
  gets wrong — `[derived → the YAML extraction cases in § Test Design]`.
- The YAML narrowing (D1a) turns a legal file into a red gate: a `---`-headed or directive-carrying
  YAML source joining the gated set later exits 2, so `verify` and the CI job go red over a file
  whose content is fine. This is the owner's fail-closed ruling and its accepted cost, not a defect.
  What bounds it: the refusal names the line and the token where it fires, D1a states both escapes,
  and no tracked YAML carries either shape today
  [measured `8371fb4` · `git ls-files '*.yml' '*.yaml' | xargs grep -nE '^(---|\.\.\.|%)'` → no
  output, over a non-empty file list].
- `mvdan.cc/sh/v3` fails to parse a script that `bash` accepts, and the gate exits 2 on a file it
  cannot read. Mitigation: exit 2 is an instrument failure by design (D6), not a pass; the extraction
  suite parses every tracked `*.sh` of the tree and asserts no parse error —
  `[derived → the shell extraction cases in § Test Design]`.
- The dispatch suite goes green for the wrong reason once the dispatcher can legitimately skip
  (D8, D16): every fixture there stages `README.md` only
  [measured `72d5bf7:ai-docs/scripts/test-precommit-dispatch.sh` § `commit_from` ·
  `grep -n 'README.md' ai-docs/scripts/test-precommit-dispatch.sh` → every `git add` in the file
  names `README.md` and nothing else], so a gate call could be deleted with the suite still passing.
  Mitigation: the D16 seam — a stub at the real invocation path whose verdict the fixture controls,
  a staged `*.sh` probe so the gated-path condition is met, and paired refuse/accept cases —
  `[derived → AC7 and the dispatch cases in § Test Design]`.
- The dispatch suite's new cases need a Go toolchain and would otherwise skip everywhere it is
  missing, CI included. Mitigation: D17's `actions/setup-go@v7` step on the Harness-guards job, and
  the skip is loud rather than silent wherever it does fire — `[derived → AC7, AC24]`.
- Subtask 14 writes instruction-file rows describing a `verify` prerequisite and a CI job that do not
  exist until subtask 15. Mitigation: the `comment-refs` target lands in subtask 10 precisely so the
  prose has a real command to cite, and subtask 15's completion condition includes that every command
  name and job name those files now cite resolves in the tree — `[derived → AC13, AC14, AC9]`.
- `gosec` refuses the `git` subprocess in `--staged` mode. Mitigation: fixed argument vector, no
  shell; if the finding stands, a specific `//nolint:gosec` with a reason, which is what the lint
  config requires
  [measured `4772d33:.golangci.yml` · `sed -n '39,57p' .golangci.yml` → `nolintlint: require-explanation: true, require-specific: true`].
- The coverage ratchet blocks the commit because the new package arrived under-covered. Mitigation:
  D2 keeps `main` thin and the logic in `internal/commentref`, and the subtasks are TDD'd — the
  legitimate exits from a ratchet block are stated in `AGENTS.md` § *Build & Test*, and `--no-verify`
  is not among them — `[derived → AC9]`.
- The sweep strips rationale that a later reader will want and that no gate will restore. This is
  the owner's decided cost, not a defect; it is bounded by the fact that a script's printed failure
  messages are not comments and are untouched, and by `config/**` and `.env.example` gaining prose
  instead — `[derived → AC11, AC12, AC15]`.

## Test Design

All of it is new, so every claim here is `[derived → …]` except where a grammar's behaviour was
measured before being specified. Go tests are table-driven subtests beside the code, per
`ai-docs/go-test-conventions.md`; the shell suites follow the shape of the guard suites already in
`ai-docs/scripts/`.

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
  **Every YAML case asserts the exact line, not presence alone** — the whole D1a reconciliation is
  invisible to a by-name table, which is the green-instrument shape one heading over from where this
  design's own YAML risk row was looking. Cases present because a rule reading `n.Line` or
  `n.Line - K` off the node would get them wrong: a multi-line file header separated from its node
  by a **blank line** (the `config/balance.yaml` shape); a head comment whose node sits below it;
  and a foot comment whose node is a mapping **key** whose subtree ends above the comment. A further
  case asserts that a comment which cannot be reconciled surfaces as **exit 2**, not as a guessed
  line and not as a silent drop.
  A second YAML table covers the accepted source shape of D1a, and it is where the refusal is proven
  rather than assumed: an unindented `---` alone, `---` carrying content, `---` and `...` each
  followed by a comment separated **by a space and by a tab**, a bare `...`, a `%YAML` directive, and
  a comment standing above the first marker — each asserted to fail with the **named sentinel**,
  never to return a partial read of the stream. The separator pair is that table's instrument check:
  a refusal keyed on the separator instead of on the token passes every space form and drops every
  tab form, so the two must be asserted side by side. Cases carrying a banned reference beyond the
  marker are what tell a refusal apart from a silent pass, since a partial read returns exactly those
  clean. One further case takes the other branch of D1a: a source the parser yields no node for —
  comments alone, and comments with blank lines between them — asserted to report every `#` line at
  its own line rather than nothing, because a nodeless branch returning nothing is invisible to every
  other case in the table.
- SQL: `--` at line start and mid-line; `--` inside a single-quoted string; a `/* */` block; a goose
  annotation.
- `Makefile`: line-start and mid-line `#`; a `#` in a recipe line.
- `.gitignore`: a line whose first character is `#` reported; a `#` **after** a pattern asserted
  **absent**, because git takes it as part of the pattern rather than as a comment marker — a
  `.gitignore` holding `foo # bar` does not ignore `foo`
  [measured `4772d33` · scratch repository, `.gitignore` holding `foo # bar`, `git check-ignore -v foo`
  → no output, exit 1]; a leading `\#` asserted absent, since it escapes a pattern beginning with a
  hash. Round 1 grouped this file with `Makefile` and `.env.example` under "line-start and mid-line
  `#`", which would have baked a false positive into the extractor.
- `.env.example`: a line-start `#` reported; a `#` preceded by whitespace after an **unquoted** value
  reported, because that is where the file's own parser ends the value
  [measured `4772d33` · `grep -n 'a comment (ie asdasd # some comment)' "$(go env GOMODCACHE)/github.com/joho/godotenv@v1.5.1/parser.go"`
  → the backwards scan that ends an unquoted value at a `#` preceded by whitespace]; a `#` inside a
  quoted value asserted **absent**. That library is the grammar to match because it is what reads
  this file in this tree (D14).
- Router: each gated path shape maps to its extractor; a path outside the gated set is reported as
  not gated rather than silently skipped; and the `.githooks/**` branch of D6, which is where the
  gated set stops being extension-keyed — an extension-less shebang-bearing file routed to the shell
  extractor; a **symlink** entry skipped with its skip reported, asserted in **both** `--staged` and
  worktree mode, and paired with a case asserting the link's target is still reported under its own
  path, so "skipped" cannot silently become "ungated"; and an extension-less, non-shebang,
  non-symlink file under `.githooks/` escalating to **exit 2** rather than passing as clean.
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
reason, so a later widening has to change a named case rather than a regex quietly; a comment in a
**non-Go** gated file carrying a package-qualified symbol of this module, caught, since no package is
its own there; and the collision case of D4 — a qualifier that is both an `internal/` package here
and a third-party module name — asserted **caught**, with the reason in the case name.
`[derived → AC1 and AC4]`

**Command — `cmd/commentrefs` (subtask 3).**
Entry point: `run(args, stdout, stderr) int`.
Scenarios: a clean file exits 0 and prints nothing; a dirty file exits 1 and prints file, line, class
and matched text; an unreadable path exits 2; explicit-paths mode; whole-tree mode; `--staged` mode
driven against a scratch repository where the index and the worktree **disagree**, asserting the
index blob is what was judged. This is where the gate's own verdict is proven; the dispatch suite
below deliberately does not attempt it.
Fixtures: a scratch repository built with `t.TempDir()` and fixed `git` invocations.
`[derived → AC2 and AC4]`

**Script shape — `ai-docs/scripts/check-script-shape.sh` and its suite (subtask 13).**
The checker asserts over the tracked tree: every script answering `--help` carries the D9 shape
marker; `--help` exits 0, prints a non-empty block and runs nothing else; every tracked file
beginning with `#!` ends in `.sh` or is a symbolic link to one (AC8).
`ai-docs/scripts/test-script-shape.sh` is its regression suite, over sandbox fixtures: a conforming
script; a script answering `--help` with a different shape; a script whose `--help` runs its body
first; a shebang file with neither a `.sh` name nor a symlink; and — the instrument check — a
sandbox in which every fixture conforms, asserted to produce no finding, so a checker that reports
nothing under all conditions cannot pass its own suite.
`[derived → AC8, AC17, AC18, AC19, AC21]`

**Pre-commit dispatch — `ai-docs/scripts/test-precommit-dispatch.sh` (subtask 13).**
Added to the existing cases, on the D16 seam: the tracked entry is recorded as a symlink whose target
is a `.sh` file in the same directory; with the stub gate refusing, a sandbox commit staging a gated
`*.sh` probe is refused and the stub's stderr reaches the caller; with the stub accepting, the same
commit succeeds; with only a non-gated path staged, the stub's marker file shows it was never
invoked; with the stub module absent, the "no `go.mod`" skip is printed and the commit proceeds; the
existing ratchet-removed instrument case still refuses. The refuse/accept pair and the
never-invoked case together are what stop this suite from passing with the dispatcher's gate call
deleted — `[derived → AC7]`.

**Everything the sweep touches (subtasks 4–9, 12).**
No new test. The completion condition is the gate's silence over the subtask's paths (D12) plus the
existing suites staying green — `make verify`, and for the harness scripts their own regression
suites, which must behave identically after their comments change.

## Open questions

- **The `module-symbol` collision** (D4). The gate flags `backoff.X` whether the writer meant this
  module's `internal/backoff` or the third-party module of the same name, because the classifier
  resolves qualifiers against this module's package names only. The design takes that over-flagging
  direction and pins it with a named test. If the owner would rather the gate resolve the qualifier
  against the file's own import set, that is a widening of subtask 2 and a further design round, not
  a Step-9 fix.
- **Two of round 1's open questions are closed by the spec, not by this design, and should not be
  reopened at Step 9.** AC15 now carries its carve-out inside the criterion, naming the wiring, the
  symlink, the `--help` scripts, `config/**`, `.env.example` and the shellcheck-gap files as outside
  it. AC2 and AC3 are not narrowed by D4, because the amended § *Banned reference classes* puts the
  bare unqualified name and the outside-module qualifier outside the banned class itself. Recorded
  here so a later reader does not re-derive them from round 1's text.
- **Round 1's third open question is now in scope, not open.** CI shellchecking less than
  `AGENTS.md` claims became Scope item 11, KD-18 and AC24; D17 chooses the shape and subtask 15 owns
  it.
