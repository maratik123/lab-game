# Design: Bot API transport over telego — retries, `retry_after`, rate limits, base-URL axis

**Issue:** #19
**Date:** 2026-09-04

> **Claim-tag conventions in this document.** A repo fact carries the commit it was read
> at — the sha in a tag is the commit of the **read**, not of this document, so it may lag
> `HEAD` after a revision that touched only this file. A fact about an **external module, an
> external tool, the standard library, or a published web page** has no repo path, so its pin
> is the **version** (or the `go doc` target, or the page plus its fetch date) the probe ran
> against; the probe modules live outside the repo, under the session scratchpad. A claim
> about an artefact this task has **not yet built** carries `[derived → …]` and no locator.
> `docs/DESIGN.md` is cited by section per the design-writer contract, so those citations
> carry no `[measured …]` tag.

---

## Approach

`internal/tg` is a thin layer whose **entire policy lives below telego's generated API**,
in one implementation of telego's own `telegoapi.Caller` interface. telego's `Bot` builds
the URL and the request body from its generated types and then hands both to that one
method; everything this task owns — the retry loop, the `retry_after` wait, both
limiters, the outbound gate seam #22 needs, and the per-call observation — happens inside
it. **Across the generated-method surface — every `Bot.SendMessage`-shaped method, which is
the whole Bot API — nothing routes around it, and nothing below it exists.** That is the
guarantee at the strength it actually holds; it is *not* unconditional, because telego's
exported options can be reapplied to a bot from outside. D2 measures that escape and
specifies the guard that closes it.

That choice falls out of reading the pinned library rather than assuming it. telego's
`Caller` is a single method taking the request URL and the marshalled body
[measured telego@v1.11.2:telegoapi/api.go · `sed -n '/^\/\/ Caller represents/,/^}/p' …/telegoapi/api.go` → `Call(ctx context.Context, url string, data *RequestData) (*Response, error)`],
its stock `net/http` implementation throws the HTTP status code away and reports any 5xx
as an untyped `fmt.Errorf`
[measured telego@v1.11.2:telegoapi/caller.go · `sed -n '/func (h HTTPCaller) Call/,/^}/p' …/telegoapi/caller.go` → `if response.StatusCode >= http.StatusInternalServerError { return nil, fmt.Errorf("internal server error: %d", response.StatusCode) }`],
and `telegoapi.Response` carries no status field at all
[measured telego@v1.11.2:telegoapi/api.go · `sed -n '/^type Response struct/,/^}/p' …/telegoapi/api.go` → fields `*Error`, `Ok`, `Result`]. So the health surface `docs/DESIGN.md` §13.2
asks for — latency and response codes by method, 429 and retry counters — **cannot** be
observed from above the stock caller. Replacing the caller is not an optimisation here;
it is the only place the required facts exist.

The whole flow, end to end, was executed against the pinned version before this document
was written: a probe wired a custom caller and a custom `encoding/json` request
constructor into `telego.NewBot`, pointed it at a fake server, and sent one `sendMessage`
[measured telego@v1.11.2 · scratchpad probe `p2`, `go run .` →
`server saw path: /bot123456:…/sendMessage body: {"chat_id":-1001234567890,"text":"hi"}` /
`server chat_id token: -1001234567890` / `status: 429 method: sendMessage wrote: true` /
`retry_after: 7`]. That single run establishes the load-bearing mechanisms D2, D4, D5 and
D7 rely on.

**Rejected — wrapping each Bot API method by hand in `tg.Client`.** It would put the
method class and chat id in our hands directly, but it hand-writes what KD-2 chose telego
to generate ("coverage is complete by construction"), and every method #22, #43 or a
later mechanic wants must then be wrapped again. D4 shows both facts are recoverable
inside the caller with no per-method code.

**Rejected — passing the method class and chat id down through `context.WithValue`.** It
works, but it makes every call site responsible for tagging, so a caller that forgets the
tag silently escapes the per-chat limiter — the exact "exempt by construction" failure the
spec's private-chat row refuses. D4's derivation cannot be forgotten.

**Rejected — using telego's own `RetryCaller`.** It exists
[measured telego@v1.11.2:telegoapi/caller.go · `grep -n 'type RetryCaller' …/telegoapi/caller.go` → `type RetryCaller struct {`],
but it is helper-layer behaviour KD-2 excludes, it retries on *any* unknown error
including one where the request was written and unanswered
[measured telego@v1.11.2:telegoapi/caller.go · `sed -n '/func (r \*RetryCaller) handleError/,/^}/p' …/telegoapi/caller.go` → `if !errors.As(err, &apiErr) { return 0, true // Unknown error }`],
and that is precisely the "never on doubt" rule the owner set in round 1.

---

### D1 — telego is pinned at `v1.11.2`, and the newest release is unreachable on this toolchain

`go get github.com/mymmrac/telego@v1.12.1` **fails** here: the release declares `go 1.26.7`
while the installed toolchain is `go1.26.5` with `GOTOOLCHAIN=local`
[measured telego@v1.12.1 · scratchpad probe, `go get github.com/mymmrac/telego@v1.12.1` in a
`go 1.26` module → `go: github.com/mymmrac/telego@v1.12.1 requires go >= 1.26.7 (running go 1.26.5; GOTOOLCHAIN=local)`;
`go version` → `go version go1.26.5-X:nodwarf5 linux/amd64`; `go env GOTOOLCHAIN` → `local`].
`v1.12.0` declares the same minimum
[measured · `curl -sS https://proxy.golang.org/github.com/mymmrac/telego/@v/v1.12.0.mod` → `go 1.26.7`],
and `v1.11.2` declares `go 1.25.7`
[measured · `curl -sS https://proxy.golang.org/github.com/mymmrac/telego/@v/v1.11.2.mod` → `go 1.25.7`].
`GOTOOLCHAIN: local` is also CI's own setting
[measured 31736b4:.github/workflows/ci.yml:16-17 · `sed -n '16,17p' .github/workflows/ci.yml` → `env:` / `  GOTOOLCHAIN: local`],
so this is not a local-machine quirk to design around.

**Decision: pin `v1.11.2`.** Verified against this module's real graph in a scratch copy of
the tree: the resolution succeeds, `go build ./...`, `go vet ./...` and every test binary
compile
[measured 31736b4 + telego@v1.11.2 · scratchpad copy of `git archive HEAD`,
`go get github.com/mymmrac/telego@v1.11.2 && go mod tidy` → `GET-TIDY-OK`;
`go build ./...` → `BUILD-OK`; `go vet ./...` → `VET-OK`;
`go test -run XXXNONEXISTENT ./...` → `TESTCOMPILE-OK`].

**Consequences the implementor must not be surprised by.**

- **The resolution upgrades `github.com/stretchr/testify` from `v1.11.1` to `v1.12.1`,
  indirect.** telego requires it
  [measured 31736b4 + telego@v1.11.2 · same scratch copy, `go get …@v1.11.2` →
  `go: upgraded github.com/stretchr/testify v1.11.1 => v1.12.1`; `grep -n 'stretchr/testify' go.mod` →
  `github.com/stretchr/testify v1.12.1 // indirect`]. The compile check above already covers
  the existing suites; the `go.mod`/`go.sum` delta belongs in the PR body.
- **`go mod tidy` deletes telego again if nothing imports it yet.** Measured on the same
  scratch copy: after `go get` with no importing file, `go mod tidy` removed the
  requirement outright
  [measured 31736b4 + telego@v1.11.2 · scratch copy, `go mod tidy` then `grep -c 'mymmrac/telego' go.mod` → `0`],
  and it became a **direct** requirement only once a file imported it
  [measured · same copy with an importing file, `go get … && go mod tidy` then `grep -n 'mymmrac/telego' go.mod` →
  `github.com/mymmrac/telego v1.11.2` inside the direct `require` block]. `make tidy-check`
  would therefore fail a standalone "add the dependency" subtask. **The decomposition binds
  the `go get` to the first importing file for exactly this reason** — see § Decomposition
  subtask 3.

*Reason for the dependency* (`AGENTS.md` § Dependency Versions, and AC3's requirement that
the design state it): KD-2 already chose it; this task is where the choice first becomes
code. AC3's remaining clauses — a **direct** requirement at a **released** version with
`go.sum` in agreement and no `go mod tidy` delta — are what the importer-binding above
protects, and subtask 3's gate is `make tidy-check` itself.

**Open for the owner, not blocking:** raising the local toolchain to ≥ 1.26.7 unlocks
`v1.12.x`. Recorded in § Open questions.

### D2 — The transport is one `telegoapi.Caller`; `tg.Client` owns configuration, not calls

```
telego.Bot (generated methods)  →  tg.caller.Call(ctx, url, data)  →  net/http
                                     ├─ gate (D12)            — once per call
                                     └─ attempt loop:
                                          ├─ limiters (D9)    — once per ATTEMPT
                                          ├─ backoff (D6), retry_after (D7)
                                          └─ (after the loop) observation (D11)
```

**The limiters are charged per attempt, not per call, and the nesting above is the whole
statement of that choice.** A retried call issues more than one request to Telegram, and the
flood ban this package exists to prevent is counted in *requests*, not in caller-visible
calls: charging once per call would let a saturated global schedule admit its configured rate
in calls while emitting a multiple of it in requests. Backoff and `retry_after` already
space the attempts of a *single* call, but they say nothing about many concurrent calls each
retrying at once — which is exactly the incident shape. Per-attempt charging costs nothing
in the common path (a call that succeeds first time is one attempt, one charge, so AC10's
"calls" and "requests" coincide) and is the strictly safer accounting. Recorded explicitly
because the implementor would otherwise settle it silently.

`tg.Client` holds the configured `*telego.Bot`, the limiter registry, the retry settings,
the gate and the observer; it exposes the bot through one accessor. Every outbound call in
the project is a call on that bot, and every one of them lands in our caller.

**The accessor's guarantee, stated at the strength it actually holds.** Across the
*generated-method surface* — every `Bot.SendMessage`-shaped method, which is the whole Bot
API — there is no path that skips the caller. That is what AC27's second clause needs, and
it preserves KD-2's "complete coverage by construction" without wrapping a single method.

**It is not an unconditional guarantee, and an earlier draft of this document wrongly said
it was.** `telego.BotOption` is exported as a plain function type
[measured telego@v1.11.2:bot.go:86-87 · `grep -n -B2 '^type BotOption' bot.go` →
"// BotOption represents an option that can be applied to [Bot]" / `type BotOption func(bot *Bot) error`],
and `WithAPICaller` merely assigns the unexported field
[measured telego@v1.11.2:bot_options.go · `sed -n '/^\/\/ WithAPICaller/,/^}/p' bot_options.go` →
`return func(bot *Bot) error { bot.api = caller; return nil }`], so any holder of the returned
pointer can apply an option to a bot it did not construct. Executed rather than inferred: a
probe built a bot with one caller, applied `telego.WithAPICaller(other)(bot)` from outside,
and sent a message
[measured telego@v1.11.2 · scratchpad probe `optesc`, `go run .` →
`original caller reached: false` / `replacement caller reached: true`]. The gate, both
limiters and the observation are all bypassed on that path.

**Why the design accepts it, and what closes it.** `bot.api` is unexported, so the only
lever is telego's own `telego.With*` options and `telego.NewBot` — a *closed, greppable*
surface. lab-game is an application with no downstream importers (`AGENTS.md` § API
Stability), so this module is the entire population of code that could hold the pointer:
a guard test asserting that no non-test file outside `internal/tg` names `telego.NewBot` or
any `telego.With*` option is therefore a **complete** cover for the reachable escape, not a
sampling of it. That guard is subtask 6's
[derived → AC27's tests: a refusing gate blocks a call issued through the accessor; the
package exposes no constructor for a bot without the caller; and no non-test file outside
`internal/tg` references `telego.NewBot` or a `telego.With*` option].

The alternative that would close it in the type system — never handing out `*telego.Bot`
and wrapping each method instead — is the option D2 already rejected above, and it trades a
complete guard for hand-written coverage of a generated API. Recorded so the trade is
visible rather than implied.

Exported shape [derived → AC1]:

- `Client`, `New(Options) (*Client, error)`, `(*Client).API() *telego.Bot`
- `Options` — base URL, bot token, the `config.Transport` settings (which now carry the
  per-attempt timeout as well as the retry and limiter values), optional `Gate`, optional
  `Observer`, optional jitter source, optional `*http.Client`, optional
  `Logger telego.Logger`
- `MethodClass`, `Call`, `ChatRef`, `Gate`, `Observation`, `Observer`, `Error`

**`Options.Logger` exists so telego's own logger is a decision rather than a default.**
Left alone, `telego.NewBot` installs a logger that writes to `os.Stderr` with error printing
on
[measured telego@v1.11.2:logger.go · `sed -n '/func newDefaultLogger/,/^}/p' logger.go` →
`Out: os.Stderr`, `DebugMode: false`, `PrintErrors: true`, `Replacer: defaultReplacer(token)`],
and `performRequest` calls `b.log.Errorf` on every error our caller returns — so every
give-up and every terminal failure would also leave the process by a channel D11 does not
own, which is not something to hand #23 unannounced. **AC26 is not at risk either way**: the
default logger redacts the token
[measured telego@v1.11.2:logger.go · `sed -n '/func defaultReplacer/,/^}/p' logger.go` →
`strings.NewReplacer(token, DefaultLoggerTokenReplacement)`, and
`grep -n -A2 'DefaultLoggerTokenReplacement =' logger.go` → `= "BOT_TOKEN"`]. The issue is
channel ownership, not leakage. **Default: `telego.WithDiscardLogger()`** — the typed error
and the observation point are the transport's output, and a duplicate ANSI stderr stream is
noise in a container. An operator or a test that wants telego's own tracing sets
`Options.Logger`.

Every outbound-call method is telego's own and already takes `ctx` first
[measured telego@v1.11.2:methods.go · `grep -n -A2 'func (b \*Bot) SendMessage' methods.go` →
`func (b *Bot) SendMessage(ctx context.Context, params *SendMessageParams) (*Message, error)`];
`Client` stores no `context.Context` in any field [derived → AC1].

`New` **validates** its options and returns an error naming the offending field rather than
falling back to a compiled-in value — the defaults live in `internal/config` (D10) and
having a second set here would be the two-sources-of-truth defect KD-24 exists to prevent.
No `panic`, no `log.Fatal`, no `must…`: the panic index is empty and this task keeps it so
[measured 31736b4:ai-docs/panic-index.md · `sed -n '/^| File:line/,$p' ai-docs/panic-index.md` →
the header row followed by `| — | — | — |`].

### D3 — The `net/http` + `encoding/json` swap: what the pinned version expresses, and the one residue

`docs/DESIGN.md` §11 and KD-2 record the swap as natively supported
[measured 31736b4:ai-docs/key-decisions.md:11 · `grep -n 'KD-2 —' ai-docs/key-decisions.md` →
"Defaults `fasthttp`/`go-json` are replaced with `net/http` and `encoding/json` (supported
swap)."]. Against `v1.11.2` that
statement resolves into **a runtime option for one half and a build tag for the other**, and
the design says so plainly rather than implying a single lever.

- **HTTP — a runtime option, fully taken.** `WithAPICaller` replaces the caller wholesale
  [measured telego@v1.11.2:bot_options.go · `grep -n '^func With' bot_options.go` →
  `WithAPICaller(caller ta.Caller)`, `WithHTTPClient(client *http.Client)`, `WithRequestConstructor(constructor ta.RequestConstructor)`, `WithAPIServer(apiURL string)`, …].
  Our caller is `net/http` throughout.
