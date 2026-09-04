# Design: Bot API transport over telego — retries, `retry_after`, rate limits, base-URL axis

**Issue:** #19
**Date:** 2026-09-04

> **Claim-tag conventions in this document.** A repo fact is pinned to the commit it was
> read at (`31736b4`, from `git rev-parse --short HEAD` in the same turn as every read
> below). A fact about an **external module, an external tool, the standard library, or a
> published web page** has no repo path, so its pin is the **version** (or the `go doc`
> target, or the page plus its fetch date) the probe ran against; the probe modules live
> outside the repo, under the session scratchpad. A claim about an artefact this task has
> **not yet built** carries `[derived → …]` and no locator. `docs/DESIGN.md` is cited by
> section per the design-writer contract, so those citations carry no `[measured …]` tag.

---

## Approach

`internal/tg` is a thin layer whose **entire policy lives below telego's generated API**,
in one implementation of telego's own `telegoapi.Caller` interface. telego's `Bot` builds
the URL and the request body from its generated types and then hands both to that one
method; everything this task owns — the retry loop, the `retry_after` wait, both
limiters, the outbound gate seam #22 needs, and the per-call observation — happens inside
it. Nothing above it can route around it, and nothing below it exists.

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
  subtask 4.

*Reason for the dependency* (`AGENTS.md` § Dependency Versions, and AC3's requirement that
the design state it): KD-2 already chose it; this task is where the choice first becomes
code. AC3's remaining clauses — a **direct** requirement at a **released** version with
`go.sum` in agreement and no `go mod tidy` delta — are what the importer-binding above
protects, and subtask 4's gate is `make tidy-check` itself.

**Open for the owner, not blocking:** raising the local toolchain to ≥ 1.26.7 unlocks
`v1.12.x`. Recorded in § Open questions.

### D2 — The transport is one `telegoapi.Caller`; `tg.Client` owns configuration, not calls

```
telego.Bot (generated methods)  →  tg.caller.Call(ctx, url, data)  →  net/http
                                     ├─ gate (D12)
                                     ├─ limiters (D9)
                                     ├─ attempt loop: backoff (D6), retry_after (D7)
                                     └─ observation (D11)
```

`tg.Client` holds the configured `*telego.Bot`, the limiter registry, the retry settings,
the gate and the observer; it exposes the bot through one accessor. Every outbound call in
the project is a call on that bot, and every one of them lands in our caller.

The accessor is deliberate, not a leak: because the seam is *below* the generated API,
handing the generated API out costs nothing — there is no reachable path that skips the
caller. That is exactly what AC27's second clause asks for, and it preserves KD-2's
"complete coverage by construction" without wrapping a single method
[derived → AC27's test: a refusing gate blocks a call issued through the accessor, and the
package exposes no constructor for a bot without the caller].

Exported shape [derived → AC1]:

- `Client`, `New(Options) (*Client, error)`, `(*Client).API() *telego.Bot`
- `Options` — base URL, bot token, the `config.Transport` settings, optional `Gate`,
  optional `Observer`, optional jitter source, optional `*http.Client`
- `MethodClass`, `Call`, `ChatRef`, `Gate`, `Observation`, `Observer`, `Error`

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
URL path is the method name — observed live in the probe (`method: sendMessage`).

The chat is read from the request body by decoding **only** the `chat_id` field as a
`json.RawMessage` — also observed live (`server chat_id token: -1001234567890`). This is
total across the Bot API by construction: every chat-scoped method names its destination
chat in that one field, and a method with no `chat_id` (an inline-message edit, `getMe`,
`getUpdates`) yields no chat key and charges no per-chat bucket. A `chat_id` may be a
`@channelusername` string rather than a number, so the key is the raw JSON token, not an
`int64` — one stable key per chat either way.

**What this buys that a per-call-site tag cannot.** It cannot be forgotten by a
caller, and it covers methods this project has not written yet — including everything #22
and #43 will add.