- **JSON — split.** The **request** side is a runtime option: `WithRequestConstructor`
  installs our `encoding/json` marshaller
  [measured telego@v1.11.2:telegoapi/api.go · `sed -n '/^\/\/ RequestConstructor represents/,/^}/p' …/telegoapi/api.go` →
  `JSONRequest(parameters any) (*RequestData, error)` and `MultipartRequest(...)`].
  The **response envelope** is decoded inside our caller, so it is ours too. What remains is
  telego's decode of the already-extracted `result` payload into its own generated type,
  inside `Bot.performRequest`
  [measured telego@v1.11.2:bot.go · `sed -n '/func (b \*Bot) performRequest/,/^}/p' bot.go` →
  `unmarshalErr = json.Unmarshal(response.Result, &vs[i])`, over the import
  `"github.com/mymmrac/telego/internal/json"`], and that package selects its backend by
  **build tag**, not by option
  [measured telego@v1.11.2:internal/json/lib.std.go · `cat …/internal/json/lib.std.go` →
  `//go:build stdjson && !sonic` / `Marshal = json.Marshal` over `import "encoding/json"`;
  README → "No tags - use goccy/go-json … `stdjson` - use `encoding/json`"].

**Decision: take the runtime half; do not introduce the `stdjson` build tag in this task.**

The tag works and does what it claims — measured, not assumed: with it, `grbit/go-json`
disappears from the build graph entirely
[measured telego@v1.11.2 · scratchpad probe `p2`, `go list -deps .` → the `github.com/grbit/go-json*`
packages present; `go list -tags stdjson -deps .` → no `grbit/go-json` package, `encoding/json` present;
`go build -tags stdjson` → `BUILD-TAG-OK`], and `run.build-tags` is a valid key in this
repo's lint configuration schema
[measured 31736b4:.golangci.yml + golangci-lint 2.13.1 · the tracked config with a
`run.build-tags: [stdjson]` insertion, `golangci-lint config verify --config <copy>` → exit `0`].

What refuses it is the **cost of adopting it correctly**. A build tag on the module changes
what every gate command is, and `AGENTS.md` § Propagation Rule routes a gate-command change
to `AGENTS.md` § Build & Test, every skill's `allowed-tools`, and the `/task` gate checklist
[measured 31736b4:ai-docs/propagation-groups.md · `grep -n 'A gate command' ai-docs/propagation-groups.md` →
"| A gate command (adding, removing, or renaming one) | `AGENTS.md` § *Build & Test* AND every skill's `allowed-tools` line that grants it AND `.claude/skills/task/reference.md` § *Gate checklist* |"].
The live surfaces spelling those commands out bare are not a handful — a sweep over the
tracked instruction corpus, excluding history surfaces, names `AGENTS.md`,
`.claude/settings.json`, most of `.claude/skills/**`, most of `.claude/agents/**`, and
several `ai-docs/**` pages
[measured 31736b4 · `rg -n --hidden --glob '!.git/**' --glob '*.md' --glob '*.json' --glob '*.yml' --glob '!ai-docs/plans/done/**' --glob '!ai-docs/learnings.md' --glob '!ai-docs/harness-gaps.md' --glob '!ai-docs/plans/2026-09-04-bot-api-transport*' 'go (build|test|vet) \./\.\.\.|go test -race' .` →
`AGENTS.md`, `ai-docs/{claude-tools-hierarchy,code-style,dependency-versions,key-decisions}.md`,
`ai-docs/templates/progress-format.md`, `.claude/settings.json`,
`.claude/agents/{code-writer,design-writer,review-findings,self-review,spec-writer}.md`,
`.claude/skills/{bugfix,context-reset,dependabot-pr,main-ci-failed,pr-ci-failed,pr-commented,project-review,task,verify-change}/**`,
and `docs/DESIGN.md` — the last being the Russian design corpus, which this rule never edits.
**The `--hidden` flag is load-bearing:** the same `rg` without it returns none of
`.claude/**`, which is most of the list — an instrument that looked clean because it could
not see the directory that matters].
Threading a tag through all of them is a harness-wide
change nobody asked this task for, and half-threading it produces a silently divergent
second build configuration — the shape `AGENTS.md` § Patterns 2 warns about.

**What the residue actually is, stated so it is not mistaken for a gap in the swap:**
telego decoding *its own generated result types* with a drop-in reimplementation of
`encoding/json`'s API. Every byte this project marshals or unmarshals itself goes through
`encoding/json`. `fasthttp` and `fastjson` remain in the *module graph* either way —
`bot.go` and `internal/json/common.go` import them unconditionally, tag or no tag
[measured telego@v1.11.2 · scratchpad probe `p2`, `go list -tags stdjson -deps .` →
`github.com/valyala/fasthttp` and `github.com/valyala/fastjson` still listed] — so no
build configuration removes them, and AC2's testable clause is about **this project's own
imports**, which name neither [derived → AC2's import-scan test].

Recorded as an owner decision in § Open questions with the one-PR path, because the
alternative reading ("the swap is only half done") deserves an explicit answer rather than
silence.

### D4 — Method name and chat key are derived inside the caller, from the URL and the body

telego composes the URL as base + `/bot<token>/<method>`
[measured telego@v1.11.2:bot.go · `sed -n '/func (b \*Bot) constructAndCallRequest/,/^}/p' bot.go` →
`url = b.apiURL + botPathPrefix + b.token + "/" + methodName`], so `path.Base` of the parsed
URL path is the method name — observed live in the probe (`method: sendMessage`). The same
function has a `useTestServerPath` branch that inserts a `/test/` segment before the method,
and `path.Base` still yields the method name from it, so the rule needs no special case for
the test-server path.

The chat is read from the request body by decoding **only** the `chat_id` field as a
`json.RawMessage` — also observed live (`server chat_id token: -1001234567890`). Every
chat-scoped method names its destination chat in that one field, so one rule covers the whole
JSON-bodied surface. A `chat_id` may be a `@channelusername` string rather than a number, so
the key is the raw JSON token, not an `int64` — one stable key per chat either way.

**Two branches produce no chat id, and they are not the same branch.** An earlier draft
called this rule "total across the Bot API by construction", which was wrong in a way that
mattered:

- **No `chat_id` in the body.** An inline-message edit, `getMe`, `getUpdates`. There is
  genuinely no destination chat, so no per-chat schedule applies. Its `Chat.Target` is
  `ChatNone` and the call charges its class-global schedule only. Correct, and not a gap.
- **No body to read at all — `BodyRaw == nil`.** telego routes a request through
  `MultipartRequest` whenever a parameter carries a file, and that constructor returns a
  `RequestData` whose `BodyStream` is an `io.Pipe` and whose `BodyRaw` is **nil**; the
  parameters, `chat_id` among them, are written into the pipe by a goroutine
  [measured telego@v1.11.2:telegoapi/api.go · `sed -n '/^\/\/ RequestData represents/,/^}/p' …/telegoapi/api.go` →
  `BodyRaw []byte` and "BodyStream body stream that will be read, ignored if BodyRaw is provided";
  telego@v1.11.2:telegoapi/request_constructor.go · `sed -n '/func (d DefaultConstructor) MultipartRequest/,/^\t}$/p' …/request_constructor.go` →
  `pr, pw := io.Pipe()` then `&RequestData{ContentType: writer.FormDataContentType(), BodyStream: pr}`
  with no `BodyRaw`, against `JSONRequest` which sets `BodyRaw`]. The chat id is therefore
  **not reachable from `RequestData`** without consuming the stream the request needs.

**Left undecided, that branch is a chat exempt from every per-chat window by construction** —
a `sendPhoto` would charge the class-global schedule and neither the per-chat rate nor the
per-chat cap. That is the failure this design's own private-chat row calls "a flood-ban path
no test would notice", and `ai-docs/domain-invariants.md` § 6 exists to prevent it. So it is
decided here rather than at implementation time.

**Decision: a call with no decodable body is charged against a single reserved unknown-chat
key of its class, and its `Chat.Target` is `ChatUnknown` — distinct from the `ChatNone` of a
method that addresses no chat at all (D12).**

- It is **bounded, never exempt**: such calls share one per-chat schedule, so the class's
  rate and cap both apply to them. Sharing one key across destinations is conservative — it
  over-restricts if several such calls ever went to different chats — and conservative is the
  right direction for a bound whose failure mode is a flood ban.
- `ChatUnknown` is carried to the gate (D12), so #22 can refuse a call whose destination it
  cannot check against `ALLOWED_CHAT_IDS` **without also refusing `getMe` or `getUpdates`,
  which report `ChatNone`**. This task installs no allowlist; it makes refusing exactly this
  case *expressible*, which is its obligation.
- The branch is **exercised by a test even though the MVP never reaches it**, so it is not an
  untested path [derived → the classifier and limiter scenarios in § Test Design].

**Recorded residue, not built now.** The owner's ruling this round is that media is outside
the MVP — #43's notifications are text and inline buttons — so no media mechanism is designed
here, and the spec already anticipated the question rather than needing amendment. The route a
later media mechanic should take is signposted: **our own `RequestConstructor.MultipartRequest`
receives `parameters map[string]string`, `chat_id` among them**, before telego writes them
into the pipe, so the chat id is recoverable at that seam without parsing a stream. Building
that hand-off is the media mechanic's work, not this task's.

**What this buys that a per-call-site tag cannot.** It cannot be forgotten by a
caller, and it covers methods this project has not written yet — including everything #22
and #43 will add.

**The token is in that URL, and the method name is the only thing extracted from it.** No
observation carries the URL, and no error carries it either — but that second half is *made
true by a named mechanism*, not merely asserted: the standard library would otherwise put the
whole URL into every transport error, so D8 specifies the sanitisation and AC26's test
verifies it [derived → AC26, and D8's sanitisation mechanism].

**That statement is about this package's outputs, not about the reachable surface — an
earlier draft over-claimed it as the latter.** `Client.API()` hands out a `*telego.Bot`, and
telego exposes the token by design through `Token()`, and inside a URL through
`FileDownloadURL`
[measured telego@v1.11.2:bot.go:117-119,158-161 · `sed -n '117,121p;156,162p' bot.go` →
`// Token returns bot token` / `func (b *Bot) Token() string { return b.token }`, and
`func (b *Bot) FileDownloadURL(filepath string) string` returning
`b.apiURL + "/file/bot" + b.token + "/" + filepath`]. AC26 is not breached — it scopes to
error messages, instrumentation observations and fixtures, none of which these are — and the
exposure is in-module only, since nothing outside `internal/tg` may construct or reconfigure
a bot (D2's guard). It is recorded here as an **accepted in-module exposure** rather than
left as a false negative, and subtask 6's token sweep is told about both methods so a future
reader does not mistake a legitimate hit for a leak.

**Method classes** — the enum is `ClassMessage`, `ClassEdit`, `ClassOther`, and the
classifier is a prefix rule on the method name, verified against the pinned version's own
method list:

| Rule | Class |
|---|---|
| `send*`, `copyMessage*`, `forwardMessage*` | `ClassMessage` |
| `editMessage*`, `editEphemeralMessage*`, `deleteMessage*`, `deleteEphemeralMessage*`, `stopMessageLiveLocation`, `stopPoll` | `ClassEdit` |
| anything else | `ClassOther` |

The prefix form is chosen for `send*` because it is **fail-safe**: a Bot API method added
later that delivers a message is throttled by default rather than escaping the limiter. The
`edit`/`delete` side is spelled out to the `Message` segment on purpose — the bare prefixes
would sweep in `editChatInviteLink`, `editForumTopic`, `deleteWebhook`, `deleteMyCommands`
and more, which are not message operations
[measured telego@v1.11.2:methods.go · the method-name set extracted with
`grep -oE 'performRequest\(ctx, "[a-zA-Z]+"' methods.go | sed -E 's/^.*"([a-zA-Z]+)"$/\1/' | sort -u`,
then filtered by prefix → `edit*` yields `editChatInviteLink editChatSubscriptionInviteLink editEphemeralMessage{Caption,Media,ReplyMarkup,Text} editForumTopic editGeneralForumTopic editMessage{Caption,Checklist,LiveLocation,Media,ReplyMarkup,Text} editStory editUserStarSubscription`;
`delete*` yields `deleteAllMessageReactions deleteBusinessMessages deleteChatPhoto deleteChatStickerSet deleteEphemeralMessage deleteForumTopic deleteMessage deleteMessageReaction deleteMessages deleteMyCommands deleteStickerFromSet deleteStickerSet deleteStory deleteWebhook`;
`send*` yields only message-delivering names plus `sendChatAction`, `sendGift`, `sendMessageDraft`,
`sendChatJoinRequestWebApp`].

`sendChatAction` is left inside `ClassMessage` rather than carved out: bounding it is the
safe direction, the MVP sends none, and a carve-out list is a second thing to keep correct.

`exhaustive` is enabled with `default-signifies-exhaustive: true`
[measured 31736b4:.golangci.yml:39-41 · `sed -n '39,41p' .golangci.yml` → `  settings:` /
`    exhaustive:` / `      default-signifies-exhaustive: true`], so a `switch` over `MethodClass`
is either total or carries a `default`; the classifier itself switches on the method-name
shape, not on the enum.

### D5 — "Provably never reached Telegram" is `httptrace.WroteRequest`, and the classifier defaults to ambiguous

An attempt's outcome is classified as:

| Evidence | Retryable? | `Ambiguous` |
|---|---|---|
| HTTP response, status 429 **with** `retry_after` | yes, after `retry_after` (D7) | no |
| HTTP response, status 429 **without** `retry_after` | yes, after the D6 backoff | no |
| HTTP response, status ≥ 500 | yes | no |
| HTTP response, any other non-success or `ok:false` | no — terminal | no |
| HTTP response, `ok:true` | success | — |
| No response, and no successful request write | yes | no |
| No response, and the request **was** fully written | **no — terminal** | **yes** |
| Context cancelled or deadline passed | no — terminal | per the write evidence above |

A 429 need not carry `parameters.retry_after`: the field is optional, and a proxy in front of
the self-hosted instance (`docs/DESIGN.md` §12.2's deployment) can emit a bare 429 of its
own. Telegram has still said "not now", so the attempt is retryable; without a stated wait the
transport falls back to D6's backoff rather than retrying immediately or inventing a duration
[derived → AC7, whose 429 case is driven both with and without the field].

The mechanism is `net/http/httptrace`: `WroteRequest` is called with a
`WroteRequestInfo{Err error}` after the transport has finished writing the request
[measured Go stdlib · `go doc net/http/httptrace.WroteRequestInfo` → "WroteRequestInfo
contains information provided to the WroteRequest hook" with field `Err error`], and it
fires on this path — the synctest probe over a real `http.Transport` recorded `wrote true`
[measured Go 1.26.5 · scratchpad probe `p4`, `go test -run TestSynctestHTTP` →
`status 429 body {"ok":false,…} wrote true elapsed 2s`]. The hook records a **sticky true**
on any `Err == nil` call, through an `atomic.Bool`, because the hook runs on the transport's
write goroutine while `Do` blocks and `go test -race` is a required gate for a change
touching shared state
[measured 31736b4:AGENTS.md:312 · `sed -n '312p' AGENTS.md` → "**`go test -race ./...` is a
required gate for any change touching goroutines, the scheduler, or shared state.** A race
is a defect, never a flake."].

**Why a sticky "any successful write" is the safe reading, verified against the transport's
own retry rules.** `http.Transport` can retry internally, but for a POST the only surviving
path is a *reused* connection that reported writing nothing
[measured Go 1.26.5 stdlib · `sed -n '/func (pc \*persistConn) shouldRetryRequest/,/^}/p' $(go env GOROOT)/src/net/http/transport.go` →
`if !pc.isReused() { … return false }` then `if _, ok := err.(nothingWrittenError); ok { return req.outgoingLength() == 0 || req.GetBody != nil }` then `if !req.isReplayable() { return false }`;
`sed -n '/func (r \*Request) isReplayable() bool/,/^}/p' $(go env GOROOT)/src/net/http/request.go` →
replayable only for `GET/HEAD/OPTIONS/TRACE` or when an `Idempotency-Key` / `X-Idempotency-Key`
header is present]. So the transport never re-sends bytes that already went out, and the
sticky flag cannot be raised by a retry of a write that produced nothing. **Corollary the
implementor must honour: the caller must never set an `Idempotency-Key` or
`X-Idempotency-Key` header** — doing so would make a POST replayable inside the transport
and put a second `sendMessage` in a player's chat behind our backs.

Any evidence outside this table classifies as **ambiguous and terminal**. The cost is
accepted explicitly (spec Key decisions): a message may be lost where a retry would have
delivered it, and that is preferred to a duplicate.

**A multipart request is never retried.** telego streams a multipart body
[measured telego@v1.11.2:telegoapi/request_constructor.go · `sed -n '/func (d DefaultConstructor) MultipartRequest/,/^}/p' …/telegoapi/request_constructor.go` →
`pr, pw := io.Pipe()` … `BodyStream: pr`], and a stream cannot be replayed without buffering
it — which telego's own helper warns leads to OOM
[measured telego@v1.11.2:telegoapi/caller.go:150-154 · `grep -n -B4 'BufferRequestData bool' …/telegoapi/caller.go` →
"// Warning: Enabling this may lead to excessive memory consumption and OOMKill" immediately
above the `BufferRequestData bool` field]. A
`BodyStream` request therefore makes exactly one attempt; the MVP sends no media, and the
rule is stated so a later media mechanic reopens it deliberately. A raw body (`BodyRaw`) is
a `[]byte`, so re-wrapping it per attempt is free.

### D6 — Backoff is equal jitter over a doubling scale, and AC5's monotonicity is a proof, not a hope

Attempt *i* (zero-based) waits `delay_i = d_i/2 + u·d_i/2` where `d_i = min(base·2^i, max)`
and `u ∈ [0,1)` comes from an injectable source.

- `delay_i ≥ d_i/2 > 0` whenever `base > 0`, which `New` validates — so **no code path
  re-attempts immediately** (AC5's second clause is structural).
- While `d_{i+1} = 2·d_i ≤ max`: `delay_{i+1} ≥ d_{i+1}/2 = d_i ≥ delay_i`. Growth is a
  property of the formula, not of the draw. At the ceiling the delays plateau by design, and
  the test configures `base`/`max` so the region under test is uncapped
  [derived → AC5's test, which injects a fixed source and asserts exact doubling].

Full jitter (`U(0, d_i)`) was rejected precisely because it can shrink a successive delay,
which would make AC5 assert something false about the code.

The source is `Options.Jitter func() float64`, defaulting to `math/rand/v2`'s `Float64`.

**That default needs a lint directive, and naming it here is what keeps `make lint` green on
the first run.** `gosec`'s G404 flags `math/rand` in non-test code, and this repo's `_test.go`
gosec exclusion does not reach a package-level default living in production source
[measured 31736b4:.golangci.yml + golangci-lint 2.13.1 · a scratch package returning
`rand.Float64()` from a non-test file, `golangci-lint run --config /home/syt/lab-game/.golangci.yml ./...` →
`G404: Use of weak random number generator (math/rand or math/rand/v2 instead of crypto/rand) (gosec)`].
The default therefore carries
`//nolint:gosec // G404: jitter is a backoff spread, not a security decision` — verified to
silence it while satisfying `nolintlint`'s require-specific and require-explanation settings
[measured 31736b4:.golangci.yml + golangci-lint 2.13.1 · the same scratch package with a second
function carrying that exact directive → the run reports the bare call only, `1 issues`, and
no `nolintlint` finding]. D13 pre-empts the analogous G101 for `tgtest`'s fake token; this is
the same courtesy for D6's own default. It
is not a determinism-rule surface — that rule is scoped to generation, combat and trail
replay [measured 31736b4:AGENTS.md:98 · `sed -n '98p' AGENTS.md` → "world generation,
combat, and any PvP-trail replay are pure functions of `(seed, input)`"] — but injecting it
is what makes AC5 an exact assertion instead of a statistical one.

### D7 — `retry_after` is honoured exactly, and the caller's deadline is the only thing that shortens the call

The value is read from the decoded envelope: `telegoapi.Error.Parameters.RetryAfter`, an
`int` of seconds
[measured telego@v1.11.2:telegoapi/api.go · `sed -n '/^type ResponseParameters struct/,/^}/p' …/telegoapi/api.go` →
`RetryAfter int \`json:"retry_after,omitempty"\``], decoded with `encoding/json` and confirmed
end to end in the probe (`retry_after: 7`). The spec flagged this field's wording as
unverified because the upstream page served truncated; reading the pinned dependency
discharges it, exactly as the spec's Technical constraints directed.

A 429 wait is **never shortened and never replaced by the backoff schedule**. It is bounded
only by the caller's context: if `now + retryAfter` is after the context deadline, the call
returns the typed error carrying the `retry_after` value instead of sleeping past it, and
`#43` owns the reschedule. A 429 pause costs **one attempt** and nothing else — the retry
budget is a count (D8), so there is no time budget for a wait to consume; the spec's
"does a `retry_after` wait charge the budget?" sub-question dissolves under that shape.

### D8 — One typed error, carrying what #43 must branch on

```
type Error struct {
    Method      string        // "sendMessage" — never the URL, never the token
    StatusCode  int           // last HTTP status; 0 when no response was received
    Description string        // Telegram's description, empty when none
    RetryAfter  time.Duration // from retry_after seconds; 0 when none
    Attempts    int
    Ambiguous   bool
    Err         error         // underlying cause, TOKEN-SANITISED — see below
}
```

#### The cause is sanitised at construction, and that is a mechanism, not a test obligation

**The bot token is in the request URL's path, and the standard library puts that URL into
every transport error verbatim.** `http.Client.Do` wraps whatever went wrong in a
`*url.Error`, and `url.Error.Error()` formats the URL with `%q`; the only redaction the
stdlib performs is on *userinfo*, so a path of `/bot<TOKEN>/sendMessage` survives intact
[measured Go 1.26.5 stdlib · `sed -n '/^func (e \*Error) Error() string/,/^}/p' $(go env GOROOT)/src/net/url/url.go` →
`return fmt.Sprintf("%s %q: %s", e.Op, e.URL, e.Err)`;
`sed -n '/func stripPassword/,/^}/p' $(go env GOROOT)/src/net/http/client.go` → it replaces
only `u.User.String()+"@"` when a password is set and otherwise returns `u.String()` whole;
`grep -n 'uerr := func' -A 12 …/client.go` → `urlStr = stripPassword(req.URL)` then
`return &url.Error{…}`]. Storing that error as `Err` and rendering it would print the token —
an incident under `AGENTS.md` § Permissions, where a leaked token is rotated through BotFather
rather than edited out of a log.

An earlier draft asserted the negative ("no error message carries the URL") and left it to
AC26's test. That is the wrong shape: **a test verifies a mechanism; it cannot be the
mechanism.** So the transport names one, in two layers:

- **The stored cause is never the `*url.Error`.** When `http.Client.Do` returns one, the
  transport stores its `Unwrap()` — the underlying `*net.OpError`, `context` error or
  transport error — which carries no URL. The error chain is unaffected for callers, because
  `url.Error.Unwrap` returns exactly that value
  [measured Go 1.26.5 stdlib · `sed -n '/^func (e \*Error) Unwrap() error/,/^}/p' $(go env GOROOT)/src/net/url/url.go` →
  `return e.Err`], so `errors.Is(err, context.DeadlineExceeded)` still reaches through for
  AC18 and `errors.As` still reaches `*Error` for AC8. Nothing needs the discarded
  `*url.Error` itself.
- **Every string the transport renders passes a token replacer**, built once from the
  configured token, as a second layer that does not depend on having predicted every path a
  token could take. telego's own logger is the in-repo precedent for exactly this
  [measured telego@v1.11.2:logger.go · `sed -n '/func defaultReplacer/,/^}/p' logger.go` →
  `strings.NewReplacer(token, DefaultLoggerTokenReplacement)`].

Sanitisation happens **at construction**, so a `*Error` value is safe for the rest of its
life and does not depend on the client that made it still existing. AC26's test then verifies
this mechanism rather than establishing the property
[derived → AC26, whose test drives a transport-level failure and asserts the token appears in
no rendered error].

`Error` implements `error` and `Unwrap`, so `errors.Is(err, context.DeadlineExceeded)` and
`errors.As(err, &tgErr)` both see through telego's own wrapping — telego wraps our return
with `%w` at each layer
[measured telego@v1.11.2:bot.go · `grep -n 'fmt.Errorf("internal execution\|fmt.Errorf("request call' bot.go` →
`fmt.Errorf("internal execution: %w", err)` and `fmt.Errorf("request call: %w", err)`;
`grep -n -A5 'func (b \*Bot) SendMessage' methods.go` → `fmt.Errorf("telego: sendMessage: %w", err)`].
Verified live: the probe's `SendMessage` returned
`telego: sendMessage: api: 429 "Too Many Requests", …, retry after: 7`, i.e. the chain
reaches the caller intact.

The caller returns **either a successful `*telegoapi.Response` or our `*Error`** — a
Telegram-level failure never leaks out as telego's own `api:` error. That gives #43 the
branches the spec's Key decisions demand: *retry later at T* is `RetryAfter > 0`,
*this chat is gone* is `StatusCode` plus `Description`, *unknown — do not re-send* is
`Ambiguous`.

**The retry budget is `RetryMaxAttempts`, a count** (owner, round 3). Its default is the
design's to choose and no source states one — see D10.

### D9 — One schedule per key owns every window; nothing is composed

**This section was rewritten after the product owner redirected the approach**, and has since
been corrected again against its own first review. Every defect it has carried shares one
recognisable shape — *a mechanism that reads correctly at the call site while failing the
property it is named for, under a configuration no acceptance criterion exercised* — so the
section is written to make each property checkable rather than argued.

**The composed-legs cause, named so the replacement can be checked against it.** Composed
legs each commit state at a *different* notion of "when", while the emission happens at the
latest instant any leg produced. The defects that triggered the owner's redirect all followed
from that one gap:

| Round | Defect | The gap it came through |
|---|---|---|
| 1 | `CancelAt` reclaims nothing once a later reservation exists | one leg's undo is not the inverse of its do |
| 2 | `burst = count` admits `b + r·W`, double the cap | one leg's parameter does not mean what the key is named for |
| 3a | a refusal that removes its instant shrinks the cap's memory | one leg mutates before the decision is final, then partially undoes |
| 3b | the global bound holds on reservations, not emissions | one leg commits at the candidate instant, another moves the grant later |

The replacement removed all four. Its own first review then found two more, of a *different*
shape — not composition, but a read that is **sufficient for safety** while reading as though
it were **optimal**, and an arithmetic rounding — and both are fixed below:

| Finding | Defect | Fix, and where |
|---|---|---|
| head-of-line | an ordered class-global key pushes a quiet chat behind a busy chat's backlog, and refuses it outright under a deadline | the class-global key is **unordered** — § *The unification* |
| pacing rounding | `W/N` truncates, so `N+1` emissions fit inside `W` | the pacing interval rounds **up**, and pairs with the quota window that states the figure directly — § *Configuration maps onto windows* |

So the replacement is built on one rule, and everything below is a consequence of it:

> **Decide the emission instant against every constraint first; commit to every constraint
> at that same instant; never commit before the answer is final and never partially undo.**

#### The unification: a pacing rate and a quota cap are the same kind of constraint

"At most one message per second" is `(count 1, per 1s)`. "At most twenty per minute" is
`(count 20, per 60s)`. **Both are window constraints, and a single mechanism can hold any set
of them.** That is what makes one mechanism able to own both windows, which is what Scope 6
asks for and what the composed design could only fake.

A **schedule** is the whole state for one key: its window set, plus the emission instants it
has already granted, held in ascending order.

**The invariant, stated once, in the form the tests assert:** for every window `(c, per)`, no
half-open interval of length `per` contains more than `c` grants. Equivalently, over the
sorted grants, `grant[j+c] - grant[j] ≥ per` for every `j`. The second form is a local check
an implementation can assert after every insertion, and § Test Design requires exactly that
— **the mechanism is correct by a checkable postcondition, not by an argument in this
document**, which is the property this design's review history says it most needed.

- **`earliest(candidate)` — a pure read, mutating nothing.** Returns the earliest instant
  `t ≥ candidate` at which one more grant still satisfies the invariant. Because the
  invariant can only change where a grant's window expires, the answer is either `candidate`
  itself or some `grant + per`; the implementation tests those instants in ascending order
  and returns the first that holds [derived → the `earliest` unit tests and their
  brute-force oracle in § Test Design].
- **`commit(t)`** inserts `t` in order.

**Two schedule kinds, because two different obligations exist — and the difference is one
flag, not a second mechanism.** The commit protocol below is identical for both; only the
answer to `earliest` differs.

| Kind | Used for | `earliest` | Why |
|---|---|---|---|
| **ordered** | one chat key `(chat, class)` | raises `candidate` to the key's newest grant first, so grants are non-decreasing and inserts are appends | AC30 requires per-key arrival order, and non-decreasing grants are what make it hold by construction |
| **unordered** | the class-global key | no clamp — the earliest admissible instant, inserted in order | a class ceiling has **no** ordering obligation, and imposing one is what caused cross-chat head-of-line blocking |

**Why the class-global key must NOT be ordered — a defect this design had and no longer
has.** An ordered schedule answers "after everything already granted", which is sound but is
not the earliest admissible instant. The class-global schedule records the future emissions
of *every* chat, so one backlogged chat would push the shared timeline minutes ahead, and a
second chat with an empty schedule of its own would be granted behind that backlog — delayed,
and with a caller deadline, **refused**. That is starvation, on a key whose spare capacity
(30/s against one chat's 20/min) was never the constraint. Making the class-global key
unordered removes it: the second chat is granted at the next free global interval regardless
of how far another chat's backlog extends [derived → AC11's cross-chat scenario **with the
global class bounded**, which § Test Design adds precisely because the old isolation
scenario turned the global class off and could not see this].

**The trade, stated so nobody re-derives it as a bug:** the class-global key no longer
emits in global arrival order. No acceptance criterion asks for one — AC30 is per key, and
per-key order is preserved by the ordered kind. What is given up is a property nothing
needed; what is bought is that a quiet chat is never held behind a busy one.

#### The decision: one lock, one instant, commit-after-decide

```
lock the limiter                      // ONE mutex covers every schedule
t := now
repeat until stable:                  // each schedule re-asked at the new t
        t = max(t, globalSchedule.earliest(t), chatSchedule.earliest(t))
if the caller has a deadline and t is after it:
        unlock and return the D8 error        // NOTHING has been mutated
globalSchedule.commit(t); chatSchedule.commit(t)   // same t, every schedule
unlock
wait until t, or until ctx is done
```

The loop is needed because `earliest` **does** depend on its argument once a schedule is
unordered: raising `t` for one schedule can move it into a region the other no longer admits.
It terminates because `t` strictly increases on every non-final pass and every schedule
admits all sufficiently large instants; in practice it settles immediately. An implementation
bounds the passes and treats exhaustion as a defect, not as a fallback
[derived → the acquire unit tests in § Test Design].

**Why round 3's defects are absent rather than fixed.**

- **A refusal cannot corrupt anything, because a refusal happens before any mutation.** There
  is no reclamation path, so there is nothing for a reclamation path to get wrong — the
  round-3a defect has no surface to exist on [derived → AC31's refusal scenario, specified
  in § Test Design to run red against a "commit-then-undo" variant first].
- **Every bound holds on the emission instant, because every schedule is committed at the
  instant the call actually goes out.** The round-3b defect required a leg to be charged at a
  candidate the other legs then moved; here there is one instant and it is the emission
  [derived → AC10's cross-chat scenario, specified to run red against a
  "commit-at-candidate" variant first].

**One mutex for the whole limiter, deliberately.** Per-key locks with a fixed acquisition
order would work, but they re-introduce multi-component state commitment — the structure that
produced every defect above. The critical section is a walk over a small window set and an
insertion; at MVP scale (one chat) and at fan-out scale (one bulk sender, `docs/DESIGN.md`
§11) that is not a contention surface worth the risk. Recorded as a decision, with contention
as the thing to measure if it is ever revisited.

#### Retention, and the one hole that remains

**Retention is by TIME, never by count: a schedule drops a grant once it can no longer
constrain any window — that is, once `grant + max(per)` is in the past.** A count bound would
be sound only for an ordered, append-only schedule, where the `c`-th newest is the oldest
grant any window can reach; on an unordered schedule, whose inserts are sorted rather than
appended, a count bound silently discards grants that are still inside a live window and the
schedule then over-emits. That is a real trap and it earns a row in subtask 4's broken-variant
table [derived → the retention unit tests and the global-ceiling occupancy assertion in
§ Test Design].

Retention stays bounded without a count, though not by the configuration alone: the invariant
caps how many *past* grants can live inside `max(per)`, while grants still ahead of now are
all retained, so the retained set is **O(calls in flight)** — bounded by concurrency. The
Registry subsection below states the same bound; it is stated once here and once there and
they agree.
**Eviction is a function of the window set and the clock alone — never of a refusal, a
cancellation, or any call outcome.** That divorce is what makes round-3a structurally
impossible rather than merely repaired.

**Grants are never removed on a call's behalf. A call cancelled *after* commit leaves its
instant behind.** That is a hole, and the design takes it deliberately rather than reaching
for the "exact reclamation" that produced round-3a:

- Removing a grant for a call outcome is the shape that has now failed twice; ageing out is
  the only removal, and it depends on the clock rather than on what any caller did.
- The hole is safe in the only direction that matters: a leftover grant can only push later
  grants **later**, so the limiter emits at or below its configured rate and never above it.
  Flood safety is preserved; throughput under saturation may dip.
- It is bounded: a hole ages out after `per` like any other grant.

This is the same trade round 2 recorded, kept honest and now applying uniformly to every
window rather than to some legs and not others [derived → AC31].

#### Configuration maps onto windows, and the key's role fixes the mapping

Both key roles produce window constraints on the same schedule; only the reading of
`count/per` differs, which is exactly what the key names already say:

| Key role | Reading of `N/W` | Windows contributed |
|---|---|---|
| `_GLOBAL`, `_CHAT_RATE` — **pacing** | steady emission at `N` per `W`, no burst | `(1, ceil(W/N))` **and** `(N, W)` |
| `_CHAT_CAP` — **quota** | at most `N` in any window of length `W` | `(N, W)` |

**Both halves of the pacing row are load-bearing, but they bear DIFFERENT properties, and an
earlier draft of this section attributed the wrong one to each.**

- **The quota window `(N, W)` bears the bound.** It states the figure the key is named for
  *directly*, and the schedule's invariant does the rest: if `N+1` grants lay inside any
  half-open interval of length `W`, some `grant[j+N] - grant[j]` would be `< W`, which the
  invariant forbids. So `N+1` per `W` is unreachable while this window is present — for any
  pacing interval, correctly rounded or not. This is D10's own standard applied to the
  mechanism: each configured row is a claim about emissions, not about a constructor
  argument.
- **The pacing window `(1, ceil(W/N))` bears the *shape*, not the bound.** Without it the
  quota window alone is satisfied by emitting all `N` at one instant and then idling for `W`
  — a burst, which is exactly what the owner's round-3 choice rejects. With it, emission is
  one per interval [derived → AC30's steady-emission assertion, specified in § Test Design to
  run red against a `quota-only` variant].
- **`ceil` rather than truncation buys evenness, and only evenness.** `W/N` in integer
  duration arithmetic rounds *down*, so `N` truncated intervals fall a few nanoseconds short
  of `W`. That shortfall cannot produce an `N+1`-th emission — the quota window forbids it —
  but it does make the quota window intervene, stretching one interval and putting a hiccup
  in an otherwise uniform stream. Rounding up removes the hiccup. **It is defence in depth,
  not the thing that stops over-emission**, and an earlier draft claimed the opposite; the
  correction matters because the repair that false claim invited was to delete the quota
  window so its red-first variant would finally fail. When `N` is 1 the two windows coincide
  and the schedule holds one.

The pacing reading is the owner's round-3 choice — a leaky bucket in the shaping sense,
steady emission, no burst — expressed with no burst parameter at all: "at most one per
interval" *is* steady emission. At the message defaults the chat schedule therefore holds
`{(1, 1s), (20, 60s)}` and the class-global schedule holds `{(1, ceil(1s/30)), (30, 1s)}`
[derived → AC12's shape and occupancy clauses and AC13's global-ceiling clause].

**An unbounded value contributes no window.** A class configured `off` has an empty window
set, so its schedule never blocks and never allocates history — which is why the registry
note below concerns bounded classes only [derived → AC13].

**Nothing is ever discarded.** The schedule only delays; a call that cannot be emitted before
its deadline returns the typed error. Silent drop is a defect, not a tuning choice (spec
Technical constraints) [derived → AC31].

**Per-key arrival order is preserved; global arrival order is not, and is not claimed.**
Within one **ordered** schedule the candidate is raised to that key's newest grant, so its
grants are non-decreasing and its emission order is its arrival order — and that same
non-decreasing property is what lets an ordered schedule read the `c`-th newest grant
directly instead of counting occupancy. Across keys there is no such guarantee and none is
needed: a later-arriving call into a quiet chat may be granted an earlier instant than one
already queued for a busy chat, which is the head-of-line fix working, not a violation. AC30
is a per-key criterion [derived → AC30, and the cross-chat scenario in § Test Design].

#### The argued wheel: what was evaluated, and why none of it fits

`AGENTS.md` § Dependency Versions requires this comparison for a hand-rolled mechanism, with
KD-4 as the model. **The requirement is unusual and specific: shape (answer *when*), never
drop; honour a context deadline and report refusal as our own typed error; and hold more than
one window on one key.** Every maintained candidate read for this decision fails the first
clause, the third, or both.

| Package | What it offers | Why it does not fit |
|---|---|---|
| `golang.org/x/time/rate` | token bucket, `Reserve`/`Wait`, context-aware | a bucket of size `b` refilled at `r` admits `b + r·W` in a window of length `W`, so no `burst` makes it mean "at most `c` per `per`"; and using it for one window while owning another is the composition this design forbids |
| `throttled/throttled/v2` | GCRA over a store | **policing**: `RateLimitCtx(...) (bool, RateLimitResult, error)` answers *allowed?*, not *when*; quota is `RateQuota{MaxRate, MaxBurst}`, the same burst parameter |
| `ulule/limiter/v3` | fixed-window counter over a store | **policing**: the whole `*Limiter` surface is `Get`/`Peek`/`Reset`/`Increment`, each returning a `Context{Limit, Remaining, Reset, Reached}` verdict |
| `go.uber.org/ratelimit` | a genuine shaping leaky bucket | `Take() time.Time` takes **no context** — a call parked behind it cannot be cancelled, contradicting Scope 10 and AC18 |
| `github.com/sethvargo/go-limiter` | store-backed token bucket | **policing**: `Take(ctx, key) (tokens, remaining, reset uint64, ok bool, err error)`, documented "If `ok` is false … the caller should NOT service the request" |

Measured, not recalled
[measured golang.org/x/time@v0.15.0:rate/rate.go · `sed -n '/^\/\/ A Limiter controls/,/^type Limiter struct/p' rate/rate.go` →
"It implements a \"token bucket\" of size b, initially full and refilled at rate r tokens per second";
throttled/throttled/v2@v2.15.0 · `sed -n '/type RateLimiterCtx interface/,/^}/p' …/rate.go` → that method with the doc comment "checks whether a particular key has exceeded a rate limit", and `sed -n '/^type RateQuota struct/,/^}/p' …/rate.go` → `MaxRate Rate` / `MaxBurst int`;
ulule/limiter/v3@v3.11.2 · `grep -rhn '^func ' …/limiter.go` → `Get`, `Peek`, `Reset`, `Increment`, and `sed -n '/^type Context struct/,/^}/p' …/limiter.go` → those fields;
go.uber.org/ratelimit@v0.3.1 · `sed -n '/^type Limiter interface/,/^}/p' ratelimit.go` → `Take() time.Time`, and `grep -rn 'context\.' *.go` outside tests → **no match in the package**;
github.com/sethvargo/go-limiter@v1.2.0 · `sed -n '/^type Store interface/,/^}/p' store.go` → that `Take` signature and that doc comment;
`grep -rln 'func .*) Wait(' <each module root>` → **no file in throttled, ulule or sethvargo**].

**The composition escape is the one this round closes off.** Keeping `x/time/rate` for the
pacing windows and owning only the quota window is exactly the composed-legs structure the
owner rejected after three rounds, and round-3b is the measurement showing why: the moment
one component commits at an instant another component can move, a bound stops applying to
emissions. A package that owns *part* of the decision cannot satisfy the one rule this design
is built on. **`golang.org/x/time` is therefore not taken as a dependency at all** — this task
adds no module beyond telego (D1).

*Escape hatch, in KD-4's spirit:* if a maintained package appears that shapes to a deadline
with a context and expresses more than one window on a key, the schedule is a single
unexported type behind an internal call and is replaceable without touching the caller.

#### Registry, lifetime and persistence

**The registry holds one schedule per `(chat, class)` of a bounded class, and this design does
not evict — recorded as a known bound.** The MVP ships to one friendly chat
(`docs/DESIGN.md` §14), and #43's fan-out is the trigger to revisit. A schedule's history is
bounded by its own invariant for the grants already in the past — at most `c` can live inside
any `per` — while grants still ahead of now extend with the in-flight backlog, so retention is
**O(calls in flight)**, bounded by concurrency rather than by the window set alone. At the
message defaults that is a handful of instants; an unbounded class allocates no history at
all. One correctness note for whoever adds eviction: dropping a key discards its grants
and so resets every window it was enforcing, which means eviction must be time-based — only a
key idle longer than its longest `per` may be dropped [derived → the § Open questions entry].

**Nothing here persists.** The state is in-process, lives with the calls in flight, and
touches no table, no disk and no migration [derived → AC32]. The owner was asked this round
whether durability should move into #19 and answered leave as designed: #43 keeps the durable
Postgres queue on `scheduled_tasks`, and this transport keeps in-process pacing.

### D10 — Configuration: one optional-with-default key class, in `internal/config`

**`internal/tg` imports `internal/config` and consumes `config.Transport` directly.** The
alternative — `tg` declaring its own option types and `cmd/bot` mapping — duplicates the
whole value-type set to save an import that points the natural way (a transport reads its
settings from the configuration package). `internal/config` stays a leaf with no project
imports; the edge is `tg → config`, and it is acyclic.

**Types** (plain data; `internal/config` imports no limiter package):

```
type Transport struct {
    RetryMaxAttempts int
    RetryBaseDelay   time.Duration
    RetryMaxDelay    time.Duration
    AttemptTimeout   time.Duration
    Limits           TransportLimits
}
type TransportLimits struct { Message, Edit, Other ClassLimits }
type ClassLimits    struct { Global, ChatRate, ChatCap Rate }
type Rate           struct { Count int; Per time.Duration }   // zero value == unbounded
```

Named fields rather than a `map[MethodClass]…`, because map iteration order is
non-deterministic and `AGENTS.md` § Code Style forbids depending on it
[measured 31736b4:AGENTS.md:98 · `sed -n '98p' AGENTS.md` → "**Determinism:** world
generation, combat, and any PvP-trail replay are pure functions of `(seed, input)`. No
`time.Now()`, no map-iteration order, and no un-seeded `math/rand` on those paths."].

**Grammar for a limit value:** `<count>/<duration>` (duration parsed by
`time.ParseDuration`), or the literal `off` for unbounded. Both parts must be positive. An
invalid value is a start-up `*KeyError` naming the variable, exactly like every existing key.

**Keys and defaults.**

| Key | Default | Source of the default |
|---|---|---|
| `LAB_GAME_TG_RETRY_MAX_ATTEMPTS` | `3` | **chosen, not sourced** — see below |
| `LAB_GAME_TG_RETRY_BASE_DELAY` | `500ms` | **chosen, not sourced** |
| `LAB_GAME_TG_RETRY_MAX_DELAY` | `30s` | **chosen, not sourced** |
| `LAB_GAME_TG_ATTEMPT_TIMEOUT` | `30s` | **chosen, not sourced** — see below |
| `LAB_GAME_TG_LIMIT_MESSAGE_GLOBAL` | `30/1s` | published, verified — below |
| `LAB_GAME_TG_LIMIT_MESSAGE_CHAT_RATE` | `1/1s` | published, verified — below |
| `LAB_GAME_TG_LIMIT_MESSAGE_CHAT_CAP` | `20/1m` | published, verified — below |
| `LAB_GAME_TG_LIMIT_EDIT_GLOBAL` | `off` | no published figure |
| `LAB_GAME_TG_LIMIT_EDIT_CHAT_RATE` | `off` | no published figure |
| `LAB_GAME_TG_LIMIT_EDIT_CHAT_CAP` | `off` | no published figure |
| `LAB_GAME_TG_LIMIT_OTHER_GLOBAL` | `off` | no published figure |
| `LAB_GAME_TG_LIMIT_OTHER_CHAT_RATE` | `off` | no published figure |
| `LAB_GAME_TG_LIMIT_OTHER_CHAT_CAP` | `off` | no published figure |

**The message-class defaults are verified against the official source, with their
modality**, which is what the spec's Technical constraints demanded before any figure
becomes a default
[measured core.telegram.org/bots/faq § "Broadcasting to Users", fetched 2026-09-04 · WebFetch →
"In a single chat, avoid sending more than one message per second.";
"In a group, bots are not be able to send more than 20 messages per minute.";
"For bulk notifications, bots are not able to broadcast more than about 30 messages per second, unless they enable paid broadcasts to increase the limit.";
"We may allow short bursts that go over this limit, but eventually you'll begin receiving 429 errors."].
The modalities differ exactly as the spec anticipated: the per-chat per-second figure is
**advisory** ("avoid sending more than"), the per-minute and global figures are **stated as
enforced** ("are not able to"). The global and per-minute figures also appear in this
repository already (`docs/DESIGN.md` §11: 30 msg/sec globally, ~20 msg/min into one chat);
the per-second one does not, which is why it reaches § Open questions.

**A verified figure is only half of AC13 — the mechanism has to deliver it, and earlier
drafts of this document twice shipped one that did not.** AC13's wording is "traceable to a
figure the design document cites **and has verified**", and a default the limiter does not
actually impose is not traceable to anything. **Each row of the table above is therefore a
claim about emissions, not about a constructor argument**, and D9's window model is what
makes the rows true by construction: a key's `N/W` becomes window constraints on one
schedule, so `_CHAT_CAP = 20/1m` means no window of sixty seconds ever carries more than
twenty message-class emissions into one chat — which is what `ai-docs/domain-invariants.md`
§ 6 states — and `_CHAT_RATE = 1/1s` means no one-second window carries more than one. The
AC12/AC13 scenarios assert occupancy per window for that reason: an instant-list assertion
cannot see this class of defect, which is how it survived two rounds (§ Test Design).

**The edit and other classes stay unbounded because the same page states no figure for
them** — asked directly, and the answer was explicit
[measured core.telegram.org/bots/faq, fetched 2026-09-04 · WebFetch, asked whether the page
states any numeric limit for editing messages, deleting messages or answering callback
queries → "This page does **not** state any numeric rate limits for editing messages,
deleting messages, or answering callback queries. The only numeric rate limits mentioned
concern **message sending**."]. Throttling on an unciteable number would delay gameplay
against a limit Telegram may not impose; a 429 is honoured exactly and is safe to retry, so
the reactive path already covers it. An operator bounds any class from the environment with
no code change [derived → AC13].

**The retry defaults are chosen, and the design says so rather than dressing them as
sourced.** An attempt cap of `3` means at most two waits, and under D6's formula with a
`500ms` base those fall in `[250ms, 500ms)` and `[500ms, 1s)` — so a give-up costs under a
second and a half of waiting: long enough to ride out a transient blip, short enough that a
player's action does not appear to hang. `30s` caps the scale so a higher configured attempt
count cannot grow the wait without bound. The bound is arithmetic over D6's formula and the
chosen values, not a measurement, and it is stated because it is the rationale — an earlier
draft asserted roughly double it by counting waits as attempts. Each is an operational
tuning value an operator may change, which is the entire point of the key class.

**`LAB_GAME_TG_ATTEMPT_TIMEOUT` is an addition beyond the spec's literal criteria, and the
owner has confirmed it explicitly — it is a settled decision, not an unasked addition a later
reader should reopen.** Nothing in the ACs requires it: every telego method takes a `ctx`, so a
caller *can* bound a call. But a caller that passes `context.Background()` against a wedged
self-hosted instance blocks for as long as the server holds the connection, and D5's
classifier only ever sees an outcome when one arrives — which is precisely the incident
`docs/DESIGN.md` §12.2 records as the motivation for running our own instance ("a specific
bot times out for hours with no 429 and no error"). The timeout bounds one attempt, leaving
the caller's context as the bound on the whole call. Put to the owner and **kept**: the
§ Open questions entry records the answer rather than the question.

**What this class does to the configuration layer, checked claim by claim.**

- **The disjointness test survives, provided every key is queried unconditionally.**
  `recordingLookup` records a key at query time, before delegating
  [measured 31736b4:internal/config/disjoint_test.go · `sed -n '/^func recordingLookup/,/^}/p' internal/config/disjoint_test.go` →
  the wrapper appends `key` to `keys` and only then `return base(key)`], so an absent
  optional key is still in the recorded set. The loader therefore calls `lookup` for **every**
  transport key on every load, with no early return and no branch that skips one.
- **"Queried unconditionally" must NOT be implemented by adding the transport keys to
  `envKeys()`, and this is the trap in the change.** Live tests iterate that unexported
  helper and assert a *required*-variable failure for every member: removing a member's value
  must yield `ErrMissing`, and emptying it must yield `ErrInvalidValue`
  [measured 31736b4:internal/config/env_test.go:53,66 · `grep -n 'func TestLoadEnv_RequiredVariableUnset\|func TestLoadEnv_RequiredVariableEmpty' internal/config/env_test.go`
  → both at those lines, each body `for _, key := range envKeys()` then `assertKeyError(t, err, ErrMissing, key)` /
  `assertKeyError(t, err, ErrInvalidValue, key)`]. An optional key added to `envKeys()` fails
  both immediately, and the tempting repair — loosening the assertion — is a direct attack on
  AC24, which exists to keep those variables required.
  **The correct shape is the one the package already uses for the balance-file and
  world-set paths:** a
  dedicated reader. `envBalancePath` and `envWorldPath` are validated by `requiredBalancePath`
  and `resolveWorldPath` rather than by `loadEnv`, and appear in `EnvKeys()` only
  [measured 31736b4:internal/config/env.go · `sed -n '/^\/\/ envKeys returns/,/^}/p' internal/config/env.go`
  → the doc comment "envKeys returns the four variables loadEnv itself validates. envBalancePath
  and envWorldPath are validated by their own dedicated readers instead"; and
  `sed -n '/^func EnvKeys/,/^}/p' internal/config/env.go` → `return append(envKeys(), envBalancePath, envWorldPath)`].
  So: `loadTransport(lookup)` owns the transport keys, `Load` joins its errors alongside the
  others, `EnvKeys()` appends `transportEnvKeys()`, and **`envKeys()` keeps exactly the
  membership it has today** — leaving `env_test.go`'s suites untouched and AC24 intact
  [derived → AC24, whose existing tests must still pass unmodified].
- **`.env.example` documents each new key carrying its default as the value**, because a
  test asserts every value is non-empty
  [measured 31736b4:internal/config/disjoint_test.go · `sed -n '/^\/\/ TestEnvExample_ValuesAreNonEmpty/,/^}/p' internal/config/disjoint_test.go` →
  `if strings.TrimSpace(v) == "" { t.Errorf(".env.example: %s has an empty value", k) }`].
  `off` and `30/1s` are non-empty, and `TestLoad_ExampleEnvironmentSucceeds` requires them to
  parse.
- **`KD-24` is untouched.** Its requiredness consequence is scoped to file paths
  [measured 31736b4:ai-docs/key-decisions.md · `grep -n 'KD-24' ai-docs/key-decisions.md` →
  "each file path is a **required** environment variable with no compiled-in default"]. A rate
  limit is neither a file path nor a balance number.
- **`internal/config/balance.go`'s and `doc.go`'s no-fallback clauses are untouched** —
  both are scoped to balance values
  [measured 31736b4:internal/config/balance.go:9-14 · `sed -n '9,14p' internal/config/balance.go` →
  "No field has a compiled-in fallback — every value is read from the balance file named by
  LAB_GAME_BALANCE_PATH"; 31736b4:internal/config/doc.go:9-11 · `sed -n '9,11p' internal/config/doc.go` →
  "the balance YAML file (LAB_GAME_BALANCE_PATH) supplies every game constant … with no
  compiled-in fallback for any of them"].
- **The live sentences this change falsifies are rewritten in the same task — that is AC25
  and AC29's first named site.** `internal/config/env.go`'s
  header
  [measured 31736b4:internal/config/env.go:11-13 · `sed -n '11,13p' internal/config/env.go` →
  "Every variable is required; none has a compiled-in default (design D10)."] and `Config`'s
  doc comment
  [measured 31736b4:internal/config/config.go:37-39 · `sed -n '37,39p' internal/config/config.go` →
  "Every field is populated by Load or Load returns an error naming every rejected key — no
  field has a compiled-in / fallback."]. Both become statements about the variables they were
  written for, with the optional-with-default tuning class named alongside.

  **The sweep that establishes there is no third site had to be multiline-aware**, and a
  single-line `grep` under-reports here for a concrete reason: `Config`'s phrase is broken
  across a comment line break (`compiled-in` / `// fallback`), so a line-oriented pattern
  misses the very file that carries the claim. Run as
  [measured 31736b4 · `rg -U -n -i --glob '*.md' --glob '*.go' --glob '!ai-docs/plans/done/**' --glob '!ai-docs/plans/2026-09-04-bot-api-transport*' --glob '!ai-docs/learnings.md' --glob '!ai-docs/harness-gaps.md' 'no telegram client|none has a compiled-in default|every variable is required|compiled-in\s+(//\s*)?fallback' .` →
  `ai-docs/context.md:43`, `ai-docs/context-status.md:79`, `ai-docs/key-decisions.md:61`,
  `internal/config/balance.go:12`, `internal/config/config.go:38`, `internal/config/config.go:39`,
  `internal/config/doc.go:11`, `internal/config/env.go:12`], of which only `env.go` and
  `config.go` assert the falsified claim — `balance.go`'s and `doc.go`'s no-fallback clauses,
  KD-22's, and `context-status.md`'s are all scoped to **balance values**, and `context.md`'s
  is the separate "no Telegram client" claim that `/task` Step 9.5 owns.

  **One site the sweep could not have found, and the reason is worth keeping.** `doc.go`'s
  package comment *enumerates* what the process environment supplies. Nothing in it is
  falsified — its no-fallback clause is scoped to balance values — but the enumeration becomes
  **incomplete** once the transport keys land, and an incomplete list matches no
  falsified-claim phrase, so a regex over claim wording is structurally unable to see it. It
  is in AC29's set by `AGENTS.md` § Propagation Rule step 4 all the same, it is named in
  subtask 1, and it is recorded here because "the sweep came back clean" is a statement about
  the sweep's pattern before it is a statement about the tree (`AGENTS.md` § Patterns 2).

The relaxation reaches **only** these operational tuning keys. Secrets, the base URL, the
chat allowlist and the balance-file and world-set paths stay required with no default
[derived → AC24's test: removing any one of the previously declared variables still fails,
naming it].

### D11 — One observation per call, no registry import

```
type Observation struct {
    Method      string
    Latency     time.Duration
    StatusCode  int   // 0 when no HTTP response was received
    RateLimited bool  // any attempt of this call received 429
    Retries     int
}
type Observer interface{ ObserveCall(Observation) }
```

`Options.Observer` is optional; nil means no observation and the package compiles and tests
green without one [derived → AC17]. The point fires **exactly once per outbound call** —
including a gate refusal, so #23 sees refusals too — after the last attempt returns.

`RateLimited` is defined as *any attempt of this call saw a 429*, not *the final response
was one*, because §13.2's 429 counter is the consumer and a 429-then-success call is exactly
the event it wants counted. `Latency` is the whole call as the caller experienced it,
limiter and backoff waits included; `Retries` and `RateLimited` let #23 separate the
components. Whether #23 also wants the final round-trip split out is § Open questions, not a
field added on speculation.

`internal/tg` imports no metrics registry [derived → AC17's import-scan test]; neither
`github.com/prometheus/client_golang` nor any other registry is reachable from this module
today [measured 31736b4:go.mod · `go mod why -m github.com/prometheus/client_golang` →
"main module does not need module github.com/prometheus/client_golang"].

### D12 — The outbound gate seam #22 installs into

```
type ChatTarget int   // what the transport could determine about the destination
const (
    ChatNone    ChatTarget = iota // the method addresses no chat at all
    ChatUnknown                   // it addresses a chat the transport could not read
    ChatKnown                     // Key holds the chat id
)
type ChatRef struct { Key string; Target ChatTarget }
type Call    struct { Method string; Class MethodClass; Chat ChatRef }
type Gate    interface{ AllowCall(ctx context.Context, call Call) error }
```

**Three states, not a boolean, because the two "no chat id" branches of D4 need opposite
treatment from #22 and a `bool` collapses them.** This is the one seam whose failure mode is
*messaging the wrong chat*, so the discriminator is explicit rather than implied by whether
`Key` happens to be empty:

| `Target` | When | What #22's allowlist gate must do |
|---|---|---|
| `ChatNone` | `getMe`, `getUpdates`, an inline-message edit — the method addresses no chat | **Allow.** There is no destination to check, and refusing here would break the update loop. |
| `ChatUnknown` | no decodable body (D4: `BodyRaw == nil`), so a destination exists but the transport could not read it | **Refuse.** An unverifiable destination is precisely what `ALLOWED_CHAT_IDS` exists to stop, and the MVP sends nothing that reaches this state. |
| `ChatKnown` | `Key` holds the chat id | Check `Key` against the allowlist. |

Under the previous boolean, one reading made #22 refuse `getUpdates` and the other let a
`sendPhoto` reach an unchecked chat silently. `ChatTarget` is also what the limiter switches
on (D9: no per-chat schedule, the reserved unknown-chat key, or the chat's own key), and
`exhaustive` requires any switch over it to be total or carry a `default`
[derived → AC27's gate scenarios, which cover each `Target` value].

`Options.Gate` is optional and is consulted **before any attempt and before any limiter
wait**, so a refused call costs no allowance. A refusal returns the D8 error with
`Attempts 0`, `StatusCode 0`, `Ambiguous false`, wrapping the gate's own error.

This task installs **no** allowlist; #22 owns `ALLOWED_CHAT_IDS` and its scope claims it.
The obligation here is negative and structural: the seam exists, it is reachable from
outside the package, and it cannot be routed around
[derived → AC27's test: a gate that refuses everything blocks a call issued through
`Client.API()`, and the package exposes no way to obtain a bot without the caller].

### D13 — `internal/tgtest`: a shared, in-process fake Bot API server

**A package, not a per-consumer helper.** The consumers already named are this package's
tests, #43's ordering / rate-limit / 429 tests, and #44's eval harness — at or above the
threshold at which `design-writer` § Rules requires a shared package under `internal/`
rather than copy-paste, and with an open-ended trajectory besides. The repository already
carries a non-test helper package of exactly this kind
[measured 31736b4:internal/testdb/testdb.go:1-7 · `sed -n '1,7p' internal/testdb/testdb.go` →
"Package testdb provisions a PostgreSQL instance for package tests that exercise real
database behaviour"], so the shape is precedent rather than invention; the name follows the
stdlib's `httptest` rather than `testdb`'s ordering, since `tgtest` reads as "the test
double for `tg`".

**No listener, no socket.** The server answers over `net.Pipe` through the *real*
`http.Transport`, wired by a `DialContext` that returns the in-memory connection. This is
what makes the whole test suite runnable inside a `testing/synctest` bubble (D14), and it
also makes AC19's "no network egress" structural rather than a promise: the base URL is
`http://bot-api.invalid`, under the reserved TLD, so a stray real dial can only fail
[derived → AC19, AC20].

Behaviours it must be able to produce on demand [derived → AC19]: a success; a 429 carrying
`retry_after`; a 5xx; a **transport-level failure with no request written** (the dial itself
fails); a **transport-level failure after the request was written** (the connection closes
without a response — the ambiguous case AC6 needs); and a delayed response.

**`tgtest.go` carries a package comment**, as `internal/testdb` does: it is non-test Go, and
`revive`'s `package-comments` rule is enabled here, so a missing one is a lint round rather
than a style preference
[measured 31736b4:.golangci.yml · `grep -n -A3 'revive:' .golangci.yml` → `rules:` /
`- name: exported` / `- name: package-comments`].

It exports a syntactically valid fake token, because `telego.NewBot` rejects anything whose
shape the package's token regexp refuses — a digit run, a colon, then a fixed-width run of
word characters and hyphens
[measured telego@v1.11.2:bot.go · `grep -n 'tokenRegexp\|func validateToken' -A3 bot.go` → the
`tokenRegexp` constant anchored `^\d+:[\w-]{35}$` and `validateToken` matching against it;
the probe's first run, whose suffix was one character short, returned
`NewBot err: telego: invalid token format`, and lengthening the suffix alone made the same
probe reach the fake server].
`tgtest` is non-test Go, so `gosec` applies to it: the constant needs
`//nolint:gosec // G101: …` with a stated reason, matching the precedent already in the tree
[measured 31736b4:internal/config/env.go:15 · `sed -n '15p' internal/config/env.go` →
`envBotToken = "LAB_GAME_BOT_TOKEN" //nolint:gosec // G101: this is an environment-variable NAME, not a credential value`],
and `nolintlint` enforces both the specific linter and the explanation
[measured 31736b4:.golangci.yml · `grep -n -A2 'nolintlint:' .golangci.yml` →
`require-explanation: true` / `require-specific: true`].

`internal/tgtest` imports neither `internal/tg` nor telego — `net`, `net/http` and
`encoding/json` are enough — so there is no cycle and no test-only dependency reaches
`cmd/bot`, mirroring the consequence KD-20 already states for `internal/testdb`
[measured 31736b4:ai-docs/key-decisions.md:53 · `grep -n 'KD-20 —' ai-docs/key-decisions.md` →
"*Consequence:* `internal/testdb` is non-test Go imported only from `_test.go` files, so the
container runtime never links into `cmd/bot`"].

### D14 — Tests use `testing/synctest`, not an injected clock

The spec asks for "an injectable clock **or an equivalent**". The stdlib now ships the
equivalent, and it is strictly better here: inside a `synctest` bubble the whole `time`
package is a fake clock
[measured Go 1.26.5 stdlib · `go doc testing/synctest` → "Within a bubble, the time package
uses a fake clock. Each bubble has its own clock. The initial time is midnight UTC
2000-01-01." and "Time in a bubble only advances when every goroutine in the bubble is
durably blocked."]. Production code therefore uses `time.Now` and `time.NewTimer`
**directly** — the schedule of D9 reads the clock and the caller sleeps on a timer, with no
clock interface threaded through the transport — and the tests still get exact,
instantaneous virtual time. That is fewer moving parts in production *and* stronger
assertions. It also matters more under D9's redesign than it did before: the schedule's whole
contract is a statement about instants, so a test that cannot pin instants exactly cannot
check it [derived → the limiter scenarios in § Test Design].

Demonstrated on **the exact stack D13 specifies**, not a nearby one: a real `*http.Transport`
whose `DialContext` returns a `net.Pipe` connection with `DisableKeepAlives` set — no
listener, no socket — serving a handler that sleeps two virtual seconds, with `httptrace`
firing and `retry_after` decoded, in `0.00s` of wall time and clean under `-race`
[measured Go 1.26.5 stdlib · scratchpad probe `p4`,
`grep -n 'DialContext\|net.Pipe\|DisableKeepAlives' probe_test.go` → the transport is
constructed with `DisableKeepAlives: true` and a `DialContext` returning `net.Pipe()`;
`go test -race -v -run TestSynctestHTTP` →
`status 429 body {"ok":false,…} wrote true elapsed 2s` / `--- PASS: TestSynctestHTTP (0.00s)`].
That matters because the whole strategy turns on a pipe conn being durably blockable inside a
bubble while a real listener is not; a probe over a real socket would not have shown it.

**The bubble rules the test code must obey, each measured:**

- **`t.Run`, `t.Parallel` and `t.Deadline` must not be called inside the bubble**
  [measured Go 1.26.5 stdlib · `go doc testing/synctest.Test` → "T.Run, T.Parallel, and
  T.Deadline must not be called."]. So a table-driven test puts `synctest.Test` **inside**
  each subtest, and `t.Parallel()` on the subtest — outside the bubble — which works
  [measured Go 1.26.5 · scratchpad probe `p4`, a test calling `t.Parallel()` then
  `synctest.Test` → `--- PASS: TestParallelOutside (0.00s)`]. The repo's table-driven +
  `t.Parallel` convention survives intact.
- **A goroutine left blocked when the bubble's root returns is a hard failure**
  [measured Go 1.26.5 · scratchpad probe `p4`, a bubble leaking one sleeping goroutine →
  `panic: deadlock: main bubble goroutine has exited but blocked goroutines remain`]. So
  `tgtest` sets `DisableKeepAlives`, closes its side of every pipe, and registers its cleanup
  through `t.Cleanup`, which the bubble runs before returning.
- **Time only advances when every bubble goroutine is durably blocked**, which is also the
  lever that makes arrival order deterministic: releasing N concurrent callers one at a time
  with `synctest.Wait()` between them fixes the order in which they reach the limiter
  [measured Go 1.26.5 stdlib · `go doc testing/synctest.Wait` → "Wait blocks until every
  goroutine within the current bubble, other than the current goroutine, is durably
  blocked."]. AC30's "in the order they arrived" is assertable because of it.

### D15 — `cmd/bot` is not wired in this task

The composition root gains nothing: there is no update loop to feed the client (#22 owns it)
and no queue to send through it (#43). `cmd/bot` keeps loading and validating configuration
and nothing else, and its existing test's environment map keeps passing unchanged because
every new key is optional
[measured 31736b4:cmd/bot/main_test.go · `sed -n '/^func validEnv/,/^}/p' cmd/bot/main_test.go` →
a map of exactly the previously declared variables]. Constructing a client that nothing uses
would be dead code in the binary and an untested wiring path at once.

---

## Decomposition

| # | Task | Files | Depends on |
|---|------|-------|------------|
| 1 | `internal/config`: the optional-with-default transport key class — the value types of D10, the key names, the compiled-in defaults, the `<count>/<duration>`\|`off` grammar and its validation, a dedicated `loadTransport` reader joined into `Load` with `EnvKeys()` extended and **`envKeys()` left exactly as it is** (D10), the falsified doc comments in `env.go` and `config.go` rewritten (AC25), **`doc.go`'s package comment extended — its no-fallback clause stays untouched and correct, but its *enumeration* of what the environment supplies becomes incomplete once the transport keys land, so it is in AC29's set and is decided here rather than left to the implementor**, **and `.env.example` carrying each new key with its default as a non-empty value** in the file's existing commented style. Tests first: absent → default, present → parsed, malformed → `*KeyError` naming the key, the three-way key-set equality, and the previously declared variables still required. | `internal/config/transport.go`, `internal/config/transport_test.go`, `internal/config/env.go`, `internal/config/config.go`, `internal/config/doc.go`, `.env.example` | — |
| 2 | `internal/tgtest`: the in-process fake Bot API server of D13 — `net.Pipe` dialer, the `.invalid` base URL, the scripted behaviours AC19 lists, the fake token constant, and its own tests. | `internal/tgtest/tgtest.go`, `internal/tgtest/tgtest_test.go` | — |
| 3 | `internal/tg` foundations **and the telego dependency**: package comment, `Error`, `Observation`/`Observer`, `MethodClass` + the D4 classifier, `Gate`/`Call`/`ChatRef`, `Options` + `New` + `API`. `go get github.com/mymmrac/telego@<pinned>` runs in this subtask, with the importing file, so `make tidy-check` stays green (D1). Tests: the classifier over the pinned version's method names, option validation, error rendering and unwrapping. | `go.mod`, `go.sum`, `internal/tg/doc.go`, `internal/tg/errors.go`, `internal/tg/observe.go`, `internal/tg/class.go`, `internal/tg/client.go`, `internal/tg/class_test.go`, `internal/tg/client_test.go` | 1 |
| 4 | `internal/tg` limiter — **one schedule type owning every window, no composed legs and no new module** (D9): the window set; the invariant `grant[j+c] - grant[j] ≥ per` asserted after every insertion; `earliest(candidate)` as a pure read over the window-expiry instants; `commit` as a sorted insert; **time-based retention** (never count-based); the **ordered** kind for a chat key and the **unordered** kind for the class-global key; the class-global and per-`(chat, class)` registry under **one mutex**; the decide-then-commit acquire (iterate `t` to a fixed point, deadline refusal **before** any mutation, commit to every schedule at that same `t`, bounded passes with exhaustion treated as a defect); the pacing-vs-quota mapping with `ceil` rounding. **Plus the minimal caller its end-to-end scenarios require** — the `encoding/json` request constructor, and a `Caller.Call` that derives the method name and `ChatRef` (D4), calls the gate, acquires from the limiter, waits, performs **exactly one** attempt and decodes the envelope. **No retry loop, no backoff, no `retry_after`, no observation — those are subtask 5, which extends the same file.** `client.go` is edited here too: `New` wires `telego.WithAPICaller` and `telego.WithRequestConstructor` to this subtask's implementations and constructs the limiter registry from `config.Transport`, which is what makes an end-to-end scenario through `Client.API()` reach the limiter at all. Tests, each written **red-first against the named broken variant** (§ Test Design): per-class global admission, cross-chat non-blocking **with the global class bounded**, per-chat isolation, the undecodable-body branch, unbounded-class pass-through and its bound counterpart, private chats charged, steady ordered emission, burst-then-cap shape, **occupancy over every window of each configured `per`, on the chat schedules and on the class-global schedule at its shipped default**, **the global bound holding on emissions when a chat window pushes**, **refusals leaving the schedule unchanged**, saturation ending in emission or error, and identical behaviour under two different base URLs. | `internal/tg/limit.go`, `internal/tg/constructor.go`, `internal/tg/caller.go`, `internal/tg/client.go`, `internal/tg/limit_test.go`, `internal/tg/schedule_test.go` | 2, 3 |
| 5 | `internal/tg` caller — **extends subtask 4's `Call`, it does not create it**: the attempt loop with the limiter acquire moved inside it (D2's per-attempt charging), the `httptrace` write-evidence classifier, equal-jitter backoff **with its default jitter source carrying the `//nolint:gosec // G404: …` directive D6 names**, exact `retry_after` honouring with the deadline bound and the bare-429 fallback, the D8 typed error with its token sanitisation, and the single observation. Tests: no-shortened `retry_after`, strictly positive and growing delays, the ambiguous case making exactly one attempt, each retryable case (including a 429 with and without `retry_after`), give-up field by field, deadline refusal, cancellation at every waiting site, the attempt cap, the token absent from a rendered transport error, and the observation for a success, a retried success and a give-up. | `internal/tg/caller.go`, `internal/tg/retry.go`, `internal/tg/caller_test.go`, `internal/tg/retry_test.go` | 4 |
| 6 | `internal/tg` package-level guard tests: the import scan over `cmd/` and `internal/` non-test files (no fasthttp, no go-json, no metrics registry), the token-absence sweep (**whose known surface includes telego's own `Token()` and `FileDownloadURL` — D4's accepted in-module exposure, so a hit there is not a leak**), **the literal scan discharging AC21's second clause (no retry or rate-limit literal at a call site in `internal/tg` — every such value arrives from `config.Transport`)**, the seam tests (a refusing gate blocks a call through the accessor, **and no non-test file outside `internal/tg` names `telego.NewBot` or a `telego.With*` option** — D2), the base-URL-appears-only-in-the-constructor source check, and the end-to-end call against `tgtest` built from a `config.Load`-produced `BotAPIBaseURL`. | `internal/tg/guards_test.go` | 5 |
| 7 | `ai-docs/key-decisions.md`: rewrite KD-2 for the shipped reality (pinned version, the caller/constructor swap, the `stdjson` residue and why it was not taken, the toolchain ceiling), and add the decisions this task settles — **the project-owned window schedule: one mechanism holding every window on a key, why no maintained package fits (the rejected-alternatives table of D9), and the two properties it deliberately does *not* provide (grant reclamation, and per-key locking)**, `testing/synctest` in place of a clock abstraction, and the optional-with-default key class with its boundary. | `ai-docs/key-decisions.md` | 6 |

**Why `.env.example` is inside subtask 1 rather than following it.** Each subtask is
committed on a green gate, and `code-writer` Mode A commits per subtask with the **full**
test gate run first
[measured 31736b4:.claude/agents/code-writer.md · `sed -n '/^## Mode A/,/^## Mode B/p' .claude/agents/code-writer.md` →
"**First rule of Mode A: you COMMIT after each subtask.**" and "Run the gates: `go build ./...`;
`go test ./...` (scoped with `-run <TestName>` while iterating, full before the commit) …"].
`internal/config` already asserts set equality between `.env.example`'s keys and
`config.EnvKeys()`
[measured 31736b4:internal/config/disjoint_test.go:103 · `sed -n '103p' internal/config/disjoint_test.go` →
`assertSameKeySet(t, ".env.example", exampleKeys, "config.EnvKeys()", EnvKeys())`], so a
subtask that extends `EnvKeys()` while deferring `.env.example` leaves `go test ./...` red
and cannot be committed at all. An earlier draft split them and depended on the split
holding; it could not. The files move together, and **the fix is never to relax the
disjointness assertion** — it is the mechanism AC23 is written against.

**AC29's propagation set, and who owns each site.** Membership is decided by `AGENTS.md`
§ Propagation Rule step 4 and is not bounded by the spec's illustrative list; what this
design fixes is the ownership, so nothing falls between the design and the workflow.
`internal/config/env.go`, `internal/config/config.go`, `internal/config/doc.go` and
`.env.example` are subtask 1's (AC25); `ai-docs/key-decisions.md` KD-2 is subtask 7's.
`ai-docs/context.md` (§ Architecture "Layout so far" and § Status, whose "no Telegram
client" claim this change falsifies) and `ai-docs/context-status.md` are **not** subtasks
here: `/task` Step 9.5 owns those writes, and `ai-docs/plans/INDEX.md` is Step 12's
[measured 31736b4:.claude/skills/task/reference.md:280,284 · `sed -n '280p;284p' .claude/skills/task/reference.md` →
"| Step 9.5 | context-status.md entry appended + context.md summary/README.md updated? …" and
"| Step 12 | Branch ≠ main? INDEX.md ✅? spec/design `git mv`d to done/? …"]. What those
entries must say is fixed here so the
step does not re-derive it: the layout gains `internal/tg` (the Bot API transport — retries,
`retry_after`, both limiters, the observation point and the outbound gate seam) and
`internal/tgtest` (the in-process fake Bot API server, imported only from `_test.go` files);
the § Status "no Telegram client" clause becomes "a transport client with no update loop
yet — #22 owns the loop".

---

## Handoff plan

`M = 7`, so grouping applies — the section is required for every `M ≥ 1`, single-subtask
designs included. Two groups, homogeneous by change-type, minimised, each marked with its
implementor model and effort. The group size cap of `10` is a **maximum**, not a target: a
group ends at whichever comes first — the cap, a change-type switch, or a dependency-forced
boundary. Here the change-type switch is what ends Group A, well below the cap.

- **Handoff into Group A:** spawn `/context-reset` per
  `.claude/skills/context-reset/SKILL.md` § Compaction recovery (re-entry). The handoff is
  bound at the start of **every** design-defined group, including the first.
- **Group A** — model `sonnet`, effort `medium` (pinned) via the `code-writer` subagent,
  1M-token window — subtasks 1–6 (code change-type: `*.go`, `go.mod`/`go.sum`, `.env.example`).
  All same-change-type subtasks are clustered into ONE group rather than interleaved with
  subtask 7; within the size cap of 10.
  `.env.example`, `go.mod` and `go.sum` are code-group artefacts by this repository's own
  classification — CI's `go` paths filter lists them alongside `**/*.go`
  [measured 31736b4:.github/workflows/ci.yml · `sed -n '/            go:/,/            harness:/p' .github/workflows/ci.yml` →
  `- '**/*.go'`, `- '**/*.sql'`, `- 'go.mod'`, `- 'go.sum'`, `- '.golangci.yml'`, `- 'Makefile'`,
  `- '.github/workflows/**'`, `- 'config/**'`, `- '.env.example'`].
- **Spawn-contract note for Group A, binding on whoever writes the prompt:** subtask 4's
  **red-first broken-variant table must be passed verbatim, not summarised**. Group A carries
  the schedule — the most defect-prone code in this task, and the place where four historical
  defects were each invisible to an assertion that reproduced perfectly — into a
  `sonnet`/`medium` implementor, which is the routing `design-writer.md` § Rules (g) mandates
  for a code group and is not a defect. The variant table is what stands between that
  implementor and those defects: each row names a wrong mechanism *and* the assertion it must
  break, so a row that stays green is itself a finding. A summary loses the pairing, which is
  the only part that works.

- **Handoff after Group A:** spawn `/context-reset` per
  `.claude/skills/context-reset/SKILL.md` § Compaction recovery (re-entry). Parent `/task`
  resumes in Group B with fresh context.
- **Group B** — model `inherit` (the orchestrator's), effort inherited from the orchestrator
  (typically xHigh) — NOT pinned — via `general-purpose` with no inline `model=`, 1M-token
  window — subtask 7 (instructions/harness change-type: `ai-docs/**`). Terminal group
  (1 subtask; within the `1..=10` range).

Group count is 2, within the default maximum of 4, so no user gate is needed. Group A is
pure `.go` + build artefacts and Group B is pure prose, so neither delegate is handed work
outside its charter — `code-writer` must STOP on a predominantly-prose assignment
[measured 31736b4:.claude/agents/code-writer.md · `grep -n 'predominantly-prose' .claude/agents/code-writer.md` →
"**STOP if handed a predominantly-prose assignment.** Your charter is *code*."].

---

## Risks

- **The pinned telego version drifts out from under the toolchain.** `v1.12.x` is
  unreachable today and `v1.11.2` is the newest that resolves — but the toolchain may have
  moved by implementation time. Mitigation: subtask 3 runs `go get` and reads its output
  rather than trusting this document; if a newer release resolves, take it and record the
  version actually pinned. The failure mode is loud, not silent —
  `[measured telego@v1.12.1 · go get … → "requires go >= 1.26.7 (running go 1.26.5; GOTOOLCHAIN=local)"]`.
- **A standalone "add the dependency" step would fail `make tidy-check`.** `go mod tidy`
  removes an unimported requirement. Mitigation: the decomposition binds `go get` to the
  first importing file (subtask 3) —
  `[measured 31736b4 + telego@v1.11.2 · scratch copy, go get with no importer then go mod tidy → grep -c 'mymmrac/telego' go.mod = 0]`.
- **The transitive `testify` bump breaks an existing suite.** Mitigation: already exercised —
  every test binary in the tree compiles against `v1.12.1`, and the running of them is
  subtask 3's gate —
  `[measured 31736b4 + telego@v1.11.2 · scratch copy, go test -run XXXNONEXISTENT ./... → TESTCOMPILE-OK]`.
- **A leaked goroutine turns a passing test into a bubble panic.** Any connection or server
  goroutine `tgtest` starts inside a bubble must exit before the root returns. Mitigation:
  `DisableKeepAlives`, an explicit close on both pipe ends, and `t.Cleanup` registration —
  `[measured Go 1.26.5 · probe p4, a leaked sleeper → "panic: deadlock: main bubble goroutine has exited but blocked goroutines remain"]`.
- **The `httptrace` write flag is written on the transport's goroutine and read on ours.**
  A plain `bool` is a data race and `-race` is a required gate for this change. Mitigation:
  `atomic.Bool`, sticky on any `Err == nil` —
  `[derived → the race-enabled run of the retry suite in AC28]`.
- **A future `Idempotency-Key` header would silently re-send a POST inside the transport.**
  Mitigation: the caller sets no such header, and the reason is stated in its doc comment —
  `[measured Go 1.26.5 stdlib · sed -n '/func (r \*Request) isReplayable/,/^}/p' $(go env GOROOT)/src/net/http/request.go → replayable for a POST only when Idempotency-Key or X-Idempotency-Key is present]`.
- **`earliest` gets "optimised" into something that inserts out of order on an ordered key,
  or `earliest` gets replaced by an "after everything queued" read on the unordered key.**
  The two kinds look interchangeable and are not: the ordered kind's non-decreasing grants are
  what let it read the `c`-th newest directly and what make AC30 hold; the unordered kind's
  earliest-admissible answer is what stops one chat's backlog blocking another. Swapping
  either way is a silent defect. Mitigation: D9 states which kind each key uses and why, the
  invariant is asserted after every commit, and subtask 4's variant table carries
  `ordered-global` as a red-first target —
  `[derived → AC11's cross-chat scenario and AC30's per-key ordering scenario]`.
- **Retention gets "tidied" back to a count bound.** "Keep the newest `max(c)`" reads as
  obviously sufficient and is — but only on an append-only schedule. On the unordered key it
  discards grants still inside a live window and the ceiling silently breaks. Mitigation:
  D9 states retention is by time and why a count bound is unsound; `count-retention` is a
  red-first variant in subtask 4 —
  `[derived → AC10 clause (iii), the global-ceiling occupancy assertion]`.
- **A pacing key gets mapped to one window instead of both.** The two windows look redundant
  and are not: dropping the quota window admits `N+1` per `W`, and dropping the pacing window
  turns the class into a burst-then-idle emitter, which is the shape the owner's round-3
  answer rejects. Mitigation: D9 says which window bears which property, and both `pace-only`
  and `quota-only` are red-first variants aimed at different assertions —
  `[derived → AC10 clause (iii) for the bound, AC30 for the shape]`.
- **A false red-first variant is worse than none, and this document shipped one.** An earlier
  draft named a truncating pacing interval as the variant AC10(iii) should fail against —
  but with the quota window present it cannot, so the assertion could never have gone red, and
  the repair it invited was to delete the quota window that actually carries the bound.
  Mitigation: every variant in subtask 4's table is named with the assertion it must break, so
  a variant that stays green is itself the finding —
  `[derived → subtask 4's red-first demonstration, which is where a green variant surfaces]`.
- **The limiter gets re-composed.** Every defect this document went through came from a
  second component that could move an instant a first component had already committed to, and
  the cheapest-looking future change — "just use `x/time/rate` for the pacing window and keep
  the schedule for the quota" — recreates exactly that. Mitigation: D9 states the single rule
  the mechanism is built on, the decomposition names "no composed legs" in the subtask, and
  the AC10 cross-chat scenario is specified to run red against a commit-at-candidate variant
  first —
  `[derived → AC10's cross-chat scenario and its red-first demonstration]`.
- **Reclamation gets reintroduced.** "A refused call should give its slot back" is intuitive,
  was tried in round 3, and broke the cap by shrinking its memory. Under D9 a refusal mutates
  nothing, so there is no slot to give back — but a future reader may add removal on
  *cancellation* and reach the same defect by the other door. Mitigation: D9 states grants are
  never removed and why interior removal corrupts the `c`-th-newest indexing every window
  depends on; AC31's scenario runs red against a commit-then-undo variant first —
  `[derived → AC31's refusal scenario and its red-first demonstration]`.
- **A test suite that asserts emission *instants* is blind to window occupancy.** The lesson
  generalises past any one bug: an instant-list assertion reproduced exactly across two review
  rounds while proving nothing about the property that mattered. Mitigation: § Test Design
  specifies occupancy and same-instant assertions *and* the broken variant each must first go
  red against — `AGENTS.md` § Patterns 2 applied to the design's own instrument —
  `[derived → AC12 and AC10, whose red-then-green demonstrations are part of subtask 4]`.
- **The remaining hole — a call cancelled after its grant is committed — invites a "fix".**
  The leftover grant can only push later grants later, so it is safe in the only direction
  that matters, and D9 takes it deliberately. The hazard is a future repair that removes the
  grant and lands back on the round-3 defect. Mitigation: the property is stated in D9 with
  its cost, and AC31 asserts what holds — **every submitted call ends in an emission or a
  returned typed error, and no window ever over-emits.** It does not claim "nothing is
  starved": starvation is a scheduling-fairness property, it was not AC31's, and an earlier
  draft asserted it here while the mechanism did not deliver it across chats. The
  head-of-line fix (D9's unordered class-global key) is what removes the cross-chat case, and
  it is asserted by AC11's own scenario rather than by a sentence in this list —
  `[derived → AC31 for the loss property, AC11 for the cross-chat one]`.
- **A holder of `*telego.Bot` can reapply a `telego.With*` option and bypass the whole
  transport.** Executed, not inferred. Mitigation: the guard test in subtask 6 forbids any
  non-test file outside `internal/tg` from naming `telego.NewBot` or a `telego.With*` option,
  and the module has no downstream importers, so that scan is the complete population —
  `[measured telego@v1.11.2 · probe optesc, go run . → "original caller reached: false" / "replacement caller reached: true"]`.
- **Per-attempt limiter charging is a decision an implementor could silently reverse.** The
  cheaper-looking arrangement (charge once per call, retry inside) passes AC10 as worded while
  emitting a multiple of the configured rate in *requests* — the unit a flood ban counts.
  Mitigation: D2's diagram places the limiters inside the attempt loop and states the reason,
  and § Test Design flags where the choice becomes visible —
  `[derived → the AC10 and AC33 scenarios, whose attempt counts and emission instants only
  agree under per-attempt charging]`.
- **A limiter-key map that never evicts grows with distinct `(chat, class)` pairs.** Accepted
  as a known bound for the MVP's single chat (`docs/DESIGN.md` §14), with the time-based
  eviction constraint recorded in D9 so a later fix cannot reset a cap by accident —
  `[derived → the § Open questions entry, which #43's fan-out reopens]`.
- **The `stdjson` residue is mistaken for an unfinished swap.** Mitigation: D3 states exactly
  which decode remains telego's, measures that no build configuration removes `fasthttp` or
  `fastjson`, and puts the one-PR path in front of the owner —
  `[measured telego@v1.11.2 · probe p2, go list -tags stdjson -deps . → valyala/fasthttp and valyala/fastjson still present]`.
- **A textual base-URL check is weaker than the property it guards.** The source check in
  subtask 6 can only see the identifier, not every way a branch could be written.
  Mitigation: it is paired with the behavioural test (identical limiter behaviour under two
  base URLs) and with the structural fact that the limiter API takes no URL at all —
  `[derived → AC15's pair of tests]`.
- **No balance moves and no schema changes**, so there is no posting signature, no basis
  document, no event and no migration to design — the telemetry this task owes is the health
  surface of D11 (`docs/DESIGN.md` §13.2, `AGENTS.md` § Domain Rules) —
  `[derived → AC32's test that the change adds no migration and writes no limiter state outside the process]`.

---

## Test Design

Every claim below is about a test that does not exist yet.

**Shared shape.** Table-driven subtests with `t.Parallel()` on the subtest and
`synctest.Test` **inside** it (D14). Fixtures come from `internal/tgtest`; no test touches
the process environment, the network, or a real clock.
[derived → AC28's race-enabled gate.]

### `internal/config` — subtask 1

- Location: `internal/config/transport_test.go`, beside the code.
- Entry points: `Load`, `EnvKeys`.
- Scenarios: every new key absent → the documented default for each (AC22); one present and
  well-formed → that value (AC22); one present and malformed → an error that `errors.Is`
  matches `ErrInvalidValue` and whose message names that variable (AC22); a limit value of
  `off` → an unbounded `Rate`; a zero or negative count, a zero duration, a missing `/`, a
  bad duration → each rejected by name; the three-way key-set equality holds with the new keys
  included (AC23); each previously declared variable removed in turn still fails, naming it
  (AC24).
- Fixtures: the existing `mapLookup` helper and `.env.example` reader already in the package.
[derived → AC21–AC24.]

### `internal/tgtest` — subtask 2

- Location: `internal/tgtest/tgtest_test.go`.
- Entry point: the server's client, driven directly by `net/http`.
- Scenarios: each scripted behaviour AC19 lists produces what it claims — success, 429 with
  `retry_after`, 5xx, a dial-time failure with nothing written, a post-write close with no
  response, and a delayed response that advances only virtual time; the server records the
  method path and body it received; a bubble containing a full request/response cycle exits
  with no lingering goroutine.
[derived → AC19.]

### `internal/tg` classifier, options and error — subtask 3

- Location: `internal/tg/class_test.go`, `internal/tg/client_test.go`.
- Entry points: the classifier, `New`, `(*Error).Error`, `(*Error).Unwrap`.
- Scenarios: a representative name from each `send*`/`copy*`/`forward*` family classifies as
  `ClassMessage`; each `editMessage*`/`deleteMessage*`/`stop*` name in D4's rule classifies as
  `ClassEdit`; `editChatInviteLink`, `deleteWebhook`, `deleteMyCommands`, `getMe` and
  `getUpdates` classify as `ClassOther`; a hypothetical future `sendSomethingNew` classifies as
  `ClassMessage` (the fail-safe property); `New` rejects a zero attempt count, a non-positive
  base delay, a max delay below the base, and an empty base URL, each naming the field; a
  constructed `Client` has no `context.Context`-typed field; `errors.As` reaches `*Error`
  through telego's wrapping and `errors.Is` reaches a wrapped `context.DeadlineExceeded`.
[derived → AC1, AC8.]

### `internal/tg` limiters — subtask 4

- Location: **two files, split at authoring time rather than when the gate complains** —
  `internal/tg/limit_test.go` for the end-to-end scenarios and the red-first variants,
  `internal/tg/schedule_test.go` for the schedule unit tests, the `earliest` oracle and its
  control, the retention tests and the concurrency exercise. `make file-limits` is a hard gate
  inside `make verify` (AC28) and this subtask's body is large; naming the split here keeps the
  implementor from having to make a decomposition call mid-flight.
- Entry point: the schedule and the limiter's acquire function directly, plus end-to-end
  calls through `Client.API()` against `tgtest`.
- **The end-to-end half is why subtask 4 carries the minimal `Caller.Call` in its file list.**
  A limiter scenario driven through `Client.API()` needs a caller, and the caller is otherwise
  subtask 5, which depends on 4 — a cycle that would leave subtask 4 unable to commit on a
  green gate under `code-writer` Mode A. The design refused exactly this shape for
  `.env.example` and refuses it here: the dependency is broken in the file list, not left for
  the implementor to improvise, and the improvisation it would otherwise invite — demoting
  these scenarios to unit level — would drop end-to-end coverage of AC10–AC15 and AC30–AC32
  that subtask 5 does not pick up.

**Binding method for this subtask: every assertion below is written red-first.** Point it at
the named broken variant, watch it fail, then wire the real mechanism and watch it pass. A
green assertion nobody has seen fail is evidence about the assertion. This is not a
suggestion here — two of the three defects this design went through were invisible to
assertions that reproduced perfectly, and the variants below are exactly those defects
reduced to a few lines.

| Broken variant to run red against | What it reproduces |
|---|---|
| **`commit-at-candidate`** — commit to the global schedule at the candidate instant, then let a chat window push the emission later | round-3b: a bound that holds on reservations, not emissions |
| **`commit-then-undo`** — commit the grant, then remove it when the deadline check refuses | round-3a: a refusal that corrupts the window's memory |
| **`bucket-cap`** — express the quota window as a token bucket sized to its count | round-2: `b + r·W`, double the configured figure |
| **`ordered-global`** — make the class-global schedule ordered, so `earliest` answers "after everything already granted" | cross-chat head-of-line blocking and, with deadlines, starvation |
| **`count-retention`** — retain the newest `max(c)` grants instead of retaining by time | an unordered schedule silently forgets live grants and over-emits |
| **`pace-only`** — map a pacing key to its pacing window alone, dropping the quota window, with a truncated interval | `N+1` emissions per `W`: the quota window is what forbids the extra one |
| **`quota-only`** — map a pacing key to its quota window alone, dropping the pacing window | `N` emissions at one instant then an idle `W`: the pacing window is what makes emission steady |

- Scenarios and the exact configurations they use:
  - **AC10** — two parts.
    (i) message global `1/1s`, everything else `off`: message calls emit one second apart
    while interleaved `getMe` calls emit immediately.
    (ii) **the part the previous scenario could not see**: global `1/1s` *with* a chat window
    engaged, so the chat pushes an emission well past its candidate instant, and traffic from
    many other chats is released at the same moment. **No two emissions share an instant, and
    no one-second window carries more than one.** Red-first against `commit-at-candidate`.
    (iii) **the global ceiling at its shipped default**: with `LAB_GAME_TG_LIMIT_MESSAGE_GLOBAL`
    left at `30/1s`, a fan-out releasing one message into each of many chats emits **at most
    thirty in any one-second window** — the figure D10 verified as enforced. Red-first against
    `pace-only`, which admits thirty-one, and against `count-retention`, which admits far
    more. **Not** against a merely-truncated pacing interval: with the quota window present
    that variant still emits thirty, which is why D9 attributes the bound to the quota window
    and the evenness to the rounding.
  - **AC11** — message chat rate `1/1s`: traffic into chat A is spaced, traffic into chat B is
    not delayed by it, and a `ClassOther` call into chat A is not delayed by the message
    class's allowance. **And the clause the old isolation test could not carry, because it
    turned the global class off: with the global class *bounded* at its shipped default and
    chat A saturated to a long backlog, a single call into an empty chat B is granted within
    one global pacing interval — not behind chat A's backlog.** Red-first against
    `ordered-global`, where chat B is pushed to the far end of chat A's queue and, with a
    deadline, refused outright.
  - **AC12** — shape and occupancy, and the second is the load-bearing one.
    (i) *Shape*: chat rate `1/100ms`, chat cap `5/1s`, calls released into one chat — the
    burst is spread by the short window and the cap binds only once it has been spread.
    (ii) ***Occupancy*, which an instant list cannot see**: over the same run, **no window of
    length `per` contains more than `count` emissions, for every configured window** — checked
    by sliding each window across the recorded instants, not by inspecting the first one.
    Red-first against `bucket-cap`.
  - **AC13** — the edit class under the default configuration imposes no delay and allocates
    no history; the same class with a configured bound binds; **and the message class under
    its shipped defaults emits at most twenty into one chat in any sixty-second window, at
    most one in any one-second window, and at most thirty class-wide in any one-second
    window**, which are the figures D10 cites. That is where
    "traceable to a verified figure" becomes a test rather than a claim about a constructor
    argument.
  - **AC14** — a positive (private) chat id is charged on the same terms as a negative one.
  - **The undecodable-body branch (D4)** — a request whose `BodyRaw` is nil is charged against
    the reserved unknown-chat key of its class, so the class's rate and cap both bind, and its
    `Call.Chat.Target` is `ChatUnknown` when the gate sees it — not `ChatNone`, which is what a
    method addressing no chat reports. **Not** exempt from the per-chat windows:
    the red-first variant here is "treat an unknown chat as no chat", under which the same run
    shows the per-chat windows never binding at all. The MVP sends no media so this branch is
    not reached in production, which is exactly why it gets a test rather than a comment.
  - **AC15** — the same configuration under two different base URLs produces identical
    emission instants.
  - **AC30** — one key at a known interval, callers released one at a time with
    `synctest.Wait()` between them so arrival order is fixed: emissions are one interval apart
    and in arrival order. Red-first against `quota-only`, where the same run emits the whole
    allowance at one instant and then idles — the assertion that shows the pacing window earns
    its place.
  - **AC31** — two parts.
    (i) *Nothing is lost*: under saturation with a deadline shorter than the queue, the
    emissions together with the returned `*Error`s account for **every** submitted call, and
    the emissions that occur are still no closer together than every configured window allows.
    (ii) *A refusal changes nothing*: with some calls refused at their deadline and the rest
    admitted, occupancy over every window still holds — a refusal must leave the schedule
    exactly as it found it. Red-first against `commit-then-undo`.
    **This scenario must NOT assert that a refused or cancelled call's slot is reclaimed.**
    D9 declines reclamation deliberately; an assertion that later callers are pulled forward
    would fail against the mechanism, and the repair a failing test invites — loosening the
    occupancy check — is precisely the flood-safety property AC31 exists to protect. The
    permitted observation about the hole is the safe-direction one: emission instants may be
    *later* than a perfect schedule, never earlier.
  - **AC32** — the change adds no migration directory entry and the package writes no state
    outside the process.
- **Schedule unit tests, below the end-to-end scenarios.** The invariant
  `grant[j+c] - grant[j] ≥ per` is asserted after **every** commit in every scenario, not only
  in the unit tests — it is the mechanism's postcondition and the cheapest place a defect
  surfaces. Beyond that: `earliest` returns the candidate unchanged while every window is
  under-filled; it mutates nothing (repeated calls change no later answer); it never returns
  an instant before its candidate; an ordered schedule's grants are non-decreasing; an
  unordered schedule's are sorted; retention drops a grant only once `grant + max(per)` is
  past; and an empty window set never blocks.
- **`earliest` gets an oracle, because "earliest" is two claims.** One test asserts the
  returned instant is admissible; a second sweeps the interval between the candidate and the
  returned instant at fine resolution and asserts **no earlier instant is admissible**. The
  sweep is meaningless alone — a grid scan can only ever *miss* an admissible point — so it
  ships with a control that feeds it a deliberately late answer and confirms it goes red.
- **Concurrency**: the acquire path is exercised from many goroutines under `-race`, since one
  mutex now guards every schedule and AC28's race gate is where a locking mistake surfaces.
- Per-attempt charging (D2) is visible here: a scenario that forces a retry consumes more
  than one unit of allowance for one caller-visible call, so the scenarios that assert exact
  instants use first-attempt-success responses unless they mean to exercise it.
[derived → AC10–AC15, AC28, AC30–AC32.]

### `internal/tg` retry, `retry_after`, cancellation and observation — subtask 5

- Location: `internal/tg/retry_test.go`, `internal/tg/caller_test.go`.
- Entry point: a call issued through `Client.API()` against a scripted `tgtest` server.
- Scenarios:
  - **AC4** — a 429 carrying `retry_after: N` is followed by no earlier next attempt than N
    later; the assertion is on the bubble's virtual clock, and a shortened wait fails it.
  - **AC5** — with a fixed jitter source, successive delays are strictly positive and double;
    no path re-attempts at zero delay.
  - **AC6** — the fake closes the connection after the request was written: exactly one
    attempt, and the returned `*Error` has `Ambiguous` set.
  - **AC7** — a dial-time failure, a 429 and a 5xx each produce more than one attempt.
  - **AC33** — with the attempt cap set to a known value against a permanently retryable
    response, exactly that many attempts are made; and a 429 pause costs one attempt rather
    than exhausting the budget.
  - **AC8** — a give-up returns `*Error` with the method, the last status code, the attempt
    count, the ambiguity flag and the `retry_after` when the final failure was a 429, each
    asserted field by field, and distinguishable from success and from a bare context error.
  - **AC9** — a deadline shorter than the required `retry_after` (and, separately, shorter
    than the next backoff) returns immediately with the `retry_after` still carried.
  - **AC18** — cancellation while parked in a backoff, in a `retry_after` wait, and behind a
    limiter each return promptly with an error that `errors.Is` matches the context error.
  - **AC16** — a success, a retried success and a give-up each produce exactly one observation
    carrying the method, a latency, the response code, the 429 flag and the retry count.
  - **AC17** — the package's tests pass with no observer installed.
  - **AC20** — the client is constructed from a `*config.Config` produced by `config.Load` with
    `LAB_GAME_BOT_API_BASE_URL` pointing at the fake server, and the request arrives there.
[derived → AC4–AC9, AC16–AC18, AC20, AC33.]

### Package-level guards — subtask 6

- Location: `internal/tg/guards_test.go`.
- Entry point: the source tree itself, parsed with `go/parser`.
- Scenarios: no non-test file under `cmd/` or `internal/` imports a fasthttp or go-json
  package (AC2); no non-test file in `internal/tg` imports a metrics registry (AC17); the
  bot token appears in no rendered `*Error`, no observation and no fixture in the package
  (AC26); **no retry or rate-limit literal appears at a call site in `internal/tg` — the scan
  walks the package's non-test files and fails on a numeric or duration literal passed as a
  window count, a window period, an attempt count or a backoff delay, since every such value
  must arrive from `config.Transport` (AC21's second clause, which no other scenario
  discharges)**; the base-URL identifier appears only in the file that constructs the bot (AC15,
  paired with the behavioural test in subtask 4); and AC27's seam, in each part D2 showed it
  needs — a gate that refuses everything blocks a call issued through
  `Client.API()`; the package exports no way to obtain a `*telego.Bot` that was not built with
  the caller; **and no non-test file outside `internal/tg` names `telego.NewBot` or any
  `telego.With*` option**, which is what closes the option-reapplication escape D2 measured.
  That last check is a source scan, and it is complete rather than sampling only because this
  module is the whole population of code that can hold the pointer (`AGENTS.md` § API
  Stability: no downstream importers).
[derived → AC2, AC15, AC17, AC21, AC26, AC27.]

**No golden artefact is minted by this task**, so `design-writer` § Rules' golden contract
(seed, covered fields, meaning of a diff, combat-system version) has nothing to bind here —
the transport renders no combat log, no narrative and no generated maze.

---

## Open questions

**Four entries below carry the owner's answer rather than a question.** They are kept here,
marked `DECIDED`, rather than deleted: each was raised as a question by an earlier draft, and
a reader who remembers the question needs to find the answer in the same place instead of
concluding it was dropped. Everything not marked `DECIDED` is genuinely still open.


- **The `stdjson` build tag — DECIDED: not now, and not in this PR.** D3 measured that the tag
  works and that it removes `grbit/go-json` from the build graph, and that the honest cost is
  threading `-tags stdjson` through the Makefile, `.golangci.yml`, `AGENTS.md` § Build & Test
  and every skill that spells a bare `go` gate. The owner's disposition is **a separate focused
  PR later**, whose whole content is that propagation. D3's residue analysis stands unchanged
  and is not a loose end: the residue is telego decoding its own generated result types with a
  drop-in `encoding/json` reimplementation, and the propagation is **deliberately deferred to
  its own PR**, not left undecided.
- **The local toolchain — DECIDED: stays at Go 1.26.5 with `GOTOOLCHAIN=local`, and telego
  stays pinned at `v1.11.2`.** D1's measurement of the `v1.12.x` ceiling stands as the reason
  the pin is where it is; the question is closed rather than deferred, so an implementor who
  finds `v1.12.x` unreachable is seeing the designed state, not a stale document.
- **`docs/DESIGN.md` §11 and the per-chat per-second figure — DECIDED: do not touch §11.**
  The owner declines the edit for now. The transport expresses both windows either way (D9,
  D10), so nothing here depends on it. Recorded as a **known, accepted incompleteness** in a
  decisions document this task does not edit (`AGENTS.md` § Project): §11 names the per-minute
  figure and not the per-second one that D10 verified upstream as published.
- **Should the observation carry the final round-trip separately from the call's latency?**
  D11 reports the caller-visible total. #23 may want the transport-only number to keep limiter
  waits out of the health latency histogram; that is a per-attempt observation, not a field,
  and it is #23's call to make once it has a dashboard to look at.
- **Should the `(chat, class)` schedule registry evict?** Not for one MVP chat. #43's fan-out
  is the trigger, and D9 records the one constraint a later fix must respect: dropping a key
  discards its grants and resets every window it enforced, so eviction must be time-based.
- **Should a grant be reclaimed when its call is cancelled mid-wait?** D9 declines it, and the
  history is the argument: reclamation has been attempted once and broke the property it was
  added to protect. The hole costs throughput in the safe direction only, and it ages out
  after `per`. If it is ever revisited, the shape that would work is not removal — it is
  deciding the grant at emission time rather than on arrival, which trades away the
  arrival-order guarantee AC30 rests on. Nothing has measured a need for that trade, and the
  owner's answer this round keeps #19 on in-process pacing with #43 owning the durable queue.
- **Is one mutex for the whole limiter the right granularity?** D9 takes it deliberately, to
  keep the decision atomic across every schedule; per-key locking would restore the
  multi-component structure that produced three defects. Contention is the thing to measure if
  fan-out ever makes it a question — and any replacement must keep the decide-then-commit rule
  intact, not merely shard the map.
- **`LAB_GAME_TG_ATTEMPT_TIMEOUT` — DECIDED: keep it, default `30s`, as designed.** The owner
  accepts it as an addition beyond the spec's literal criteria, motivated by
  `docs/DESIGN.md` §12.2's wedged-instance incident. It is no longer an open question and
  should not be reopened as an unasked addition.
- **Should the class-global key ever emit in arrival order?** D9 makes it unordered so a
  backlogged chat cannot hold a quiet one behind it, which costs global arrival order — a
  property no acceptance criterion asks for. If a later mechanic ever needs class-wide
  fairness *and* ordering, that is a queue discipline question and belongs where the queue
  lives (#43), not in the transport's ceiling.
- **Should `sendChatAction` leave `ClassMessage`?** It delivers no message but is throttled
  today because the `send*` rule is deliberately fail-safe. The MVP sends none; a later
  "typing…" indicator that feels laggy is the signal to carve it out.
- **The leaky-bucket reading, and the 5xx residue** — both carried forward from the spec
  unchanged. The owner's round-3 choice was shaping with steady emission and no burst, and D9
  now expresses it without a burst parameter at all: a pacing key becomes the window
  "at most one per interval", which *is* steady emission. A quota key is a bound, not a pacer,
  so the leaky-bucket question does not reach it. If either reading is overturned the change
  stays contained — the pacing/quota mapping is one table in D9, and the 5xx row is one line
  of the D5 table.