**The token is in that URL, and never leaves the caller.** The method name is the only thing
extracted; no error message, log line or observation carries the URL
[derived → AC26's test: the token string appears in no rendered `*Error`, no observation,
and no fixture].

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
| HTTP response, status 429 | yes, after `retry_after` (D7) | no |
| HTTP response, status ≥ 500 | yes | no |
| HTTP response, any other non-success or `ok:false` | no — terminal | no |
| HTTP response, `ok:true` | success | — |
| No response, and no successful request write | yes | no |
| No response, and the request **was** fully written | **no — terminal** | **yes** |
| Context cancelled or deadline passed | no — terminal | per the write evidence above |

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

The source is `Options.Jitter func() float64`, defaulting to `math/rand/v2`'s `Float64`. It
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
    Err         error         // underlying cause: net error, context error, gate refusal
}
```

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

### D9 — The limiters use `golang.org/x/time/rate`, and the wait is ours

**Adopted: `golang.org/x/time/rate`** as a new direct requirement, published through
`v0.15.0`
[measured · `go list -m -versions golang.org/x/time` → `… v0.13.0 v0.14.0 v0.15.0`] and not
reachable from this module today
[measured 31736b4:go.mod · `go mod why -m golang.org/x/time` → `# golang.org/x/time` /
`(main module does not need module golang.org/x/time)`; `grep -c 'golang.org/x/time' go.mod` → `0`].

**The claim about its algorithm was checked against its source, not its name**, as the spec
required. `rate.Limiter` is a token bucket of size `b` refilled at `r`
[measured golang.org/x/time@v0.15.0:rate/rate.go · `sed -n '/^\/\/ A Limiter controls/,/^type Limiter struct/p' rate/rate.go` →
"It implements a \"token bucket\" of size b, initially full and refilled at rate r tokens per second"].
**A token bucket with `burst == 1` is a shaping leaky bucket at rate `r`** — emission is one
event per interval with no burst beyond the first — which is the mechanism the owner chose
in round 3, and the equivalence is the article's own (the spec's Key-decisions row records
the meter/token-bucket equivalence). This is not a rename of the rejected option: the
rejected option is the *burst-allowing* configuration, and `burst` is ours to set.

Measured, not argued: with `rate.Every(time.Second)` and `burst 1`, successive `Wait` calls
returned at exactly `0s`, `1s`, `2s`
[measured golang.org/x/time@v0.15.0 · scratchpad probe `p4` inside a `testing/synctest`
bubble → `emit 0 at 0s` / `emit 1 at 1s` / `emit 2 at 2s`]. That is AC30's steady emission,
demonstrated on the candidate before adopting it.

**What the package cannot express, and is therefore ours:** the exported `Wait`/`WaitN`
return the package's own "would exceed context deadline" error rather than a context error
or ours
[measured golang.org/x/time@v0.15.0:rate/rate.go · `sed -n '/^func (lim \*Limiter) wait(/,/^}/p' rate/rate.go` →
`r := lim.reserveN(t, n, waitLimit)` / `if !r.ok { return fmt.Errorf("rate: Wait(n=%d) would exceed context deadline", n) }`].
AC9 and AC18 need our typed error and a context error respectively, so the wait is written
on the package's **documented reserve-and-wait pattern**
[measured golang.org/x/time@v0.15.0:rate/rate.go · `sed -n '/^\/\/ ReserveN returns a Reservation/,/^func (lim \*Limiter) ReserveN/p' rate/rate.go` →
the usage example `r := lim.ReserveN(time.Now(), 1)` / `time.Sleep(r.Delay())` / `Act()`, and
"Use this method if you wish to wait and slow down in accordance with the rate limit without dropping events"]:
reserve, compare the delay against the deadline, sleep or cancel. `Reservation.Cancel`
returns the tokens so a refused call displaces nobody
[measured golang.org/x/time@v0.15.0:rate/rate.go · `sed -n '/^\/\/ CancelAt indicates/,/^func (r \*Reservation) CancelAt/p' rate/rate.go` →
"reverses the effects of this Reservation on the rate limit as much as possible"] — that is
AC31's "no other call is displaced".

**Rejected — `go.uber.org/ratelimit`.** It is a genuine leaky-bucket shaper with an
injectable clock, but its `Take()` takes no `context.Context`, so a call parked behind it
cannot be cancelled — a direct contradiction of Scope 10 and AC18. Not adopted, and not
probed further once that blocked it.

**Rejected — hand-rolling the bucket.** `AGENTS.md` § Dependency Versions refuses both "it's
only a few lines" and bare dependency aversion outright
[measured 31736b4:AGENTS.md:138-139 · `sed -n '138,139p' AGENTS.md` → "*\"It's only 10–20
lines — cheaper than writing the import\"* | **REFUSED.** Line count is not the cost." and
"*\"Better to write our own than to pull in an established dependency\"* | **REFUSED.**
Dependency aversion is not a reason by itself."], and neither escape the same table allows
applies: `x/time/rate` is maintained, and its API *does* express the accounting. Only the
wait needed writing, and the package documents that split itself.

**Structure.** Every call charges its class's buckets, so there is no special case for "what
charges the buckets" — the class table is the whole answer:

- **global**, one shaper per class, `burst 1`;
- **per chat**, keyed on `(chat key, class)`, a **shaping rate** (`burst 1`) and a **window
  cap** (`burst = count`). Those roles are why the keys are named `_CHAT_RATE` and
  `_CHAT_CAP` (D10) rather than "short" and "long": a cap shaped at burst 1 would emit one
  message every three seconds into a chat and would be stricter than any published figure.

`Reserve` does not block, so acquiring all applicable buckets is allocation-order-free and
there is no lock-ordering hazard; the call then waits once, until the **latest** of the
reserved times, and each bucket is charged exactly once. If that instant is past the
caller's deadline, every reservation is cancelled and the call returns the D8 error. A
private chat is charged on the same terms as a group — no chat is exempt by construction
(spec Key decisions).

**A `Rate` with zero count is unbounded**, implemented as `rate.Inf`, which the package
documents as allowing all events regardless of burst
[measured golang.org/x/time@v0.15.0:rate/rate.go · `sed -n '/^\/\/ Inf is the infinite rate limit/,/^const Inf/p' rate/rate.go` →
"Inf is the infinite rate limit; it allows all events (even if burst is zero)"].

**Nothing is ever discarded.** The bucket only delays; a call that cannot be emitted before
its deadline returns the typed error. Silent drop is a defect, not a tuning choice (spec
Technical constraints).

**The per-key registry grows with distinct `(chat, class)` pairs, and this design does not
evict — recorded as a known bound.** The MVP ships to one friendly chat (`docs/DESIGN.md`
§14), the entries are small, and #43's fan-out is the trigger to revisit. One correctness
note for whoever adds eviction: an evicted key returns a *full* bucket, so eviction must be
time-based — only a key idle longer than its longest window may be dropped, or the cap it
enforced is silently reset.

**Nothing here persists.** The state is in-process, lives with the calls in flight, and
touches no table, no disk and no migration [derived → AC32].

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
sourced.** `3` attempts with a `500ms` base and equal jitter puts the worst case at roughly
three seconds of waiting before a give-up — long enough to ride out a transient blip,
short enough that a player's action does not appear to hang; `30s` caps the scale so a
higher configured attempt count cannot grow the wait without bound. Each is an operational
tuning value an operator may change, which is the entire point of the key class.

**What this class does to the configuration layer, checked claim by claim.**

- **The disjointness test survives, provided every key is queried unconditionally.**
  `recordingLookup` records a key at query time, before delegating
  [measured 31736b4:internal/config/disjoint_test.go · `sed -n '/^func recordingLookup/,/^}/p' internal/config/disjoint_test.go` →
  the wrapper appends `key` to `keys` and only then `return base(key)`], so an absent
  optional key is still in the recorded set. The loader therefore calls `lookup` for **every**
  transport key on every load, with no early return and no branch that skips one.
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
  `config.go` assert the falsified claim — `balance.go`, `doc.go` and `key-decisions.md`
  line 61 (KD-22) are scoped to balance values, `context-status.md` line 79 likewise, and
  `context.md` line 43 is the separate "no Telegram client" claim that `/task` Step 9.5 owns.

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
type ChatRef struct { Key string; Known bool }
type Call    struct { Method string; Class MethodClass; Chat ChatRef }
type Gate    interface{ AllowCall(ctx context.Context, call Call) error }
```

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
durably blocked."]. Production code therefore uses `time.Now`, `time.NewTimer` and
`x/time/rate` **directly**, with no clock interface threaded through the transport — and the
tests still get exact, instantaneous virtual time. That is fewer moving parts in production
*and* stronger assertions.

Demonstrated on this exact stack before adopting it: a handler sleeping two virtual seconds
over a real `http.Transport`, `httptrace` firing, `retry_after` decoded, and the limiter
emitting at exact one-second boundaries — all in `0.00s` of wall time
[measured Go 1.26.5 + golang.org/x/time@v0.15.0 · scratchpad probe `p4`,
`go test -v -run TestSynctestHTTP` → `status 429 … wrote true elapsed 2s` /
`emit 0 at 0s` / `emit 1 at 1s` / `emit 2 at 2s` / `--- PASS: TestSynctestHTTP (0.00s)`].

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
| 1 | `internal/config`: the optional-with-default transport key class — the value types of D10, the key names, the compiled-in defaults, the `<count>/<duration>`\|`off` grammar and its validation, `Load` + `EnvKeys` wiring with every key queried unconditionally, and the falsified doc comments in `env.go` and `config.go` rewritten (AC25). Tests first: absent → default, present → parsed, malformed → `*KeyError` naming the key, the three-way key-set equality, and the previously declared variables still required. | `internal/config/transport.go`, `internal/config/transport_test.go`, `internal/config/env.go`, `internal/config/config.go` | — |
| 2 | `.env.example`: each new key documented with its default as a non-empty value, in the file's existing commented style. Lands with subtask 1's loader or the disjointness test fails. | `.env.example` | 1 |
| 3 | `internal/tgtest`: the in-process fake Bot API server of D13 — `net.Pipe` dialer, the `.invalid` base URL, the scripted behaviours AC19 lists, the fake token constant, and its own tests. | `internal/tgtest/tgtest.go`, `internal/tgtest/tgtest_test.go` | — |
| 4 | `internal/tg` foundations **and the telego dependency**: package comment, `Error`, `Observation`/`Observer`, `MethodClass` + the D4 classifier, `Gate`/`Call`/`ChatRef`, `Options` + `New` + `API`. `go get github.com/mymmrac/telego@<pinned>` runs in this subtask, with the importing file, so `make tidy-check` stays green (D1). Tests: the classifier over the pinned version's method names, option validation, error rendering and unwrapping. | `go.mod`, `go.sum`, `internal/tg/doc.go`, `internal/tg/errors.go`, `internal/tg/observe.go`, `internal/tg/class.go`, `internal/tg/client.go`, `internal/tg/class_test.go`, `internal/tg/client_test.go` | 1 |
| 5 | `internal/tg` limiters: the per-class global shaper, the per-`(chat, class)` shaping rate and window cap, the reserve/compare/wait/cancel loop of D9, `rate.Inf` for an unbounded class, and the fixed charge-once discipline. Tests: per-class global admission, per-chat isolation, unbounded-class pass-through and its bound counterpart, private chats charged, steady ordered emission, the cap binding after the burst is spread, saturation ending in emission or error, and identical behaviour under two different base URLs. | `internal/tg/limit.go`, `internal/tg/limit_test.go` | 3, 4 |
| 6 | `internal/tg` caller: the `encoding/json` request constructor, the attempt loop, the `httptrace` write-evidence classifier, equal-jitter backoff, exact `retry_after` honouring with the deadline bound, the gate call, and the single observation. Tests: no-shortened `retry_after`, strictly positive and growing delays, the ambiguous case making exactly one attempt, each retryable case, give-up field by field, deadline refusal, cancellation at every waiting site, the attempt cap, and the observation for a success, a retried success and a give-up. | `internal/tg/constructor.go`, `internal/tg/caller.go`, `internal/tg/retry.go`, `internal/tg/caller_test.go`, `internal/tg/retry_test.go` | 5 |
| 7 | `internal/tg` package-level guard tests: the import scan over `cmd/` and `internal/` non-test files (no fasthttp, no go-json, no metrics registry), the token-absence sweep, the seam-cannot-be-bypassed test, the base-URL-appears-only-in-the-constructor source check, and the end-to-end call against `tgtest` built from a `config.Load`-produced `BotAPIBaseURL`. | `internal/tg/guards_test.go` | 6 |
| 8 | `ai-docs/key-decisions.md`: rewrite KD-2 for the shipped reality (pinned version, the caller/constructor swap, the `stdjson` residue and why it was not taken, the toolchain ceiling), and add the decisions this task settles — the limiter package with its rejected alternatives, `testing/synctest` in place of a clock abstraction, and the optional-with-default key class with its boundary. | `ai-docs/key-decisions.md` | 7 |

**AC29's propagation set, and who owns each site.** Membership is decided by `AGENTS.md`
§ Propagation Rule step 4 and is not bounded by the spec's illustrative list; what this
design fixes is the ownership, so nothing falls between the design and the workflow.
`internal/config/env.go` and `internal/config/config.go` are subtask 1's (AC25);
`.env.example` is subtask 2's; `ai-docs/key-decisions.md` KD-2 is subtask 8's.
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

`M = 8`, so grouping applies — the section is required for every `M ≥ 1`, single-subtask
designs included. Two groups, homogeneous by change-type, minimised, each marked with its
implementor model and effort. The group size cap of `10` is a **maximum**, not a target: a
group ends at whichever comes first — the cap, a change-type switch, or a dependency-forced
boundary. Here the change-type switch is what ends Group A, well below the cap.

- **Handoff into Group A:** spawn `/context-reset` per
  `.claude/skills/context-reset/SKILL.md` § Compaction recovery (re-entry). The handoff is
  bound at the start of **every** design-defined group, including the first.
- **Group A** — model `sonnet`, effort `medium` (pinned) via the `code-writer` subagent,
  1M-token window — subtasks 1–7 (code change-type: `*.go`, `go.mod`/`go.sum`, `.env.example`).
  All same-change-type subtasks are clustered into ONE group rather than interleaved with
  subtask 8; within the size cap of 10.
  `.env.example`, `go.mod` and `go.sum` are code-group artefacts by this repository's own
  classification — CI's `go` paths filter lists them alongside `**/*.go`
  [measured 31736b4:.github/workflows/ci.yml · `sed -n '/            go:/,/            harness:/p' .github/workflows/ci.yml` →
  `- '**/*.go'`, `- '**/*.sql'`, `- 'go.mod'`, `- 'go.sum'`, `- '.golangci.yml'`, `- 'Makefile'`,
  `- '.github/workflows/**'`, `- 'config/**'`, `- '.env.example'`].
- **Handoff after Group A:** spawn `/context-reset` per
  `.claude/skills/context-reset/SKILL.md` § Compaction recovery (re-entry). Parent `/task`
  resumes in Group B with fresh context.
- **Group B** — model `inherit` (the orchestrator's), effort inherited from the orchestrator
  (typically xHigh) — NOT pinned — via `general-purpose` with no inline `model=`, 1M-token
  window — subtask 8 (instructions/harness change-type: `ai-docs/**`). Terminal group
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
  moved by implementation time. Mitigation: subtask 4 runs `go get` and reads its output
  rather than trusting this document; if a newer release resolves, take it and record the
  version actually pinned. The failure mode is loud, not silent —
  `[measured telego@v1.12.1 · go get … → "requires go >= 1.26.7 (running go 1.26.5; GOTOOLCHAIN=local)"]`.
- **A standalone "add the dependency" step would fail `make tidy-check`.** `go mod tidy`
  removes an unimported requirement. Mitigation: the decomposition binds `go get` to the
  first importing file (subtask 4) —
  `[measured 31736b4 + telego@v1.11.2 · scratch copy, go get with no importer then go mod tidy → grep -c 'mymmrac/telego' go.mod = 0]`.
- **The transitive `testify` bump breaks an existing suite.** Mitigation: already exercised —
  every test binary in the tree compiles against `v1.12.1`, and the running of them is
  subtask 4's gate —
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
- **A limiter-key map that never evicts grows with distinct `(chat, class)` pairs.** Accepted
  as a known bound for the MVP's single chat (`docs/DESIGN.md` §14), with the time-based
  eviction constraint recorded in D9 so a later fix cannot reset a cap by accident —
  `[derived → the § Open questions entry, which #43's fan-out reopens]`.
- **The `stdjson` residue is mistaken for an unfinished swap.** Mitigation: D3 states exactly
  which decode remains telego's, measures that no build configuration removes `fasthttp` or
  `fastjson`, and puts the one-PR path in front of the owner —
  `[measured telego@v1.11.2 · probe p2, go list -tags stdjson -deps . → valyala/fasthttp and valyala/fastjson still present]`.
- **A textual base-URL check is weaker than the property it guards.** The source check in
  subtask 7 can only see the identifier, not every way a branch could be written.
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

### `internal/tgtest` — subtask 3

- Location: `internal/tgtest/tgtest_test.go`.
- Entry point: the server's client, driven directly by `net/http`.
- Scenarios: each scripted behaviour AC19 lists produces what it claims — success, 429 with
  `retry_after`, 5xx, a dial-time failure with nothing written, a post-write close with no
  response, and a delayed response that advances only virtual time; the server records the
  method path and body it received; a bubble containing a full request/response cycle exits
  with no lingering goroutine.
[derived → AC19.]

### `internal/tg` classifier, options and error — subtask 4

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

### `internal/tg` limiters — subtask 5

- Location: `internal/tg/limit_test.go`.
- Entry point: the limiter registry's acquire function, and end-to-end calls through
  `Client.API()` against `tgtest`.
- Scenarios and the exact configurations they use:
  - **AC10** — message global `1/1s`, everything else `off`: message calls emit one second
    apart while interleaved `getMe` calls emit immediately.
  - **AC11** — message chat rate `1/1s`: traffic into chat A is spaced, traffic into chat B is
    not delayed by it, and an `ClassOther` call into chat A is not delayed by the message
    class's allowance.
  - **AC12** — chat rate `1/100ms`, chat cap `5/1s`, ten calls released into one chat: the
    first nine are spaced by the shaping rate (`0, 100ms, …, 800ms`) and the tenth is pushed to
    `1s` by the cap rather than to `900ms` — the cap binding once the burst is spread is the
    difference between those two instants, and the assertion is on the exact instants.
  - **AC13** — the edit class under the default configuration imposes no delay; the same class
    with a configured bound binds.
  - **AC14** — a positive (private) chat id is charged on the same terms as a negative one.
  - **AC15** — the same configuration under two different base URLs produces identical
    emission instants.
  - **AC30** — one key at a known interval, callers released one at a time with
    `synctest.Wait()` between them so arrival order is fixed: emissions are one interval apart
    and in arrival order.
  - **AC31** — under saturation with a deadline shorter than the queue, every submitted call
    ends in an emission or a returned `*Error`, and the emissions together with the returned
    errors account for every submitted call — never silence; a refused call's reservation is
    cancelled, so a later call is not displaced.
  - **AC32** — the change adds no migration directory entry and the package writes no state
    outside the process.
[derived → AC10–AC15, AC30–AC32.]

### `internal/tg` retry, `retry_after`, cancellation and observation — subtask 6

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

### Package-level guards — subtask 7

- Location: `internal/tg/guards_test.go`.
- Entry point: the source tree itself, parsed with `go/parser`.
- Scenarios: no non-test file under `cmd/` or `internal/` imports a fasthttp or go-json
  package (AC2); no non-test file in `internal/tg` imports a metrics registry (AC17); the
  bot token appears in no rendered `*Error`, no observation and no fixture in the package
  (AC26); a gate that refuses everything blocks a call issued through `Client.API()`, and the
  package exports no way to obtain a `*telego.Bot` that was not built with the caller (AC27);
  the base-URL identifier appears only in the file that constructs the bot (AC15, paired with
  the behavioural test in subtask 5).
[derived → AC2, AC15, AC17, AC26, AC27.]

**No golden artefact is minted by this task**, so `design-writer` § Rules' golden contract
(seed, covered fields, meaning of a diff, combat-system version) has nothing to bind here —
the transport renders no combat log, no narrative and no generated maze.

---

## Open questions

- **Should the module adopt the `stdjson` build tag?** D3 measured that it works and that it
  removes `grbit/go-json` from the build graph, and that the honest cost is threading
  `-tags stdjson` through the Makefile, `.golangci.yml`, `AGENTS.md` § Build & Test and every
  skill that spells a bare `go` gate — a harness-wide change this transport task does not
  claim. If the owner wants it, it is one focused PR whose whole content is that propagation.
  Until then the residue is telego decoding its own generated result types with a drop-in
  `encoding/json` reimplementation.
- **Should the local toolchain move to ≥ Go 1.26.7?** It is the only thing between this
  project and telego `v1.12.x`. Nothing here blocks on the answer; `v1.11.2` carries every
  mechanism this design uses.
- **Should `docs/DESIGN.md` §11 gain the per-chat per-second figure?** The spec raised it from
  an unverified report; it is now verified against the official source, with the modality
  intact — "In a single chat, avoid sending more than one message per second", advisory,
  alongside the enforced "not be able to send more than 20 messages per minute" in a group.
  §11 names only the per-minute figure, so it is incomplete rather than wrong. `docs/DESIGN.md`
  is decisions and this task does not edit it (`AGENTS.md` § Project); the transport expresses
  both windows either way.
- **Should the observation carry the final round-trip separately from the call's latency?**
  D11 reports the caller-visible total. #23 may want the transport-only number to keep limiter
  waits out of the health latency histogram; that is a per-attempt observation, not a field,
  and it is #23's call to make once it has a dashboard to look at.
- **Should the `(chat, class)` limiter map evict?** Not for one MVP chat. #43's fan-out is the
  trigger, and D9 records the one constraint a later fix must respect: eviction must be
  time-based, because a dropped key returns a full bucket.
- **Should `sendChatAction` leave `ClassMessage`?** It delivers no message but is throttled
  today because the `send*` rule is deliberately fail-safe. The MVP sends none; a later
  "typing…" indicator that feels laggy is the signal to carve it out.
- **The leaky-bucket reading, and the 5xx residue** — both carried forward from the spec
  unchanged. This design implements the shaping reading (burst 1) and retries 5xx; if either
  is overturned, the change is contained: `burst` is one constant per limiter role, and the
  5xx row is one line of the D5 table.
