# Design: Update ingestion — long polling, dispatch, operation idempotency, chat allowlist

**Issue:** #22
**Date:** 2026-09-06
**Amended:** 2026-09-07 — round 6, owner-decided: the exponential ramp's growth factor becomes
configuration with a default of `2` (D20, decomposition rows 13-16). Every decision rounds 1-5
settled stands; nothing else in this document moves. Rounds 7-9 changed no decision: they fixed the
amendment's own coverage (a non-default-factor scenario is per **production call site**, so
`internal/scheduler` takes two), extended D20's stale-claim checklist to the doubling prose inside
`internal/backoff` and `internal/tg`, and folded in the spec's round-8 amendment, which records D20
as **AC39-AC43** and adds the factor to AC27 — so no clause of D20 now traces to prose alone.

## Approach

A new `internal/ingest` package holds the whole front door: the long poll, the router, the
per-update transaction with its bounded retry, the persisted offset, the give-up record, the
observation seam, and the `tg.Gate` implementation the allowlist rides on. Its structural model is
`internal/scheduler` — a Postgres-backed loop with an immutable registry, one transaction per
attempt handed to a consumer-declared handler, a terminal give-up state, and an observer interface
with no exporter — because that shape is already shipped
[measured 1fce8b5:internal/scheduler/doc.go:1-4 · `sed -n '1,4p' internal/scheduler/doc.go` →
`Package scheduler implements a Postgres-backed task scheduler: one worker over the scheduled_task
table, claiming due rows with FOR NO KEY UPDATE ... SKIP LOCKED and executing each in its own
transaction`], already tested against a real database
[measured 1fce8b5:ai-docs/go-test-conventions.md:42 · `grep -n 'testdb.Schema'
ai-docs/go-test-conventions.md` → `42:- Integration tests run against a real Postgres provisioned by
internal/testdb … Each test takes its own schema from testdb.Schema(t) and applies the same
migrations production applies (store.Migrate).`], and the spec names it as the precedent (spec Scope 5, Scope 12).

### Key decisions

**D1 — `internal/ingest`, one package, no sub-packages.** The loop, the router, the offset, the
give-up store, the observer and the gate are one cohesive unit with one contract surface; splitting
them would produce packages that only ever import each other. The package imports `internal/tg`
(the client and the `Gate`/`Call` types), `internal/config`, `internal/store` (the
`ErrAlreadyPosted` sentinel and the owner read of D11), `internal/backoff` (D2) and `telego`.
`internal/tg` gains no import of `internal/store` and no pool: the gate is consumer-declared there
[measured 1fce8b5:internal/tg/gate.go:66-79 · `sed -n '66,79p' internal/tg/gate.go` → `Gate is the
outbound seam #22 installs its ALLOWED_CHAT_IDS allowlist into … This package installs no allowlist
itself`], so its implementation lives here (spec Scope 8's fourth property, AC15).

> **Amended by D20 (round 6).** Everything D2 fixes stands — the shared package, the
> delete-and-re-point, the per-file dispositions, the call-site gate and its mutation probe —
> with one substitution, and it applies **wherever this document speaks of the ramp**: in D2 below,
> in the § Risks rows that restate it, and in the § Test Design entries for subtasks 1 and 2.
> Wherever any of them says *doubling* or `2^i`, read *the configured growth factor, whose
> compiled-in default is `2`*. Because that default is unchanged, every ramp value D2's gate pins is
> unchanged too, and D2's behaviour-preserving claim survives the amendment intact. D20 carries the
> amended contract; the rounds-1-5 text is left as it was verified.

**D2 — the exponential ramp is lifted into `internal/backoff`, not copied again.** `internal/tg`
and `internal/scheduler` already own a `min(base·2^i, ceiling)` ramp: the equal-jitter delay
[measured 1fce8b5:internal/tg/retry.go:30-44 · `sed -n '30,44p' internal/tg/retry.go` → `func
backoffDelay(base, maxDelay time.Duration, attempt int, jitter func() float64) time.Duration`] and
`internal/scheduler`'s plain doubling [measured 1fce8b5:internal/scheduler/cadence.go:40-55 ·
`sed -n '40,55p' internal/scheduler/cadence.go` → `func backoff(failures int, base, ceiling
time.Duration) time.Duration`]. AC23 needs one more. `design-writer` § Rules forbids another
copy-paste and forbids "minimal surface" as the justification for one, so this task creates
`internal/backoff` exporting `Exponential(attempt int, base, ceiling time.Duration)` (zero-based
attempt) and `EqualJitter(attempt int, base, ceiling time.Duration, jitter func() float64)`, and
re-expresses **both** existing sites in terms of it. The call sites after this change are
`internal/backoff` (definition), `internal/tg`, `internal/scheduler` and `internal/ingest`, and #43's
outbound queue is the "more to come" trajectory the rule names.

**`Exponential` clamps at `ceiling` *before* the doubling that would overflow, and that is part of
the exported contract rather than an implementation detail the tests happen to miss.** Neither ramp
being replaced doubles a value it has not first tested against the ceiling, and both leave the loop
at that test — `internal/tg`'s at
[measured c221784:internal/tg/retry.go:33-34 · `sed -n '33,34p' internal/tg/retry.go` →
`if d > maxDelay/2 {` / `d = maxDelay`], `internal/scheduler`'s at
[measured c221784:internal/scheduler/cadence.go:46-47 · `sed -n '46,47p'
internal/scheduler/cadence.go` → `if d >= ceiling {` / `return ceiling`]. A shared implementation
written as `base << attempt`, or as a loop whose ceiling test comes *after* the doubling, is
therefore a regression rather than a translation: `time.Duration` is an `int64` of nanoseconds, and a
doubling past its range wraps to a negative value. That attempt argument is operator-reachable, not
theoretical — the attempt caps are parsed by `lookupPositiveInt`, which bounds a value from below
only [measured c221784:internal/config/transport.go:207-217 · `sed -n '207,217p'
internal/config/transport.go` → `func lookupPositiveInt(lookup Lookup, key string) (int, bool,
error)` whose only rejection is `if err != nil || n <= 0`], and `internal/scheduler` feeds the row's
`consecutive_failures` straight into the ramp at the persisted-`run_at` computations
[measured c221784:internal/scheduler/settle.go:142,242 · `grep -n 'backoff(k,'
internal/scheduler/settle.go` → `142:		runAt := s.Add(backoff(k, cfg.RetryBaseDelay,
cfg.RetryMaxDelay))` and the same expression at `242`]. A negative delay there persists a `run_at` in
the past: a hot-looping dead task, which would be a data-visible regression inside a change this
design bills as behaviour-preserving. `Exponential`'s stated contract is therefore, over the domain
both adopters' own constructors already enforce — `base > 0` and `ceiling > 0`
[measured c37e642:internal/tg/client.go:109-113 and internal/scheduler/worker.go:78-84 ·
`sed -n '109,113p' internal/tg/client.go; sed -n '78,84p' internal/scheduler/worker.go` →
`if opts.Transport.RetryBaseDelay <= 0 {` and `if opts.Transport.RetryMaxDelay <= 0 {`, each
returning `optionErrorf(…, "must be positive, got %s", …)`, and a `positiveFields` list carrying
`{"RetryBaseDelay", opts.Config.RetryBaseDelay}` and `{"RetryMaxDelay", opts.Config.RetryMaxDelay}`
rejected by `if f.value <= 0`] — and for **every** `attempt` including arbitrarily large ones: the
result is strictly positive, never exceeds `ceiling`, and never decreases as `attempt` grows.
Subtask 1's table pins it at attempts far past any a naive shift survives, and what happens outside
that domain is decided below rather than left to the implementor.

**The contract quantifies over `base` too, because one adopter already depends on that half.**
`base > ceiling` is not a degenerate case a shared implementation may leave undefined here: a shipped
assertion in `internal/tg` pins exactly it, at the zeroth attempt, through the post-loop clamp
[measured df9b1a2:internal/tg/retry_test.go:724-725 · `sed -n '724,725p' internal/tg/retry_test.go` →
`if got := backoffDelay(40*time.Second, shippedMax, 0, func() float64 { return 0 }); got !=
shippedMax/2 {` with the failure message `post-loop clamp: base alone already exceeds maxDelay`], and
`internal/scheduler`'s ramp answers the same input at its own post-loop clamp
[measured df9b1a2:internal/scheduler/cadence.go:51-53 · `sed -n '51,53p'
internal/scheduler/cadence.go` → `if d > ceiling {` / `return ceiling`]. An `Exponential` that clamps
only *inside* the doubling loop returns `base` for `attempt = 0` and breaks that assertion the moment
`internal/tg` re-points at it. So the contract reads: for every `attempt` **and every `base > 0`**,
including a `base` already above `ceiling`, the result is strictly positive, never exceeds `ceiling`,
and never decreases as `attempt` grows — and subtask 1's table carries a `base > ceiling` row of its
own, so `internal/backoff`'s table covers the adopters' full input domain instead of leaning on
subtask 2 to catch a hole in it.

**Outside that domain the answer is decided here, not discovered at implementation.** The `base > 0`
bound is what makes "strictly positive" satisfiable at all, and it must not be read as licence to
clamp from below: neither replaced ramp has a lower clamp — every clamp in either body acts on a
value that has grown *above* its ceiling [measured c37e642:internal/tg/retry.go:30-44 and
internal/scheduler/cadence.go:40-55 · `sed -n '30,44p' internal/tg/retry.go; sed -n '40,55p'
internal/scheduler/cadence.go` → the whole of each function: `if d > maxDelay/2 { d = maxDelay }`
then `if d > maxDelay { d = maxDelay }`, and `if d >= ceiling { return ceiling }` then
`if d > ceiling { return ceiling }`, with no comparison in either body raising a value] — so an
implementor honouring an unbounded reading would add a behaviour neither shipped ramp has, inside a
change billed as behaviour-preserving and where no shipped assertion would catch it. The decided
answers, in this order: **`base <= 0` returns `base` unchanged at every attempt** — no doubling, no
lower clamp; **otherwise `ceiling <= 0` returns `ceiling`**. At `base = 0`, and at a non-positive
`ceiling`, that is what the replaced ramps already answer — `internal/tg`'s
in its equal-jitter half — while at a *negative* `base` they are deliberately not followed, because
what they do there is diverge and wrap
[measured c37e642 · a probe over verbatim copies of both shipped ramps · `go run .` → at `base = 0`
both answer `0s` at every attempt tried; at `base = 1s` with `ceiling = -1s` `internal/scheduler`'s
answers `-1s` and `internal/tg`'s answers `-500ms`; at `base = -1ns` both diverge negatively and wrap
through `-2562047h47m16.854775808s` to `0s`] — which is the very outcome this contract exists to
exclude. `EqualJitter` answers each row with its equal-jitter half of the same value. Neither input
is reachable from any adopter — the adopters' constructors above refuse a non-positive base or
ceiling, `internal/ingest`'s `New` refuses a non-positive tuning field (D15), and `internal/config`
refuses a non-positive duration for the keys that feed them
[measured c37e642:internal/config/transport.go:223-230 · `sed -n '223,230p'
internal/config/transport.go` → `func lookupPositiveDuration(lookup Lookup, key string)
(time.Duration, bool, error) {` rejecting `if err != nil || d <= 0` with `must be a positive
duration, got %q`] — so these rows pin a decision rather than a behaviour anyone can observe, and
subtask 1 carries them.

**Scope — settled by the owner, round 2.** `internal/backoff` ships **and** both `internal/tg` and
`internal/scheduler` adopt it. Adopting it in only one would leave the shared package standing
beside a surviving copy of the same ramp one directory away, which is the outcome
`design-writer` § Rules exists to prevent. This is no longer an open question.

**Per package, the decision is *delete and re-point*, never *keep a thin adapter*.** `AGENTS.md`
§ API Stability's table names a delegating wrapper as the thing to delete
[measured febae63:AGENTS.md:108 · `grep -n 'Keep \`func OldName' AGENTS.md` → `108:> | Keep \`func
OldName(...)\` delegating to \`NewName\` "for compat" | **DELETE** it — call sites update
directly |`], and an unexported one is
not exempt: there is no downstream client forcing either shape, so an adapter here would exist
solely so that call sites need not change. In `internal/scheduler` the adapter is not merely
disfavoured but unavailable under its present name — Go refuses a package-scope identifier that an
import of the same name also declares, and the refusal is package-wide rather than per-file
[measured febae63 · a probe module declaring `func sub()` in `a.go` and importing
`probe2/sub` in `b.go`, `go build ./...` → `./a.go:3:6: sub already declared through import of
package sub ("probe2/sub")` / `./b.go:3:8: other declaration of sub`], so keeping `func backoff`
would force either a rename or an import alias — cost with no benefit.

| package | deleted | re-pointed to |
|---|---|---|
| `internal/tg` | `backoffDelay` | `backoff.EqualJitter` (attempt argument first), at the production call site in `internal/tg/caller.go` and at the direct calls in `internal/tg/retry_test.go` [measured febae63 · `rg -U --type go -n 'backoffDelay\s*\(' internal/tg/` → the declaration in `internal/tg/retry.go`, call sites in `internal/tg/caller.go` and `internal/tg/retry_test.go`, plus failure-message mentions in the latter; no other file]. `defaultJitter` and the jitter-bounds contract stay in `internal/tg`. |
| `internal/scheduler` | `backoff` | `backoff.Exponential`, at the persisted-`run_at` computations in `internal/scheduler/settle.go` and at the direct calls in `internal/scheduler/cadence_test.go`, `internal/scheduler/failure_test.go` and `internal/scheduler/deadline_test.go` [measured febae63 · `rg -U --pcre2 --type go -n '(?<![A-Za-z_.])backoff\s*\(' internal/scheduler/` → the declaration in `internal/scheduler/cadence.go`, calls in `internal/scheduler/settle.go`, `internal/scheduler/cadence_test.go`, `internal/scheduler/failure_test.go` and `internal/scheduler/deadline_test.go`, plus comment and failure-message mentions in the last three; no other file]. The one-based→zero-based translation is written at the call site, where the `consecutive_failures` value it converts is visible [measured febae63:internal/scheduler/settle.go:142 · `sed -n '142p' internal/scheduler/settle.go` → `runAt := s.Add(backoff(k, cfg.RetryBaseDelay, cfg.RetryMaxDelay))`, with `k` the row's `consecutive_failures`]. **Per test file, the disposition is fixed, not left open.** `cadence_test.go`'s `TestBackoff_exactTable` and `TestBackoff_strictlyGrowingUntilCeiling` stay, with the same translation written into their *arguments* so their one-based `cases` table and every asserted bound stay byte-identical — see the disposition paragraph above. `failure_test.go` and `deadline_test.go` call no ramp at all afterwards: their `backoff(attempt, …)`-derived expected value is replaced by the literal one-based ramp of the call-site gate. |

The scan above is multiline-aware (`rg -U`) per `design-writer` § Rules, and it reaches three
classes the implementor must not conflate: **call expressions**, which re-point; **comment prose**
naming `backoff(k)`, which is re-worded to name the shared function; and **`t.Errorf` format
strings**, which are failure-message text, not assertions — re-wording them is free, and doing so
does not touch the expected values the gate below fixes.

**What "behaviour-preserving" can be gated on.** A delete-and-re-point does not leave the shipped
ramp tests *compiling* unchanged, so "they pass unchanged" is not an available gate and must not be
written as one. The first half of the gate is narrower and actually checkable: in
`internal/tg/retry_test.go`, `internal/scheduler/cadence_test.go`,
`internal/scheduler/failure_test.go` and `internal/scheduler/deadline_test.go`, **every
*pre-existing* assertion and every *pre-existing* expected value stays byte-identical; the call
expression is re-pointed, and its *argument* may carry the one-based→zero-based translation.**
The rule reads in that order and only in that order: *an argument may carry the translation; an
assertion or an expected value may not move.* A changed pre-existing expected value in any of them is
the signal that the adoption was not behaviour-preserving, and is a scope-boundary item for the
orchestrator rather than something the implementor absorbs. The tests this rule governs exist today
[measured 1fce8b5:internal/scheduler/cadence_test.go:8,33 and internal/tg/retry_test.go:127,682 ·
`grep -n 'func Test.*[Bb]ackoff' internal/scheduler/cadence_test.go internal/tg/retry_test.go` →
`TestRetry_JitterOptionThreadedThroughToBackoff`, `TestBackoffDelay_JitterBoundsExactly`,
`TestBackoff_exactTable`, `TestBackoff_strictlyGrowingUntilCeiling`].

**The argument half is not a loophole — it is mandatory at one site, and that site is a test.**
`cadence_test.go` pins the scheduler's ramp through a `cases` table whose key column is **one-based**
[measured 07e5177:internal/scheduler/cadence_test.go:12-24 · `sed -n '12,24p'
internal/scheduler/cadence_test.go` → `failures int` / `want time.Duration` over
`{1, time.Second}` … `{7, 30 * time.Second}`], so a re-point that leaves `tc.failures` standing in
front of a zero-based `Exponential` reds it — measured below. Keeping that table byte-identical
therefore *requires* translating the argument there, exactly as `settle.go` translates `k`. Nothing
about that buys a green run for a moved expectation: what may not move is the expected value, and the
site where a wrong argument still silently **passes** is the production call site, which is the
literal ramp's job further down.

**Disposition of `cadence_test.go`'s ramp tests, fixed here so the implementor does not have to
choose.** `TestBackoff_exactTable` and `TestBackoff_strictlyGrowingUntilCeiling` both survive the
adoption in place, re-pointed at `backoff.Exponential` with the translation written into the argument
— `tc.failures` in the table-driven one, the seed and the loop variable in the monotonicity one —
so the `cases` table, every expected duration and every asserted bound stay byte-identical. They are
neither deleted as superseded by subtask 1's table nor re-indexed in their data: what they pin after
the adoption is `internal/scheduler`'s **one-based** `failures → delay` mapping, cheaply and with no
database, and that mapping stays this package's meaning even though the arithmetic has moved one
directory away. What they do not pin — and never did — is which argument `settle.go` passes.

**That first half of the gate is necessary and NOT sufficient, and the hole sits exactly where the
translation is.**
`internal/scheduler`'s persisted-`run_at` sites are the ones needing the one-based→zero-based
translation, and the shipped assertions over a persisted retry `run_at` compute their expected value
through **the same function the call site uses**
[measured df9b1a2:internal/scheduler/failure_test.go:154 and internal/scheduler/deadline_test.go:252
· `rg -n --pcre2 '(?<![A-Za-z_.])backoff\s*\(' internal/scheduler/*_test.go` → `want :=
backoff(attempt, cfg.RetryBaseDelay, cfg.RetryMaxDelay)` at each, the test's `attempt` asserted equal
to the row's `consecutive_failures` on the preceding lines; every other test hit is `cadence_test.go`
pinning the function itself, or comment and failure-message prose]. What that suite can and cannot
see was measured on a scratch copy of this tree, and the shape of the hole is narrower — and worse —
than "the suite is blind".

**Measured, un-translated everywhere: the suite catches it, at the one test that pins the ramp
directly.** Re-pointing every `backoff(x, …)` to `backoff.Exponential(x, …)` with every argument left
standing reds `cadence_test.go`'s one-based `TestBackoff_exactTable`
[measured 07e5177 · a scratch copy of this tree with `internal/backoff.Exponential` added,
`internal/scheduler`'s `backoff` deleted and every call re-pointed with its argument untouched ·
`go test ./internal/scheduler/` → `--- FAIL: TestBackoff_exactTable` reporting
`backoff(1, 1s, 30s) = 2s, want 1s` through `backoff(5, 1s, 30s) = 30s, want 16s`, with
`TestBackoff_strictlyGrowingUntilCeiling`, `TestFailurePolicy_oneShotAttemptsGrowAndGiveUp` and
`TestDeadline_successiveBreaches_growingDelay` green]. An earlier revision of this design asserted
the opposite — that the natural re-point keeps every suite green — and ordered that sentence into
the implementor's prompt; it is false, and neither the gate below nor its spawn contract may be
justified by it.

**Measured, translated only where that red forces it: the suite goes green, and the persisted delay
doubles.** The only way to keep the `cases` table byte-identical is to translate the argument in
`cadence_test.go`. An implementor who does exactly that — the minimal fix for the only failure the
suite reports — and leaves the argument alone at `settle.go` and at both `want :=` lines gets a
wholly green package
[measured 07e5177 · the same scratch tree with `cadence_test.go`'s argument translated
(`Exponential(tc.failures-1, …)`, its table untouched) and `settle.go` plus both `want :=` lines left
zero-based · `go test ./internal/scheduler/` → `ok  	github.com/maratik123/lab-game/internal/scheduler`],
while every scheduler retry's persisted `run_at` doubles and the give-up cadence moves with it.

**Measured, translated correctly: the suite returns the same green.** With the translation written at
`settle.go` and at both of those lines, the package passes again
[measured 07e5177 · the same scratch tree with `Exponential(k-1, …)` at both `settle.go` sites and
`Exponential(attempt-1, …)` at both `want :=` lines · `go test ./internal/scheduler/` →
`ok  	github.com/maratik123/lab-game/internal/scheduler`]. So the shipped suite's verdict is
**invariant** to the call-site translation: the same green for the adoption that preserves every
persisted `run_at` and for the one that doubles it. That invariance — not blindness in general — is
the hole, and its cause is structural: the assertions that watch a persisted `run_at` derive their
expected value from the very function the call site calls, so both legs move together.
`internal/backoff`'s own table cannot close it either, since that table pins the shared function and
the defect is in the argument handed to it. `internal/tg` is not exposed at all — its expected values
are literals and its adoption needs no translation — so no amount of green from the other adopter
says anything about this one.

**So the second half of the gate pins the CALL SITE, independent of the shared function.** In
`internal/scheduler/failure_test.go`, which drives the one-shot settlement
[measured df9b1a2:internal/scheduler/settle.go:242 · `sed -n '242p' internal/scheduler/settle.go` →
`runAt := s.Add(backoff(k, cfg.RetryBaseDelay, cfg.RetryMaxDelay))`], and in
`internal/scheduler/deadline_test.go`, which drives the drain settlement at the identical expression
[measured df9b1a2:internal/scheduler/settle.go:142 · `sed -n '142p' internal/scheduler/settle.go` →
the same `s.Add(backoff(k, cfg.RetryBaseDelay, cfg.RetryMaxDelay))`], the persisted `run_at` bracket
is computed from a **literal one-based ramp written out in the test** — an indexed table of
durations keyed by the loop's `attempt`, with no call to `backoff`, to `backoff.Exponential`, or to
any shift or doubling expression inside it. Both tests already iterate attempts that sit strictly
below their ceiling under a `200ms` base
[measured df9b1a2:internal/scheduler/failure_test.go:83 and internal/scheduler/deadline_test.go:201 ·
`sed -n '83p' internal/scheduler/failure_test.go; sed -n '201p' internal/scheduler/deadline_test.go`
→ `cfg.RetryBaseDelay = 200 * time.Millisecond` in each probe config, against the `1s` and `500ms`
ceilings their base configs set], so the one-based ramp (`200ms`, `400ms`) and the zero-based one
(`400ms`, `800ms`-clamped-to-`500ms`) disagree at **every** attempt either test asserts: the omitted
translation reds both, by a readable factor rather than by a flake.

**That substitution is the sole carve-out from the expected-value half of the byte-identical rule,
and in substance it is not an exception to it.** The literal ramp reproduces the *same durations* the
shipped `backoff(attempt, …)` computes for those attempts — which is exactly what step (a)'s green run
against the still-shipped ramp proves before anything is re-pointed. What changes is where the
expected value comes from, never what it is; and after step (a) the literal ramp is itself a
pre-existing expected value that step (c)'s re-point must leave byte-identical. Nowhere else in
these files does new assertion text land.

**Ordering is part of the deliverable, and a mutation probe is what makes the instrument
trustworthy.** The literal ramp lands **first**, against the still-shipped one-based `backoff`, and
is green there; only then is the re-point applied. And because a green instrument is a claim about
the instrument until it has been seen to go red (`AGENTS.md` § Patterns 2), the implementor confirms
it discriminates before trusting it: with the literal ramp in place and the re-point not yet applied,
apply the *wrong* delay at both `settle.go` sites — written in the shipped ramp's own units as
`backoff(k+1, …)` — run `go test ./internal/scheduler/ -run
'TestFailurePolicy_oneShotAttemptsGrowAndGiveUp|TestDeadline_successiveBreaches_growingDelay'`,
observe **RED**, then restore.

Two properties of that expression are deliberate. It computes exactly what the omitted translation's
`Exponential(k, …)` would [measured 07e5177 · a probe comparing the shipped `backoff` with the
`Exponential` this design specifies · `go run .` → `Exponential(k,b,c) == backoff(k+1,b,c) over every
(base,ceiling,k) tried: mismatches=0`, over the base/ceiling pairs these tests and `cadence_test.go`
use and attempts past the ceiling]. And it **compiles**, which `Exponential(k, …)` at this point in
the sequence does not: while `func backoff` still stands in the package, no file of
`internal/scheduler` may import `internal/backoff` at all
[measured 07e5177 · a scratch copy of this tree with `internal/backoff` added and `settle.go`
importing it while `cadence.go`'s `backoff` still stands · `go build ./internal/scheduler/` →
`internal/scheduler/cadence.go:34:6: backoff already declared through import of package backoff
("github.com/maratik123/lab-game/internal/backoff")`] — the same package-wide collision D2's
delete-and-re-point table already turns on. The probe's RED is attributable to the literal ramp
rather than to a surviving `backoff(attempt, …)` expected value, because step (a) has already
replaced both `want :=` lines with it.
The restore is a `cp` backup of `settle.go` taken before the probe and copied back — never
`git checkout -- <file>`, which `AGENTS.md` § Workflow names as the command that silently discards
every other uncommitted edit to the file. A probe that comes back **green** means the gate is not the
gate this design asked for: that is a STOP for the orchestrator, not something to reason past.

`internal/backoff` carries its own exact table besides, in its own zero-based units, so the shared
function's contract is pinned independently of either adopter — but that table says nothing about
*which argument a call site passes*, which is the whole reason the literal ramp exists.

*Rejected:* a fourth private copy in `internal/ingest` (the rule's named anti-pattern); a
third-party backoff module (the stdlib plus this arithmetic is the whole requirement — `AGENTS.md`
§ Dependency Versions asks the comparison and there is nothing here a package would carry); a thin
unexported adapter in either package (it would shrink the diff and make the gate literal, at the
price of the shim `AGENTS.md` § API Stability tells you to delete — and the translation it would
hide, one-based `failures` against a zero-based attempt, is exactly what a reader of `settle.go`
should be able to see).

**D3 — `allowed_updates` is always transmitted, and the empty-route case rides a reserved sentinel
kind.** `telego.GetUpdatesParams.AllowedUpdates` carries `json:"allowed_updates,omitempty"`
[measured 1fce8b5:telego@v1.11.2/methods.go:28-35 · `sed -n '28,35p'
$(go env GOMODCACHE)/github.com/mymmrac/telego@v1.11.2/methods.go` → `AllowedUpdates []string
\`json:"allowed_updates,omitempty"\``] and this project marshals request parameters with
`encoding/json` [measured 1fce8b5:internal/tg/constructor.go:22-27 · `sed -n '22,27p'
internal/tg/constructor.go` → `body, err := json.Marshal(parameters)`]. `encoding/json` erases an
empty slice under `omitempty` — not only a nil one
[measured 1fce8b5 · `go doc encoding/json.Marshal` → `The "omitempty" option specifies that the
field should be omitted from the encoding if the field has an empty value, defined as false, 0, a
nil pointer, a nil interface value, and any array, slice, map, or string of length zero.`], so "send
an empty list" is unreachable through the generated params struct, and an absent parameter silently
inherits the server's previous setting (spec *Technical constraints* item 2). AC5 therefore forces a
**non-empty** list even with no route registered. The loop transmits, in that case, a one-element
list holding `telego.ShippingQueryUpdates` — the id space of an update this bot cannot receive,
because a shipping query arrives only for an invoice with a flexible price
[measured 1fce8b5:telego@v1.11.2/types.go:81 · `grep -n 'ShippingQuery - Optional'
$(go env GOMODCACHE)/github.com/mymmrac/telego@v1.11.2/types.go` → `81:	// ShippingQuery - Optional.
New incoming shipping query. Only for invoices with flexible price.`], and this project sends no
invoices [measured febae63 · `grep -rn 'invoice\|shipping' --include=*.go internal/ cmd/` → no
match (exit 1)]. The sentinel is present **only** when the route set is empty, so a kind with no route
still never arrives once any mechanic registers one (spec Scope 3). *Rejected:* refusing to start a
routeless loop (makes AC5 vacuous and diverges from `internal/scheduler`, which ships with an empty
registry); bypassing `telego.GetUpdatesParams` with a hand-built request (breaks AC3's
one-ingestion-path guarantee).

**D4 — the update's kind is a table, not reflection; the drift check is the test's job.** `Kind` is a
string type whose values are the Bot API's `allowed_updates` tokens, which `telego` already declares
as constants [measured 1fce8b5:telego@v1.11.2/methods.go:38-65 · `sed -n '38,65p'
$(go env GOMODCACHE)/github.com/mymmrac/telego@v1.11.2/methods.go` → the `// Update types you want
your bot to receive` const block, from `MessageUpdates = "message"` to
`ManagedBot = "managed_bot"`]. Production code carries one explicit table row per token: the kind,
a `present func(*telego.Update) bool` probe, and optional extractors for the update's own date and
for its destination chat id — supplied per **payload type** rather than per kind, so the `*telego.Message`
kinds share one date/chat helper, the `*telego.ChatMemberUpdated` kinds share another, and a kind
whose payload declares neither carries nil. Derivation is total: an update matching no row yields the
zero `Kind`, which is unrouted (AC9). `NewRouter` refuses a `Kind` with no table row, so a route can
never be registered into a hole. Drift against a `telego` bump is caught by a **test** that reflects
over `telego.Update`'s exported pointer fields and asserts each one's json tag is either a table row
or a named exemption — reflection in the test, explicitness in production. *Rejected:* reflection in
production (a clever derivation for a surface a reviewer must be able to read); a table covering only
the kinds today's mechanics need (a silent hole the moment #37 or #42 registers a kind nobody added).

**D5 — the offset advances inside the handler's own transaction.** The offset is a row (spec
Scope 4), and the advance for a successfully handled update is written in the same transaction as
that handler's effects, so "handled" and "confirmed" commit together and the crash window between
them is empty. The other settled outcomes advance in a transaction of their own: an unrouted update
(no handler ran), a give-up (the failed attempt's writes are already rolled back, and the give-up row
is written with the advance), and a duplicate — where the loop rolls the attempt back first, because
`ErrAlreadyPosted` means the effects already exist from the first delivery and any other write the
handler made before `Post` is a replayed effect, not a wanted one. `store.Post`'s own contract
guarantees the transaction is still usable in that case
[measured 1fce8b5:internal/store/errors.go:32-35 · `sed -n '32,35p' internal/store/errors.go` →
`ErrAlreadyPosted is returned when the batch's PlayerOperation basis replays a (source,
operation_id) pair already posted. The transaction is usable; nothing was written.`], which is what
makes the rollback a choice rather than a necessity. The advance statement is guarded and monotone —
it writes only when the new value exceeds the stored one — so a re-run can never move the offset
backwards. The value it writes is the settled update's `update_id + 1`, because that is what the
column holds and what the next poll transmits verbatim (D14). AC6, AC24, AC25.

**D6 — one attempt is one transaction; a retry is a new one.** The loop begins a transaction per
attempt, hands the `pgx.Tx` to the handler, and commits or rolls back itself — `store.Post`'s
division [measured 1fce8b5:internal/store/post.go:44-46 · `sed -n '44,46p' internal/store/post.go` →
`Post applies one balanced batch of postings under basis, inside the caller's transaction tx. The
caller owns the transaction: Post neither commits nor rolls back.`] and `internal/scheduler`'s. A
failed attempt is rolled back before the next begins (AC24), the delay between attempts is
`backoff.Exponential`, and the wait selects on `ctx.Done()` so a cancellation mid-retry leaves the
update unsettled with the offset behind it (AC25). No savepoint machinery: unlike the scheduler,
this loop has no settlement statement that must survive a handler's failed subtransaction, so
rollback-and-begin-again is both simpler and stronger.

**D7 — the handler runs inline, under `recover`.** No goroutine, therefore no leak to reason about
(AC26), and no per-update execution deadline — the spec asks for none, and the scheduler's
hijack-the-connection machinery exists for a lock-release problem this loop does not have. A
recovered panic rolls the attempt back and counts as a failure for the retry policy (spec Scope 9),
and is reported to the observer as its own outcome, distinct from a returned error (AC10). The
package's non-test source contains no `panic(`, `log.Fatal` or `os.Exit` (AC11); `recover` raises
none, so `ai-docs/panic-index.md` gains no row — it is empty today and this task keeps it so
[measured 1fce8b5:ai-docs/panic-index.md · `cat ai-docs/panic-index.md` → `**The project targets
zero production panics and currently holds it** — the table below is empty.`].

**D8 — the outcome classification is a sentinel test, and duplicates never retry.** A handler error
whose chain `errors.Is`-matches `store.ErrAlreadyPosted` settles the update as a duplicate: counted
as an idempotency hit, no attempt consumed, offset advanced (AC22). Every other error, and every
recovered panic, takes the retry path. The classification survives `%w` wrapping because it is
`errors.Is`, and the `errorlint` linter already forbids the `==` comparison that would not
[measured 1fce8b5:.golangci.yml · `grep -n 'errorlint' .golangci.yml` → `- errorlint          # %w
wrapping, errors.Is/As over == and type asserts`].

**D9 — the `operation_id` grammar is `<space>:<id>`, and its derivation is unexported.** `IDSpace`
is a string type with a member for the `update_id` sequence and one for the `callback_query.id`
sequence; the builder joins the space token, `:` and the raw identifier, refusing an empty space or
an empty id. `internal/store` treats `operation_id` as opaque
[measured 1fce8b5:internal/store/basis.go:28-34 · `sed -n '28,34p' internal/store/basis.go` →
`operationID is that client's idempotency key — store treats it as opaque`], which is what makes the
grammar this package's to define. The builder is **unexported**, and `ingest.Update` carries the
derived keys as fields — the canonical one in the `update_id` space, plus the callback-query one
where the update is a callback query. That is the structural form of spec Scope 6's third
requirement: a handler cannot assemble an `operation_id` from raw Telegram fields because it has no
function to assemble one with. AC13, AC14. A third id space costs a new member and a new field, never
a migration of stored rows.

**`ingest.Update` holds the raw `telego.Update` in a NAMED field, and a handler's context is its
first parameter.** Spec *Technical constraints* item 5 states the rule; what the dependency actually
does makes it a design obligation rather than a reminder to be careful. `telego.Update` carries an
unexported `context.Context` reachable through `Update.Context()` and `Update.WithContext()`
[measured 07e5177:telego@v1.11.2/types.go:126-129,163-165,171-177 ·
`sed -n '126,129p;163,165p;171,177p' $(go env GOMODCACHE)/github.com/mymmrac/telego@v1.11.2/types.go`
→ `ctx context.Context` under `// ctx - Internal context value can be retrieved using
[Update.Context] and set by [Update.WithContext]`; `// The returned context is always non-nil; it
defaults to the background context.`; and `// Warning: Panics if nil context passed.` over
`func (u Update) WithContext(ctx context.Context) Update {` / `if ctx == nil {` /
`panic("Telego: nil context not allowed")`]; and the generated `GetUpdates` this loop polls through
unmarshals its result and attaches no context to it
[measured 07e5177:telego@v1.11.2/methods.go:70-77 · `sed -n '70,77p'
$(go env GOMODCACHE)/github.com/mymmrac/telego@v1.11.2/methods.go` → `var updates []Update` /
`err := b.performRequest(ctx, "getUpdates", params, &updates)` / `return updates, nil`]. So the
context riding in every update this package sees is a `context.Background()`: a handler reading it
would hold a context with no cancellation and no deadline, silently escaping both the loop's `ctx`
and D6's per-attempt transaction — the failure would look like a handler that keeps working after the
loop was cancelled, which is D7's risk row seen from the other end. Therefore: the raw update is a
**named** field of `ingest.Update`, never embedded, because embedding would promote `Context()` and
`WithContext()` onto this package's own exported type; the `Handler`'s context is its first
parameter; the riding one is never read; and `WithContext` is never called — it panics on a nil
context, as the measurement above shows, and D7's zero-panic posture has no reason to invite that
`panic` into a handler's call path. Subtask 11's ctx walk covers each of those — the field's type,
the embedding, and the calls. AC2.

**D10 — the gate asks *who*, never *what kind of chat*.** `ingest.Gate` implements `tg.Gate`:
`ChatNone` is allowed, `ChatUnknown` is refused, and a `ChatKnown` destination is allowed when its
`Key` parses as an `int64` present in `config.Config.AllowedChatIDs`, or when an `owner` row exists
with `kind = 'player'` **and** that `telegram_id`. The predicate names the kind because the schema's
uniqueness is on the pair and a chat's own id lives in the same column
[measured 1fce8b5:internal/store/migrations/00001_ledger_core.sql:6-12 · `sed -n '6,12p'
internal/store/migrations/00001_ledger_core.sql` → `CREATE UNIQUE INDEX owner_kind_telegram_id_key
ON owner (kind, telegram_id) WHERE telegram_id IS NOT NULL`], so a lookup keyed on `telegram_id`
alone would match every chat the bot was ever added to (AC33). A token that parses as no integer —
a `@channelusername` — is refused with no special case, since it can be neither an `AllowedChatIDs`
member nor an `owner.telegram_id` (AC28). A lookup error refuses the call (AC34). The cache holds
**positive** results only, for the process's lifetime; nothing negative is remembered, so a player
who presses Start after a refusal is allowed on the next attempt with no restart (AC35).

**The cache's shape is a mutex-guarded `map[int64]struct{}`, not a bare map.** `AllowCall` sits on
the outbound path, which is concurrent by construction — the per-chat and class-global limiters
downstream of it exist precisely because concurrent senders do, and they hold their own state under
a single mutex [measured febae63:internal/tg/limit.go:202-207 · `sed -n '202,207p'
internal/tg/limit.go` → `type Limiter struct {` / `mu     sync.Mutex` / `global [3]*schedule` /
`chats  map[chatKey]*schedule`]. An unsynchronised map on that seam is a `concurrent map writes`
fatal error on the one component whose job is to stop a write to an unintended chat, and it is a
race `go test -race` is a required gate for (`AGENTS.md` § Go Test Conventions). The critical
section is a map lookup and, on a miss that the lookup confirms, one insert; the database call
itself happens **outside** the lock, so a slow lookup never serialises other destinations, at the
cost of a benign duplicate lookup on a first concurrent miss for the same id (the value written is
the same either way). *Rejected:* `sync.Map` — its wins are stable read-mostly keys under heavy
core-count contention, which this seam does not have, and it would trade the shipped limiter's
already-reviewed mutex idiom for a second concurrency vocabulary in the same call path.

**D11 — the owner read is `internal/store`'s, and it takes a queryer, not a transaction.** The gate
runs on the outbound path, where there is no transaction to borrow, so the new read is
`store.PlayerExists(ctx, q, telegramID)` over a consumer-declared `store.Queryer` — the single
`QueryRow(ctx, sql, args ...any) pgx.Row` method that both `pgx.Tx`
[measured 1fce8b5:pgx/v5@v5.10.0/tx.go:147 · `grep -n 'QueryRow(ctx context.Context'
$(go env GOMODCACHE)/github.com/jackc/pgx/v5@v5.10.0/tx.go` → `QueryRow(ctx context.Context, sql
string, args ...any) Row`] and `*pgxpool.Pool`
[measured 1fce8b5:pgx/v5@v5.10.0/pgxpool/pool.go:774 · same command over `pgxpool/pool.go` →
`func (p *Pool) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row`] already satisfy.
This keeps `owner`'s schema knowledge in the package that owns the table and leaves `Post`'s path
untouched (AC21). `ingest.PlayerLookup` is the consumer-declared interface the gate depends on, with
a pool-backed implementation in this package, so the gate is testable against a fake and against a
real schema without either being a mock of this package's own making (AC29). **What that pool handle
can and cannot see is D18's subject**, and the answer there is a designed property of the handler
contract rather than a limitation of this decision.

**`Queryer` has a shipped near-duplicate inside `internal/store`, and subtask 5 re-points it rather
than adding a second spelling.** The package's own test file already declares an unexported
interface of exactly this shape for exactly this reason
[measured c221784:internal/store/post_test.go:72-75 · `sed -n '72,75p' internal/store/post_test.go` →
`// queryRower is satisfied by both pgx.Tx and *pgxpool.Pool.` / `type queryRower interface {` /
`QueryRow(ctx context.Context, sql string, args ...any) pgx.Row`], and it is consumed by the
`balanceOf` helper in the same file. Exporting `Queryer` beside it and leaving `queryRower` standing
would put two names for one method set in one package — the shape `design-writer` § Rules refuses at
the package level and there is no reason to accept at the file level. Subtask 5 deletes `queryRower`
and re-points `balanceOf` at `store.Queryer`; `AGENTS.md` § API Stability's delete-the-wrapper rule
applies unchanged to an unexported test-local one.

**Why two interfaces over one read, stated so a later reader does not collapse them.** They narrow
at two different seams and each is declared by its own consumer, which is this project's rule for
interfaces (`AGENTS.md` § API Naming). `store.Queryer` narrows what `PlayerExists` needs from its
*handle* — one `QueryRow` method — so the same read serves a `pgx.Tx` on a handler's path and a
`*pgxpool.Pool` on the gate's without `internal/store` naming a pool type in a signature.
`ingest.PlayerLookup` narrows what the *gate* needs from the read — one boolean answer — which is
what lets the cache scenarios substitute a stub carrying a call counter and assert that a second
`AllowCall` for a cached id issues no second lookup (AC35). Collapsing them costs one of those
two properties: giving the gate a `Queryer` puts a fake of `internal/store`'s row-scanning
semantics inside `internal/ingest` (a mock of another package's read, and no place to count calls),
while giving `internal/store` a `PlayerLookup` puts the gate's caching vocabulary into the table's
owning package.

**D12 — the observation is one per attempt plus one per attempt-less settlement.** §13.2 wants
handler duration, handler errors *and* panics, and idempotency hits on the health surface
(`docs/DESIGN.md` §13.2).
A single terminal observation per update would erase a panic that a later attempt recovered from, so
the loop reports one `Observation` per handler call, carrying the kind, the attempt number, the
outcome, the call's duration and the lag; unrouted and give-up each report one of their own. A
`LoopObservation` per cycle carries the cycle's duration, the batch size and any poll error (AC17).
The package declares the interface and imports no metrics library; a nil observer is checked, not
called (AC19). Exposition stays #23's.

**D13 — lag is reported only where the update carries its own date.** `Observation` pairs a
`time.Duration` with a boolean saying whether it is meaningful; a kind whose payload declares no date
sets the boolean false rather than a zero duration, because a zero lag is indistinguishable from a
healthy poll and §13.2 makes lag the dashboard's star metric (AC18). `telego.CallbackQuery` declares
no date field [measured 1fce8b5:telego@v1.11.2/types.go:3729-3754 · `sed -n '3729,3754p'
$(go env GOMODCACHE)/github.com/mymmrac/telego@v1.11.2/types.go` → the struct's fields are `ID`,
`From`, `Message`, `InlineMessageID`, `ChatInstance`, `Data`, `GameShortName`], while
`telego.Message` and `telego.ChatMemberUpdated` each declare a Unix `Date`
[measured 1fce8b5:telego@v1.11.2/types.go:619-620,3958-3959 · `sed -n '619,620p;3958,3959p'
$(go env GOMODCACHE)/github.com/mymmrac/telego@v1.11.2/types.go` → `// Date - Date the message was
sent in Unix time. …` / `Date int64` and `// Date - Date the change was done in Unix time` /
`Date int64`]. The extractor lives in D4's table.

**D14 — the schema is one forward migration — the offset's table and the give-up table — and the
offset is a guarded singleton.** `internal/store/migrations/00004_ingest.sql`, goose, no `-- +goose Down` section, in
the discipline `store.Migrate` already applies
[measured 1fce8b5:internal/store/migrate.go:19-23 · `sed -n '19,23p' internal/store/migrate.go` →
`Migrate applies every pending migration in internal/store/migrations to pool's database,
forward-only (no -- +goose Down section exists in this package).`]. `ingest_offset` is a single row
pinned by `CHECK (id = 1)` and seeded by the migration itself, and **its column holds the offset to
transmit, not the last update seen — the name says which.** `GetUpdatesParams.Offset` is defined as
one greater than the highest `update_id` already received
[measured df9b1a2:telego@v1.11.2/methods.go:11-18 · `sed -n '11,18p'
$(go env GOMODCACHE)/github.com/mymmrac/telego@v1.11.2/methods.go` → `Offset - Optional. Identifier
of the first update to be returned. Must be greater by one than the highest among the identifiers of
previously received updates. … An update is considered confirmed as soon as getUpdates … is called
with an offset higher than its update_id.`], so the column is `next_update_id`: settling
`update_id = n` writes `n + 1`, and the loop transmits the stored value **verbatim** as
`GetUpdatesParams.Offset`, with the `+ 1` living in exactly one statement — the advance — and nowhere
on the poll path. The alternative shape, storing `n` and adding one at transmission, is **refused**,
and not on taste: combined with D5's guarded-monotone advance it is a permanent **live-lock**. The
same update is re-fetched every cycle, its handler's `Post` returns `ErrAlreadyPosted`, the loop
settles it as a duplicate and writes `n` again, the guard rejects the write because `n` does not
exceed `n`, and the offset never moves — and the only symptom is the idempotency-hit counter §13.2
puts on the health surface, where duplicates are ordinary traffic and only a *spike* reads as a
signal (`docs/DESIGN.md` §13.2), and which nothing exposes at all until #23 lands (spec § Out of
scope). One semantic, named in the column, pinned by subtask 9's transmitted-offset scenario. The seeded value is
`0`, which the Bot API reads as "the earliest unconfirmed update" — the first-start behaviour
§ Risks already states. `ingest_dead_update` carries the
update's identity, its kind, a nullable chat id, the attempt count and the last error — the shape
`scheduler.DeadTask` already has [measured 1fce8b5:internal/scheduler/task.go:78-90 · `sed -n
'78,90p' internal/scheduler/task.go` → `DeadTask is one give-up row … Type / InstanceKey / RunAt /
ConsecutiveFailures / LastError`] — and **not** the raw update payload. Table names are singular per
KD-17. **Forward-only, and its rollback:** both tables are created by this migration, so no row
predates it and no code reads a second shape during a deploy window; undoing it is a later forward
migration that drops them, at the cost of the offset (a fresh deploy then resumes from Telegram's
earliest unconfirmed update).

**Granularity — settled by the owner, round 2.** The offset row stays the singleton this decision
specifies. Widening it to a per-bot key is a later forward migration adding a column and relaxing
the `CHECK`; this task takes the MVP's one-bot-per-database shape rather than a speculative key.
This is no longer an open question.

**Both of `ingest_dead_update`'s identifying columns are §12.5 sanitisation targets — the chat id
*and* `last_error`.** The chat id column is the obvious one. `last_error` is free text produced by
whatever the handler returned, so it can embed a real chat or user id that a column-targeted rewrite
would walk straight past, and §12.5's whole obligation is that a restored snapshot cannot address a
real person. The only read this design gives the column is `DeadUpdates`' projection (AC37), which
carries it to an operator and branches on nothing in it, so the column is diagnostic data at rest
and sanitisation is free to **blank** it rather than rewrite ids inside it — cheaper and safer than
a regex over arbitrary error prose `[derived → AC36, AC37, and the `DeadUpdates` scenarios in
§ Test Design subtask 7]`. **`ingest_offset` is a §12.5 target too, and it is the one that bullet already names.** §12.5's
sanitisation transaction is required to «сбросить updates offset», and until this task there was no
table for that clause to refer to — after it, `ingest_offset` is that table by name, and its reset is
a `UPDATE ingest_offset SET …` on the singleton row rather than a rewrite of ids. So subtask 12
records **three** obligations against the same bullet, not one: reset `ingest_offset`, rewrite
`ingest_dead_update`'s chat id, blank `ingest_dead_update.last_error`. The design states this
so §12.5's script author does not have to infer it, and subtask 12 writes the same obligation into
`ai-docs/domain-invariants.md` beside the existing sanitisation bullet
[measured febae63:ai-docs/domain-invariants.md:115 · `sed -n '115p' ai-docs/domain-invariants.md` →
`A production snapshot reaches testing only through sanitisation — rewrite chat/user ids, drain the
outbound notification queue, reset the updates offset.`].

**D15 — the tuning keys join KD-27's optional-with-default class.** `internal/config` gains an
`Ingest` struct and a `LAB_GAME_INGEST_` reader, mirroring `loadScheduler` exactly
[measured 1fce8b5:internal/config/scheduler.go:15-20 · `sed -n '15,20p' internal/config/scheduler.go`
→ the `LAB_GAME_SCHEDULER_POLL_INTERVAL` … `LAB_GAME_SCHEDULER_TASK_TIMEOUT` constants]: the keys are
appended by `EnvKeys()`, never by the unexported `envKeys()` the required-variable suites iterate.
The keys are the poll interval, the long-poll timeout, the batch limit, the retry attempt cap, the
retry base delay, the retry maximum delay and — added by the round-6 amendment (D20), after this
decision's first implementation had already shipped — the retry growth factor. Each key's *role* is fixed by a decision rather than
left to the implementor: the poll interval is the wait between cycles and, per D19, the whole bound
on the retry rate against a failing Bot API; the long-poll timeout is D16's; the retry family is
D6's, consumed through `backoff.Exponential`, with the growth factor's own domain rule — finite and
strictly above `1`, refused at start-up otherwise — fixed by D20 rather than here; the batch limit
is `GetUpdatesParams.Limit`. Their defaults are chosen operational tuning, never
balance numbers: poll interval `1s`, long-poll timeout `25s`, batch limit `100`, attempt cap `5`,
retry base delay `1s`, retry maximum delay `8s`, retry growth factor `backoff.DefaultFactor` (D20's
`2`, so the ramp this paragraph's stall arithmetic assumes is the shipped default). The retry values are picked together: under that
cap and that ramp at its default factor (D20) the worst-case head-of-line stall a poisoned update
imposes on the sequential loop is `1s + 2s + 4s + 8s`, a quarter-minute, which is the term spec Scope 7 says the cap exists to
bound. The batch limit is validated against the
Bot API's stated range [measured 1fce8b5:telego@v1.11.2/methods.go:19-22 · `sed -n '19,22p'
$(go env GOMODCACHE)/github.com/mymmrac/telego@v1.11.2/methods.go` → `// Limit - Optional. Limits the
number of updates to be retrieved. Values between 1-100 are accepted.` above
`Limit int \`json:"limit,omitempty"\``]. `.env.example` gains a line
per key with its default as the value, because the disjointness test asserts set equality between
that file, `EnvKeys()` and the keys the loader consults (KD-23).

**D16 — the long-poll timeout is cross-checked against `AttemptTimeout` in `config.Load`.**
`Transport.AttemptTimeout` bounds every attempt including the long poll
[measured 1fce8b5:internal/tg/caller.go:170-176 · `sed -n '170,176p' internal/tg/caller.go` →
`attemptCtx, cancel = context.WithTimeout(ctx, to)` under `if to := c.client.transport.AttemptTimeout;
to > 0`], and it defaults to 30s [measured 1fce8b5:internal/config/transport.go:123-128 · `sed -n
'123,128p' internal/config/transport.go` → `AttemptTimeout: 30 * time.Second` among
`defaultTransport`'s fields]. A long-poll timeout at or above it turns every poll that runs its full
window — every poll with no update to return, which on an idle bot is all of them — into a cancelled
attempt. `Load` holds both structs, so the check lands there and reports a `*KeyError` naming
`LAB_GAME_INGEST_LONG_POLL_TIMEOUT` (AC27).

**The whole-seconds requirement is a second `config.Load` check on the same key, not a loop-side
truncation.** The Bot API parameter is an integer count of seconds
[measured c221784:telego@v1.11.2/methods.go:24-26 · `sed -n '24,26p'
$(go env GOMODCACHE)/github.com/mymmrac/telego@v1.11.2/methods.go` → `// Timeout - Optional. Timeout
in seconds for long polling. Defaults to 0, i.e. usual short polling. Should be positive …` above
`Timeout int \`json:"timeout,omitempty"\``], so a duration that is not a whole number of seconds
cannot be transmitted as configured. The loop must therefore never be handed one: a value such as
`25500ms` is rejected by `Load` with a `*KeyError` naming `LAB_GAME_INGEST_LONG_POLL_TIMEOUT`,
exactly as the `AttemptTimeout` cross-check on the same key does, and both checks live beside each
other. *Rejected:* letting the loop truncate to whole seconds — it would silently run a different
timeout than the operator configured, and it would loosen the margin the strict
`LongPollTimeout < AttemptTimeout` inequality was computed against, since the check would then be
comparing a value the loop does not use. One key, one owner, one error shape. The loop transmits the
timeout explicitly on every poll, so no poll degrades into short polling.

**The check is a strict inequality by design; no slack term is enforced, and the margin ships in the
defaults.** `LongPollTimeout < AttemptTimeout` admits a pair as tight as `25s` against `26s`,
leaving a second for the whole round trip — deliberately. The slack an operator actually needs is a
property of their network path to their `telegram-bot-api` instance, not a number this package can
pick, and a minimum enforced here would reject a legitimately tuned pair on a fast local hop while
still being too small on a slow one. What the design *does* commit to is that the shipped defaults
carry a real margin — `25s` against `AttemptTimeout`'s `30s` — and that both values are separately
tunable and both appear in `.env.example`, so the margin is visible rather than implicit. Only the
degenerate case, where every poll that runs its full window is guaranteed to be cancelled, is a
start-up error.

**D17 — the composition order the gate implies, stated once.** `Options.Gate` is set at `tg.New` and
never afterwards [measured 1fce8b5:internal/tg/client.go:28-31,127-135 · `sed -n '28,31p;127,135p'
internal/tg/client.go` → `Gate, when non-nil, is consulted before every outbound call` and
`c := &Client{… gate: opts.Gate, …}`], and the gate needs only the allowlist and a pool. So the
wiring order is gate, then client, then loop — no cycle. `cmd/bot` stays a scaffold this task does
not wire (spec § Out of scope); the order is recorded so the first mechanic's composition root does
not rediscover it.

**D18 — an outbound call that only a row of the handler's own uncommitted transaction would permit is
not issued from inside the handler. The gate refusing it is correct behaviour, not a defect to work
around.** D11 gives the gate a `*pgxpool.Pool`, so `PlayerExists` runs on a connection of its own,
and a row the loop's still-open transaction has written is not visible on it. §1's onboarding is
exactly the shape that meets this — button in the chat → the player presses Start → the player's
`owner` row is created → the bot writes to the player's DM, whose chat id is in no
`ALLOWED_CHAT_IDS` list, since §1 also puts every game command in DM (`docs/DESIGN.md` §1; the spec
quotes both clauses at Scope 8). Written naively —
create the row, then DM before returning — the gate refuses that DM on the attempt and identically on
every retry (each retry is a fresh transaction, D6, so the row is no more visible), and the update
ends in `ingest_dead_update`. This design states the constraint rather than leaving #30 to discover
it. Nothing in the project overrides the transaction isolation level: no production `Begin` in the
tree passes `pgx.TxOptions` [measured c37e642 · `rg -U --type go -n '\.Begin(Tx)?\(' internal/ cmd/`
restricted to non-test files → every production hit is a plain `conn.Begin(ctx)` with no options
argument; `rg -U --type go -n 'TxOptions' internal/ cmd/` → no match (exit 1)], so the visibility
rule the gate lives under is the server's default one, and subtask 10's uncommitted-owner-row
scenario is what pins it against a real Postgres
`[derived → subtask 10's uncommitted-owner-row scenario]`.

**The rule belongs on the `Handler` contract, and it is not a gate workaround.** A handler MUST NOT
issue an outbound Bot API call whose permission rests on a row its own uncommitted transaction
created. The reason survives even if the gate were made transaction-aware: a Telegram send is not
rollback-able, so a message justified by a row the transaction then rolls back has already reached a
real person. That is the dual-write hazard the outbound notification queue exists to remove
(`docs/DESIGN.md` §13.2, which puts that queue on the health surface beside the update metrics this
task ships). **The designed route is that queue (#43)**: the handler enqueues a
row inside its own transaction, and the send happens after the commit, at which point `PlayerExists`
sees the `owner` row on any connection. #43 is out of this task's scope (spec § Out of scope), so
until it lands a mechanic needing a post-commit send performs it after the loop has committed,
outside the handler — and this design adds no speculative after-commit hook for a handler that does
not exist yet.

**Why not the obvious alternative — making the gate transaction-aware.** Giving the gate a way to
consult the in-flight transaction means smuggling a `pgx.Tx` into `tg.Gate.AllowCall`'s `ctx`, because `Call` carries only
the method, its class and its destination [measured c221784:internal/tg/gate.go:53-63 · `sed -n
'53,63p' internal/tg/gate.go` → `type Call struct {` with `Method string`, `Class MethodClass`,
`Chat ChatRef`]. That puts a transaction in a context value on the outbound seam, makes
`internal/tg`'s gate contract transaction-aware in a package the spec keeps free of the database
(spec Scope 8's fourth property), and buys a *worse* answer: a permission derived from a row that may
still roll back. Refusing is the direction the spec already fixed for this gate — it fails closed.

Three surfaces carry the obligation so it is met rather than rediscovered: the `Handler` doc comment
states it beside the ctx-propagation obligation `scheduler.Handler`'s already carries
[measured c221784:internal/scheduler/task.go:92-101 · `sed -n '92,101p' internal/scheduler/task.go` →
`A Handler MUST propagate the ctx it is handed to every call it makes on tx`] (subtask 6); subtask
10's Test Design pins the behaviour with a negative scenario; and subtask 12 writes it into
`ai-docs/domain-invariants.md` beside the allowlist bullets.

**D19 — `Run` observes a poll failure and keeps polling; only cancellation ends it.** A `getUpdates`
failure is ordinary traffic against a self-hosted `telegram-bot-api` instance — one 502 must not stop
all ingestion, and `Run` returning it would demand an external restart for a transient fault. So:
`PollOnce` returns the poll error to its caller **and** reports it through `LoopObservation.Err`
(D12); `Run` discards `PollOnce`'s return, having already observed it, and continues. `Run` returns
non-nil only on cancellation, and returns `ctx.Err()` (AC26). This is `internal/scheduler`'s shipped
worker shape, transposed without variation
[measured c221784:internal/scheduler/worker.go:145-161 · `sed -n '145,161p'
internal/scheduler/worker.go` → `ticker := time.NewTicker(w.cfg.PollInterval)` then a loop whose body
is a `ctx.Done()` check returning `ctx.Err()`, `_ = w.RunOnce(ctx)`, and a `select` on `ctx.Done()`
returning `ctx.Err()` or on `<-ticker.C`], and its doc comment already argues the same point for the
same reason [measured c221784:internal/scheduler/worker.go:136-139 · `sed -n '136,139p'
internal/scheduler/worker.go` → `Run does not stop on a RunOnce error — each cycle already reports it
through ObserveLoop (design D12) — because a worker that stopped on a transient discovery failure
would need an external restart for no reason.`].

**The poll interval is the whole retry-rate bound, and no consecutive-failure ramp is added.** The
loop waits `Ingest.PollInterval` between cycles — after a successful cycle and after a failed one
alike — so a failing Bot API is polled once per interval rather than in a tight loop, which is the
busy-loop an observe-and-`continue` with no wait would produce. A second, failure-counting ramp on top would be a third backoff
policy in a package that already sits behind two: `internal/tg` retries a retryable `getUpdates`
attempt up to `RetryMaxAttempts`, waiting `EqualJitter` between attempts, before returning anything
to its caller [measured c221784:internal/tg/caller.go:122-126 · `sed -n '122,126p'
internal/tg/caller.go` → `if !out.retryable || attempts >= c.client.transport.RetryMaxAttempts {`
returning a give-up error, so the loop continues otherwise], so a *retryable* poll error reaching
`Run` is one the transport layer has already backed off over — and a non-retryable one, which that
same branch returns without waiting, is bounded by the poll interval like every other cycle. Adding
a ramp here would compound two delays for one fault and give the loop a state variable that survives
across cycles, which `internal/scheduler` also
declined. If measured behaviour later shows the flat interval is too aggressive against a sustained
outage, that is a tuning change to `LAB_GAME_INGEST_POLL_INTERVAL`, not a new mechanism.

An offset consequence, stated because it is what makes the policy safe: a failed poll returns no
updates, so nothing is settled and the offset does not move (D5). A cycle that could not reach the
server therefore leaves the loop exactly where it was, and the next cycle re-asks for the same
`offset`.

**D20 — the ramp's growth factor is configuration, its default is `2`, and the clamp moves from
"before the doubling" to "before the conversion". (Round-6 amendment; every clause below is the
owner's decision, recorded, not reopened.)** The `2` in `Exponential`'s loop is a tuning value with
semantic meaning compiled into Go source
[measured fc5e6dd:internal/backoff/backoff.go:50-55 · `sed -n '50,55p' internal/backoff/backoff.go`
→ `d := base` / `for range attempt {` / `if d >= ceiling {` / `return ceiling` / `}` / `d *= 2`],
which `AGENTS.md` § Code Style names as belonging in configuration rather than in a `.go` file. The
owner asked for `1.3` on the transport's ramp and, once the blast radius was surfaced, fixed the
decision this section implements: **the value becomes configurable, the compiled-in default stays
`2`, and the literal is lifted into a named constant**; `1.3` becomes an operator's choice through
configuration, never a code change.

Two consequences are load-bearing and are stated before anything else, because everything rounds 4
and 5 verified rests on them. *The default is unchanged*, so every pinned ramp value in
`internal/backoff/backoff_test.go`, `internal/tg/retry_test.go`, `internal/scheduler/cadence_test.go`,
`internal/scheduler/failure_test.go` and `internal/scheduler/deadline_test.go` stays valid, and D2's
behaviour-preserving adoption still holds. *The literal one-based ramps stay literal and stay at the
default*, so D2's call-site gate keeps discriminating a one-based/zero-based mistranslation at
`settle.go` exactly as it does today
[measured fc5e6dd:internal/scheduler/failure_test.go:165-168 and
internal/scheduler/deadline_test.go:261-264 · `sed -n '165,168p' internal/scheduler/failure_test.go;
sed -n '261,264p' internal/scheduler/deadline_test.go` → `literalOneBasedRamp := map[int]time.Duration{`
/ `1: 200 * time.Millisecond,` / `2: 400 * time.Millisecond,` / `}` at each]. No expected value moves
anywhere in this amendment, and any that does is the same scope-boundary item D2 already defines.

**Where the configurable value lives: one key per adopter, not one shared key.** The keys are
`LAB_GAME_TG_RETRY_FACTOR`, `LAB_GAME_SCHEDULER_RETRY_FACTOR` and `LAB_GAME_INGEST_RETRY_FACTOR`,
each joining the retry family its scope already owns beside `…_RETRY_BASE_DELAY` and
`…_RETRY_MAX_DELAY` [measured fc5e6dd:internal/config/transport.go:19-20,
internal/config/scheduler.go:18-19 and internal/config/ingest.go:18-19 ·
`grep -n 'RETRY_BASE_DELAY\|RETRY_MAX_DELAY' internal/config/*.go | grep -v _test` → the constant
declarations `envTGRetryBaseDelay   = "LAB_GAME_TG_RETRY_BASE_DELAY"`, `envSchedulerRetryBaseDelay  =
"LAB_GAME_SCHEDULER_RETRY_BASE_DELAY"` and `envIngestRetryBaseDelay   =
"LAB_GAME_INGEST_RETRY_BASE_DELAY"`, each with its `_MAX_DELAY` neighbour on the following line].
The argument is not symmetry for its own sake: the three ramps measure different things and are
already tuned apart. The transport's ramp is the wait between attempts of one outbound Bot API call,
defaulting from `500ms` to a `30s` cap; the scheduler's is a task-retry cadence defaulting from `1s`
to `5m`; the ingestion loop's is the head-of-line stall a poisoned update imposes on a sequential
loop, whose default cap and ramp were picked together to bound that stall
[measured fc5e6dd:internal/config/transport.go:126-127, internal/config/scheduler.go:78-79 and
internal/config/ingest.go:82-84 · `sed -n '126,127p' internal/config/transport.go; sed -n '78,79p'
internal/config/scheduler.go; sed -n '82,84p' internal/config/ingest.go` → `RetryBaseDelay: 500 *
time.Millisecond,` / `RetryMaxDelay: 30 * time.Second,`; `RetryBaseDelay: time.Second,` /
`RetryMaxDelay: 5 * time.Minute,`; `RetryMaxAttempts: 5,` / `RetryBaseDelay: time.Second,` /
`RetryMaxDelay: 8 * time.Second,`]. The motivating case the owner named — a gentler `1.3` against a
rate limiter and a flaky network — is the **transport's**; a single shared key would move the
scheduler's persisted retry cadence and the ingestion loop's stall budget with it, silently, and
neither is what the operator asked to change. There is also no home for a shared key that does not
invent one: `internal/config` groups tuning by consumer — `Transport`, `Scheduler`, `Ingest` — and a
cross-cutting scalar would need a fourth grouping, plus a route into three option structs that today
receive exactly one config struct each. *Rejected:* one `LAB_GAME_RETRY_FACTOR` for the module (it
couples three unrelated ramps and contradicts KD-27's per-scope class); a key on the shared package's
own package-level variable (mutable global state, and untestable in parallel).

**What it is called, and why not "base".** In this package `base` already names the ramp's *base
delay*, a `time.Duration`, and it is the first duration parameter of the very function gaining the new
argument. The exponent's base is therefore called the **factor** — `factor float64` as a parameter,
`RetryFactor float64` as a config field, `…_RETRY_FACTOR` as the key — so no reader and no call site
has to disambiguate two things called "base" in one signature. The owner's Russian phrasing
(«основание степени») is preserved in meaning, not transliterated into a collision.

**The signature: the factor joins the ramp-shape parameters; `EqualJitter` keeps its jitter last.**
`Exponential(attempt int, base, ceiling time.Duration, factor float64)` and
`EqualJitter(attempt int, base, ceiling time.Duration, factor float64, jitter func() float64)`. The
factor sits immediately after `ceiling` because it describes the ramp's shape alongside base and
ceiling, while `attempt` is the variable and `jitter` is the callback this project's convention keeps
last. Every existing call therefore keeps its prefix and gains one argument, which is the smallest
diff that can express the change, and every parameter is type-distinct, so a positional mix-up is a
compile error rather than a wrong delay. *Rejected:* a `backoff.Ramp{Base, Ceiling, Factor}` value
with `Delay`/`EqualJitter` methods — it is the tidier API in isolation, but it rewrites every shipped
call site and every direct test call for no behaviour, it creates a second place where the three
tuning values live beside the `config.*` structs that already hold them, and KD-31 deliberately made
this package stateless arithmetic with nothing to construct; *rejected:* keeping `Exponential` at its
current signature beside a new factor-taking entry point — that is precisely the two-APIs-side-by-side
row `AGENTS.md` § API Stability tells you to delete, and this module has no downstream client that
could justify it; *rejected:* a `backoff.Factor` newtype whose constructor refuses an illegal value —
attractive, but the project's precedent for a validated scalar is a plain field checked in the
consumer's constructor, which is how every `RetryBaseDelay` in the tree is already handled, and the
newtype would put a conversion at every literal in every test config; *rejected:* a
`shopspring/decimal` factor for exactness (`internal/config` already imports that module
[measured fc5e6dd · `go list -f '{{join .Imports " "}}' ./internal/config` → `errors fmt
github.com/shopspring/decimal go.yaml.in/yaml/v3 net/url os strconv strings time`]) — the arithmetic
this feeds is `math.Pow` on `float64`, the value is operational tuning (KD-27's class) rather than a
balance number (KD-24's), and exactness buys nothing for a delay truncated to a nanosecond.

**The overflow contract, restated: the clamp is the last thing before the conversion, and nothing
out of range is ever converted.** A non-integer factor cannot be applied as `d *= factor` on a
`time.Duration`, so the multiplication becomes float64 arithmetic and the shipped guarantee — "the
ceiling is tested and clamped BEFORE the doubling that would overflow it, so no attempt, however
large, can wrap `time.Duration`'s underlying int64 into a negative value"
[measured fc5e6dd:internal/backoff/backoff.go:13-16 · `sed -n '13,16p' internal/backoff/backoff.go` →
`exceeds ceiling, and never decreases as attempt grows. The ceiling` / `is tested and clamped BEFORE
the doubling that would overflow it,` / `so no attempt, however large, can wrap time.Duration's
underlying` / `int64 into a negative value.`] — has to be re-anchored rather than dropped. It is
re-anchored, not weakened:

- the factor is **normalised before it is used**: anything not strictly greater than `1` — a value
  below `1`, exactly `1`, or a `NaN` — becomes exactly `1`. That normalisation is what answers the
  contract table's `factor <= 1, or NaN` row below, and it runs **after** the out-of-domain `base`
  and `ceiling` returns and **before** the multiplication. The whole order, stated once so the two
  clauses below cannot be read as deciding the same input twice: clamp a negative `attempt` to the
  zeroth; return the out-of-domain `base` and `ceiling` rows unchanged; normalise the factor;
  multiply; clamp; convert;
- the delay is computed as `float64(base) * math.Pow(factor, float64(attempt))`, in float64, where
  overflow **saturates to `+Inf`** instead of wrapping;
- **no `time.Duration` conversion happens until the value has been proven strictly below `ceiling`.**
  Anything not strictly below it — a finite value at or above the ceiling, `+Inf`, or a `NaN` —
  returns `ceiling` itself, which is already a `time.Duration` and is returned unconverted;
- therefore the only value ever converted lies in `[base, ceiling)`, inside `int64`'s range by
  construction, and the guarantee reads unchanged in substance: **no attempt, however large,
  produces a negative or wrapped delay.**

The trap is real but sits one step later than the premise that raised it. `math.Pow(1.3, 1000)` is
**not** `+Inf` — it is a finite `8.7771254729739e+113`; what overflows is the product and, decisively,
the conversion: `time.Duration` of an out-of-range float64 and of a `NaN` both yield `int64`'s minimum
on this toolchain, i.e. a negative delay, silently
[measured fc5e6dd · a probe module under `tmp/` · `go run .` → `math.Pow(1.3, 1000) =
8.7771254729739e+113`, `float64(base)*Pow: 1s*Pow(1.3,1000) = 8.7771254729739e+122`,
`time.Duration(math.Pow(1.3,1000)) = -2562047h47m16.854775808s` and `time.Duration(math.NaN()) =
-2562047h47m16.854775808s`]. The guard above is exactly the guard for that, and it is written as an
explicit `math.IsNaN` test rather than as a negated `<` comparison, so that no boolean-simplification
rewrite — by a linter, or by a later reader tidying the expression — can silently drop the `NaN` case.

With the normalisation step in front of it, a `NaN` *product* is unreachable rather than merely
unlikely: `base` is positive by the guard above it, and a normalised factor is either finite and
above `1` or exactly `1`, so the multiplication yields a finite value or `+Inf` and never a `NaN`
[measured 515f03d · a probe module under `tmp/` carrying the float form and the normalisation order
this section specifies · `go run .` → `NaN product after normalisation, over the swept grid:
unreachable`]. The `math.IsNaN` test on the *product* is therefore the defensive second gate, not the
clause that decides a `NaN` **factor** — the normalisation is, and it is what makes a `NaN` factor
answer `base` rather than `ceiling`
[measured 515f03d · the same probe · `go run .` → `NaN factor, base=1s ceiling=8s, attempts 0..3:
[1s 1s 1s 1s]` and `NaN factor at MaxInt attempt: 1s`].

**`math.Pow` rather than a loop, and the amendment is strictly stronger than what it replaces.** A
loop that multiplies by a float factor `attempt` times keeps the shipped shape but loses its
termination argument: the shipped loop leaves early only when the running value reaches the ceiling,
which integer doubling reaches quickly for every ceiling this tree configures, while a factor near
`1` needs unboundedly many iterations — and the shipped loop already fails that way where the ceiling
is out of reach, returning
a **negative** delay at one attempt and `0` past it rather than the ceiling
[measured fc5e6dd · the same probe, over a verbatim copy of the shipped `Exponential` and the float
form specified here · `go run .` → `ceiling=MaxInt64 base=1ns attempt=63: shipped=-2562047h47m16.854775808s
proposed=2562047h47m16.854775807s`, `attempt=64: shipped=0s proposed=2562047h47m16.854775807s`, and the
same at attempts 65 and 100]. `math.Pow` is O(1) at every attempt and returns the ceiling in each of
those rows. Nothing in the tree configures a ceiling near `int64`'s maximum and no default
approaches one — but nothing refuses one either, since the readers refuse only a non-positive
duration, so the hole is closed here rather than inherited. It is recorded because KD-31 states the
no-wrap guarantee unconditionally, and the amendment must make that statement more true, not less.

**Behaviour preservation at the default is measured, not argued.** Over a grid of bases from `1ns` to
`5m`, ceilings from `1ns` to `1h` and attempts from `-1000` through `1000`, the float form at
`factor = 2` and the shipped integer form return the **same value in every row**
[measured fc5e6dd · the same probe · `go run .` → `factor=2 grid: mismatches=0`]. The agreement is
exact rather than approximate because a power of two is exactly representable and a `time.Duration`
below `2^53` nanoseconds converts to float64 without loss; above that the operands themselves round,
and the two forms diverge by a few nanoseconds at most
[measured fc5e6dd · the same probe over bases at and above `2^53ns` · `go run .` → `worst absolute
deviation=128ns … at attempt=7 base=9007199254740993 ceiling=2305843009213693952`, against
`2^53 ns = 2501h59m59.254740992s`]. A base above `2^53` nanoseconds is a retry delay past a hundred
days, which no default in this tree approaches; and even there the clamp holds, so the result stays
positive and bounded by the ceiling.

**AC40 is therefore discharged over bases below `2^53` nanoseconds, and the divergence above that
bound is a recorded exception rather than an undiscovered one.** AC40 quantifies over *every* attempt
of *every* adopter
[measured c222623:ai-docs/plans/2026-09-06-update-ingestion-dispatch-idempotency.spec.md:444 ·
`sed -n '444p' ai-docs/plans/2026-09-06-update-ingestion-dispatch-idempotency.spec.md` → `With none
of the three variables set, every retry delay every adopter produces, at every attempt, equals the
delay that adopter produced before this change.`], and the measurement above is narrower than that
quantifier: it is exact over the
grid it swept, and above `2^53` nanoseconds the two forms are near-equal rather than equal. The gap
is not closed by widening the sweep, because it is a property of float64 — a `time.Duration` at or
above `2^53` nanoseconds does not survive the round trip through a float64 mantissa, so no
implementation of this amendment can make the two forms bit-identical there. It is stated instead,
because such a base is **configuration-reachable**: the duration readers refuse only a non-positive
value
[measured 515f03d:internal/config/transport.go:223-233 · `sed -n '223,233p'
internal/config/transport.go` → `func lookupPositiveDuration(lookup Lookup, key string)
(time.Duration, bool, error) {` … `if err != nil || d <= 0 {`], so an operator who configures a
retry base past a hundred days gets a delay differing from the shipped ramp's by the recorded amount
above — bounded, positive, still clamped by the ceiling, and past every operational magnitude this
tree ships a default for.

What this costs the implementor is one bound, stated so a red is never misread: **every
behaviour-preservation row subtask 15 writes sits at a base below `2^53` nanoseconds**, which is the
range the exact-equality reading of AC40 holds over. A red in that range is a defect in the
amendment. A row *above* that bound is not a tolerance to widen and not a defect to chase — it is
outside what this amendment can promise, and belongs to the orchestrator as a scope-boundary item,
by the same rule D2 already applies to a moved expected value.

**A factor at or below `1` is a configuration error, refused at start-up — the owner's rule,
implemented where he put it.** A configured `LAB_GAME_*_RETRY_FACTOR` that does not parse, or that parses to a value not
strictly greater than `1`, or that is not finite, fails `config.Load` with a `*KeyError` naming that
variable, in the same shape as every other malformed tuning value
[measured fc5e6dd:internal/config/transport.go:223-233 · `sed -n '223,233p' internal/config/transport.go`
→ `func lookupPositiveDuration(lookup Lookup, key string) (time.Duration, bool, error) {` … rejecting
with `keyErrorf(key, ErrInvalidValue, "must be a positive duration, got %q", val)`]. The non-finite
half is not decoration: `strconv.ParseFloat` accepts `"NaN"`, `"Inf"` and `"+inf"` as valid input
[measured fc5e6dd · the same probe · `go run .` → `ParseFloat("NaN") = NaN, err=<nil>`,
`ParseFloat("Inf") = +Inf, err=<nil>`, `ParseFloat("+inf") = +Inf, err=<nil>`], so a reader that only
checked `> 1` would admit `+Inf` and a reader that only checked "parses" would admit `NaN`. That
two-clause shape is now the spec's, not this design's prose alone: the round-8 amendment records the
refusal as **two** criteria — AC42 for the `> 1` half and AC43 for the finiteness-and-parse half —
and re-derived the underlying fact independently, adding `"infinity"` to the strings `ParseFloat`
accepts and pinning that `+Inf > 1` is true while `NaN > 1` is false
[measured ef69c56:ai-docs/plans/2026-09-06-update-ingestion-dispatch-idempotency.spec.md:391-399 ·
`sed -n '391,399p' ai-docs/plans/2026-09-06-update-ingestion-dispatch-idempotency.spec.md` → the item headed *A float that
parses is not thereby a legal factor — which is why the refusal has two clauses*, whose body reads
that `strconv.ParseFloat` returns a nil error for the four strings above and that `+Inf` compares
strictly greater than `1` while `NaN` compares greater than nothing at all, closing with `AC42 and
AC43 are two criteria for that reason, not one restated twice.`]. Both readers and every constructor therefore
test the same two clauses, and `"infinity"` joins the invalid-value rows § Test Design lists.
`internal/config` has no float reader today to inherit the shape from
[measured fc5e6dd · `grep -rn 'ParseFloat' internal/config/` → no match (exit 1)], so row 14 adds one
beside `lookupPositiveInt` and `lookupPositiveDuration`, with the same `(value, present, error)`
signature and the same unconditional-query rule the disjointness test depends on.

**Because the boundary is asked in four packages — the call-site count `design-writer` § Rules asks
a shared-package decision to record — it has exactly one definition.** The legal-factor test is
needed by `internal/config`'s reader and by `internal/tg`'s, `internal/scheduler`'s and
`internal/ingest`'s constructors, each of which reports it in its own error shape — a `*KeyError`
there, an `*OptionError` or `optionErrorf` here
[measured fc5e6dd:internal/scheduler/worker.go:71-85, internal/tg/client.go:109-117 and
internal/ingest/loop.go:89-114 · `sed -n '71,85p' internal/scheduler/worker.go; sed -n '109,117p'
internal/tg/client.go; sed -n '89,114p' internal/ingest/loop.go` → a `positiveFields` list of
`time.Duration`-typed rows rejected by `if f.value <= 0`; an `if opts.Transport.RetryBaseDelay <= 0`
chain returning `optionErrorf(…, "must be positive, got %s", …)`; and `positiveDurations` /
`positiveInts` lists rejected by `if f.value <= 0`]. That is past the `≥ 3` threshold
`design-writer` § Rules sets for lifting a shared constant or helper, so `internal/backoff` — the
package that defines what a factor *means* — exports both **`DefaultFactor`**, the named constant the
literal `2` is lifted into and the value each adopter's config default falls back to, and
**`ValidFactor(factor float64) bool`**, true exactly for a finite factor strictly greater than `1`.
Each caller keeps its own error text and its own error type; only the predicate and the default are
shared. This gives `internal/config` its first import of another package in this module
[measured fc5e6dd · the same `go list` above → its import list carries no
`github.com/maratik123/lab-game/...` entry], which is safe in the one direction that matters:
`internal/backoff` imports `time` and (after this amendment) `math` and nothing else
[measured fc5e6dd · `go list -f '{{join .Imports " "}}' ./internal/backoff` → `time`], so no cycle is
reachable. The alternative — `internal/config` owning the boundary — was rejected because it puts the
domain rule of a function in a package that never calls it, leaving a future adopter reading
`internal/backoff`'s own doc comment with no way to check the rule it states.

**The contract rows, decided here so no implementor guesses.** Over every attempt (negative ones
clamped to the zeroth) the package answers:

| input | answer |
|---|---|
| `base > 0`, `ceiling > 0`, `factor > 1` | strictly positive, never above `ceiling`, and **never decreasing as `attempt` grows** — D2's clause, unchanged, and still the universal one. Growth is not promised at every step: the product is truncated to a whole nanosecond, so consecutive attempts return the **same** delay wherever one step of the ramp falls below that resolution, while a step of a nanosecond or more always advances the result. Once the product reaches `ceiling` the answer is exactly `ceiling` from there on |
| `base > ceiling`, both positive | `ceiling`, exactly, at every attempt — unchanged from D2 |
| `base <= 0` | `base`, unchanged, at every attempt — unchanged from D2 |
| `base > 0`, `ceiling <= 0` | `ceiling`, unchanged, at every attempt — unchanged from D2 |
| `factor <= 1`, or `NaN` | treated as exactly `1`: the result is `base` at every attempt, clamped by `ceiling` — no growth, and, as everywhere else in this package, **no lower clamp** |
| `factor = +Inf` | `base` at the zeroth attempt and `ceiling` from the first on — the clamp, not a special case |

The last two rows are new, and unreachable through configuration since every reader refuses their
inputs at start-up; they exist because this package is total by design and a future adopter must not
have to guess. The three middle rows are D2's, and they are where the amendment could have changed an
answer by accident — it does not: each of them comes back unchanged at every factor tried, `NaN` and
`+Inf` included [measured fc5e6dd · the same probe · `go run .` → for each of `factor=1`, `1.3`, `2`, `0.5`,
`NaN`, `+Inf`: `base=0 -> 0s ; base=-1ns -> -1ns ; ceiling=-1s -> -1s ; ceiling=0 -> 0s`]. The
non-decrease and ceiling clauses were swept the same way across factors from `1` to `+Inf` over a run
of attempts with no violation found [measured fc5e6dd · the same probe · `go run .` →
`monotonicity/positivity/ceiling sweep done (violations printed above; none printed = none found)`
over factors `1`, `1+2^-52`, `1.3`, `1.5`, `2`, `3`, `10`, `+Inf`].

**Why that first row promises non-decrease and not strict increase — measured at the edges of the
admitted input set, not at the defaults.** An earlier draft of this amendment wrote the row as
*strictly increasing until the ceiling*, on the reasoning that the owner's `> 1` start-up rule buys
growth the doubling-only version could not state. It does not, and the refutation is inside the
inputs the readers admit rather than outside them. The float→`time.Duration` conversion truncates to
a whole nanosecond, so a ramp whose step is smaller than that stands still: at `base = 500ms`,
`ceiling = 30s` and a `factor` of the smallest float64 above `1` — the transport's own shipped
defaults, and a factor `ValidFactor` accepts by definition — the ramp returns `500ms` unchanged from
the zeroth attempt through a millionth, and at `base = 1ns` with `factor = 1.3` it returns
`1ns, 1ns, 1ns, 2ns, 2ns, 3ns` before it starts moving every step
[measured 515f03d · a probe module under `tmp/` carrying the float form and the normalisation order
specified above · `go run .` → `base=500ms ceiling=30s factor=1+2^-52 attempts 0..9: [500ms 500ms
500ms 500ms 500ms 500ms 500ms 500ms 500ms 500ms]`, `1000000->500ms`, and `base=1ns ceiling=30s
factor=1.3 attempts 0..9: [1ns 1ns 1ns 2ns 2ns 3ns 4ns 6ns 8ns 10ns]`]. Both bases reach
configuration: the duration readers refuse only a non-positive value
[measured 515f03d:internal/config/transport.go:223-233 · `sed -n '223,233p'
internal/config/transport.go` → `func lookupPositiveDuration(lookup Lookup, key string)
(time.Duration, bool, error) {` … `if err != nil || d <= 0 {`], and `ValidFactor` refuses only a
factor that is not finite and above `1`.

What is true, and what this design states instead, is D2's clause plus one sufficient condition:
the sequence never decreases, and where one step of the ramp is at least a nanosecond the next
attempt's delay is strictly larger. Both were swept over bases from `1ns` to `1h`, ceilings from
`1ns` to `math.MaxInt64`, factors from the smallest float64 above `1` through `math.MaxFloat64` and
`+Inf`, and a run of attempts from the zeroth
[measured 515f03d · the same probe · `go run .` → `non-decrease/positivity/ceiling sweep over the
admitted domain: no violation found`, `float step >= 1ns => strict increase: no counterexample
found`, and `float step < 1ns: rows that returned the SAME delay at the next attempt exist`]. The
ceiling is not out of reach at that flattest edge either — it is simply reached at attempt indices no
retry loop visits [measured 515f03d · the same probe · `go run .` → at `base=500ms ceiling=30s
factor=1+2^-52`, `MaxInt attempt: 30s`]. Nothing downstream wanted the stronger reading: AC23 asks
for a delay that does not shrink, which is exactly the clause kept.

**The amendment has its own call-site invariance hole, and it gets its own gate.** KD-31's recorded
lesson is that a shipped suite can be invariant to what a call site passes; this amendment reopens
exactly that shape one argument to the right. An implementor who threads `backoff.DefaultFactor`
into the production call sites instead of the adopter's configured `RetryFactor` breaks the whole
feature and **every test stays green**, because every test runs at the default. The byte-identical
ramp values cannot see it, `internal/backoff`'s own table cannot see it, and D2's literal one-based
ramps cannot see it either — they too run at the default. So a scenario driven at a **non-default**
factor is added, asserting the delay the production path actually produces: `internal/tg`'s retry
wait, `internal/scheduler`'s persisted `run_at`, and `internal/ingest`'s between-attempt delay.
§ Test Design fixes the shape; the point is structural — the only assertion that can distinguish
"reads the configured field" from "reads the default" is one taken at a value that is not the
default.

**The unit that gets a scenario is the production call site, not the adopter package.** The
distinction is load-bearing, because a factor is threaded per call expression and getting it wrong
reopens the hole one function to the side. `internal/scheduler` has **two** such call expressions,
in two separate production functions, each reading `cfg` for itself and each writing a persisted
`run_at`: the deferred drain settlement and the inline one-shot settlement
[measured ef69c56:internal/scheduler/settle.go:133,143,222,243 ·
`grep -n 'backoff\.Exponential\|^func ' internal/scheduler/settle.go` → the call expression
`runAt := s.Add(backoff.Exponential(k-1, cfg.RetryBaseDelay, cfg.RetryMaxDelay))` at `143`, inside
`func deferredFailedStatement(p pendingSettlement, s time.Time, cfg config.Scheduler) (string,
[]any)` declared at `133`, and the identical expression at `243`, inside
`func settleFailed(ctx context.Context, tx pgx.Tx, task Task, decl Declaration, handlerErr error,
cfg config.Scheduler) error` declared at `222`]. D2 already built its call-site gate as **two**
instruments for
exactly that reason, and each instrument's own comment names the site it watches
[measured ef69c56:internal/scheduler/failure_test.go:155-164 and
internal/scheduler/deadline_test.go:253-260 · `sed -n '155,164p' internal/scheduler/failure_test.go;
sed -n '253,260p' internal/scheduler/deadline_test.go` → `// omitted one-based-to-zero-based
translation at settle.go's own` / `// call site: cadence_test.go's table alone cannot, because it`
at the first, and `// translation at settle.go's own drain-settlement call site.` at the second].
One scenario per *adopter* would therefore leave an implementor free to thread `cfg.RetryFactor` at
one of the two and `backoff.DefaultFactor` at the other with the whole suite green — both literal
ramps run at the default and cannot see it, and neither can anything else in the tree, for the
reason this section opened with. Row 15's floor-enumerated site list does not reach it either, and
the reason is worth stating so it is not mistaken for a gap in the enumeration: that list already
names both `settle.go` computations, so the *call sites* are enumerated correctly; what a scan of
names cannot decide is **which value each one passes**, and that is precisely what a scenario at a
non-default factor decides. The scheduler therefore gains **two** non-default-factor scenarios, one
per settlement path; `internal/tg` and `internal/ingest` have one production call expression each —
the transport's retry wait and the loop's between-attempt delay — and gain one apiece
[measured ef69c56:internal/tg/caller.go:138 and internal/ingest/attempt.go:35 ·
`grep -rn 'backoff\.Exponential\|backoff\.EqualJitter' --include='*.go' internal/ | grep -v
_test.go` → `wait = backoff.EqualJitter(attempts-1, c.client.transport.RetryBaseDelay,
c.client.transport.RetryMaxDelay, c.client.jitter)` and `delay := backoff.Exponential(attempt,
l.cfg.RetryBaseDelay, l.cfg.RetryMaxDelay)`, alongside the two `settle.go` rows above and no other
non-test call]. The rule is therefore one scenario per production call site, never one per adopter
`[derived → AC41]`.

**Exactness, and where an epsilon is allowed — the owner's second point, bounded.** Exact equality
stays the rule wherever the arithmetic is exact, and that is nearly everywhere: every row at
`factor = 2` (measured identical to the shipped integer form above), every clamp row (the ceiling is
returned unconverted), every out-of-domain row (an input returned unchanged), the negative-attempt
row, `DefaultFactor`'s value, `ValidFactor`'s booleans, `internal/tg`'s pinned equal-jitter values,
`cadence_test.go`'s one-based table, and both literal one-based ramps. A tolerance is admitted for
one thing only: an expected *value* at a non-power-of-two factor, where `float64(base) *
math.Pow(factor, attempt)` is inexact by construction and Go specifies no accuracy bound for
`math.Pow`. The tolerance is `max(1ns, 1e-9 × want)` and its bound is what matters: one step of the
ramp is `(factor − 1) × want`, which at the smallest factor these tests use (`1.3`) is `0.3 × want` —
eight orders of magnitude above the tolerance — while the noise the tolerance absorbs is a
last-place rounding plus the at-most-one-nanosecond truncation of the conversion, which on this
toolchain is currently zero at operationally realistic magnitudes
[measured fc5e6dd · the same probe against exact rational arithmetic in Python's `fractions` ·
`go run .` then a `Fraction(13,10)**k` comparison → `factor 1.3: worst relative deviation from
exact-then-truncate = 0` over attempts `0..20` at `base = 1s`, and `factor 1.5: worst relative
deviation = 0` at `base = 200ms`]. **A tolerance at or above one step is forbidden**: it would swallow
an off-by-one in the attempt index, which is the single defect class D2's whole gate exists to catch.
The same rule binds any use of `math.Pow` inside a test: it may compute a *reference ratio* between
adjacent attempts, and it may not compute the expected value of a row whose job is to pin the
function's own formula — a test that recomputes the implementation's formula pins nothing.

**What this amendment does not touch, said explicitly.** D2's scope (both adopters re-pointed, the
old functions deleted), its per-file dispositions, the literal one-based ramp gate and its mutation
probe, D14's singleton `ingest_offset`, and every expected value rounds 4 and 5 verified. The
amendment adds an argument to a call and a key to a config family; it moves no assertion. It adds no
panicking call either — the failure mode it guards against is a silent negative, not a crash — so
`ai-docs/panic-index.md` keeps the empty table it holds today
[measured fc5e6dd:ai-docs/panic-index.md · `cat ai-docs/panic-index.md` → `**The project targets zero
production panics and currently holds it** — the table below is empty.`].

**What the propagation subtask owes the decision record.** `ai-docs/key-decisions.md` **KD-31**
asserts the doubling as a property in four places a reader would rely on: the `min(base·2^i, ceiling)`
shape, the `Exponential(attempt, base, ceiling)` and `EqualJitter(attempt, base, ceiling, jitter)`
signatures, the phrase "clamps at the ceiling *before* the doubling", and the `base << attempt` hazard
statement [measured fc5e6dd:ai-docs/key-decisions.md:83 · `grep -n 'KD-31' ai-docs/key-decisions.md`
→ the entry carrying `min(base·2^i, ceiling)`, `Exponential(attempt, base, ceiling)` and `clamps at
the ceiling *before* the doubling`]. Row 16 amends it in place in the shape KD-12 already uses for a
later concretisation — a marked amendment clause inside the entry, not a rewritten history — recording
the configurable factor, its default, the new signatures, the clamp's move to the conversion boundary,
the `> 1` start-up rule, and the one thing the amendment makes *more* true rather than merely
different — the no-wrap guarantee now holds at a ceiling the shipped loop could not reach, where that
loop returned a negative delay at one attempt and `0` past it (measured above). KD-31's **growth**
wording is not strengthened and must not be rewritten as though it were: the amended entry says
*never decreases*, the same clause the shipped package states, for the reason the contract table's
first row now records.

**KD-27** enumerates each scope's optional-with-default key set by name and by count, so a new key
per scope falsifies its enumerations
[measured fc5e6dd:ai-docs/key-decisions.md:73 · `grep -n 'KD-27' ai-docs/key-decisions.md` → the entry
naming the `LAB_GAME_TG_*`, `LAB_GAME_SCHEDULER_*` and `LAB_GAME_INGEST_*` sets with a spelled-out
count for each]. Row 16 **updates** that entry's enumeration, spelled-out count included:
`ai-docs/key-decisions.md` is a decision record, the enumerated set is part of what the decision
recorded, and `/task` Step 9.5's no-tally rule does not reach this file: it names `context-status.md`
and `context.md` and stops there, as the paragraph below measures. The same falsification, and the
same update, reach the declaration-order counts in the three `*EnvKeys()` doc comments
[measured fc5e6dd:internal/config/scheduler.go:23, internal/config/transport.go:40 and
internal/config/ingest.go:22 · `grep -n 'returns the .* tuning variables' internal/config/*.go` →
`returns the six scheduler tuning variables`, `returns the thirteen transport tuning variables`,
`returns the six ingest tuning variables`], which row 14 owns because they sit in the files it edits.
Three claims of the same class sit in code rather than in the decision record, and belong on this
checklist for the same reason: the `RetryMaxDelay` doc comment on `config.Transport`,
`config.Scheduler` and `config.Ingest` each says the ceiling caps the backoff scale's *doubling*,
which stops being true the moment a factor other than `2` is configurable
[measured 515f03d:internal/config/transport.go:105, internal/config/scheduler.go:59 and
internal/config/ingest.go:67 · `grep -rn 'doubling' internal/config/*.go` → `// RetryMaxDelay caps
the backoff scale's doubling` at each]. Row 14 owns the edit, because those three files are already
its own; the checklist names them so the propagation stays a checklist rather than a re-derivation.

**`ai-docs/context.md` and `ai-docs/context-status.md` carry the same falsified claim and take the
opposite repair, and the difference is a rule rather than a preference: there the tally is
*removed*, never raised.** Both spell a tally for the `LAB_GAME_INGEST_*` key family that row 14
falsifies — `context.md`'s inventory sentence spells the scheduler's and the ingestion loop's
[measured fc5e6dd:ai-docs/context.md:43 ·
`grep -o 'its six \`LAB_GAME_[A-Z]*_\*\` tuning keys' ai-docs/context.md` → ``its six
`LAB_GAME_SCHEDULER_*` tuning keys`` and ``its six `LAB_GAME_INGEST_*` tuning keys``], and
`context-status.md`'s *What landed* bullet for **this PR's own entry** spells the ingestion loop's
again
[measured c222623:ai-docs/context-status.md:147 ·
`grep -n 'the six \`LAB_GAME_INGEST_\*\` tuning keys' ai-docs/context-status.md` → one hit inside
the entry headed `## Update ingestion — long polling, dispatch, operation idempotency, the chat
allowlist (PR #66, 2026-09-07)`, reading ``config.Ingest` and the six `LAB_GAME_INGEST_*` tuning
keys``]. For these two files, and only these two, `/task`'s own Step 9.5 forbids a count outright —
a test count, a file count, a package tally, an "N sites" figure — and gives the repair as naming
the things instead
[measured c222623:.claude/skills/task/SKILL.md:178 and .claude/skills/task/reference.md:141 ·
`grep -n 'No counts here' .claude/skills/task/SKILL.md .claude/skills/task/reference.md` → **No
counts here — name the things, do not tally them.** at each, the `reference.md` copy adding that
three figures written into this file were all three later found false]. So the edit row 16 makes to
each is to strike the quantifier — leaving the `LAB_GAME_SCHEDULER_*` and `LAB_GAME_INGEST_*` key
families named and uncounted — not to write the amendment's new number, which would re-commit the
same falsehood at the next key this project adds. That is the whole of row 16's business in
`context-status.md`: one clause of one sentence, in the entry this task itself wrote.

**The same class lives in the packages row 15 rewrites, and the checklist is a class with a
decision rule, not a closed list.** The three `config.*` comments above are named because row 14
already owns those files; the identical claim — the ramp *doubles*, and it doubles *in a loop* — is
asserted inside `internal/backoff` and `internal/tg`, where the `math.Pow` rewrite falsifies it most
directly. The rule that decides a site, so a later reader need not re-derive it: a sentence
describing the **function's contract** or the **loop's structure** goes stale the moment the factor
is configurable and the loop is gone, and is re-worded; a sentence describing what a specific
**default-factor row** computes stays true and stays put, because that row still runs at `2`. Under
that rule the sites are `Exponential`'s **own exported doc comment**, which states the shape and the
clamp's position — the *package* comment above it is already row 15's business, this line is not
[measured ef69c56:internal/backoff/backoff.go:36-39 · `sed -n '36,39p' internal/backoff/backoff.go`
→ `// Exponential returns the delay before an attempt-th (zero-based) retry:` /
`// min(base*2^attempt, ceiling), clamped before the doubling that would` /
`// overflow time.Duration's underlying int64. See the package comment for`];
`internal/backoff/backoff_test.go`'s comment naming a naive `base<<attempt`, and its
"no doubling, no lower clamp" texts — one a comment, one a `t.Errorf` string
[measured ef69c56:internal/backoff/backoff_test.go:89,124,142 ·
`grep -n 'doubling\|base<<' internal/backoff/backoff_test.go` →
`89:// exists for: attempts far past any a naive base<<attempt survives (64,`,
`124:// unchanged (no doubling, no lower clamp), and a positive base with a`, and the `t.Errorf`
format string at `142` carrying `want %v unchanged (no doubling, no lower clamp)`]; and
`internal/tg/retry_test.go`'s `t.Errorf` texts narrating the loop's exit branches, together with the
R1-8 comment block above them that attributes each branch to a **line range of a loop this rewrite
deletes**, and the further `t.Errorf` below them naming the post-loop clamp again
[measured ef69c56:internal/tg/retry_test.go:709,712,715-722,729 ·
`sed -n '709p;712p;715,722p;729p' internal/tg/retry_test.go` → `(post-loop clamp: 500ms*2^6=32s >
max, d exits the loop at 32s and is clamped after, half=15s)`, `(in-loop early break: d reaches >=
maxDelay inside the loop and returns immediately)`, the comment block reading `// round 1 finding
R1-8): attempt=6 returns via the POST-LOOP clamp` / `// (line 57-59)` … `// attempt=10 returns via
the IN-LOOP early return (line 52-54)`, and `(post-loop clamp: base alone already exceeds
maxDelay)`]. The same file's `// formula predicts (base/2, doubling)` is the counter-example the rule
above sorts the other way: it describes what that test's own default-factor rows compute, so it stays
[measured ef69c56:internal/tg/retry_test.go:126 · `sed -n '126p' internal/tg/retry_test.go` →
`// formula predicts (base/2, doubling), the shape the design's own Test`]. `make verify` sees none
of this — a stale comment compiles, and a stale `t.Errorf` string never prints on a green run — so,
exactly as with subtask 16, the check is a reader's, and the
recurrence this guards against is on record: self-review R1-8 already cost a round for false
branch-attribution prose in `internal/tg/retry_test.go` itself.

`docs/DESIGN.md` needs nothing, and is not this task's to edit in any case: its transport clause names
exponential backoff without fixing a factor for it
[measured fc5e6dd:docs/DESIGN.md:303 · `grep -n 'backoff' docs/DESIGN.md` → one hit, reading
`Транспорт с первого дня: честная обработка 429 (уважать retry_after, не долбить), exponential backoff
на сетевых ошибках`].

### Rejected alternatives, at the level of the whole approach

- **A separate "seen updates" table.** Refused by §11 and by the spec's Key decisions; idempotency is
  the basis document's unique constraint or it is nothing.
- **Concurrent dispatch.** The MVP runs one friendly chat, per-chat ordering is free in a sequential
  loop, and the update-lag metric this task ships is the evidence that would justify changing it
  (spec § Deferred).
- **A per-update execution deadline modelled on the scheduler's.** The scheduler's hijack machinery
  exists to release a locked `scheduled_task` row; nothing here holds a row another worker waits on,
  and a second poller is unrepresentable (spec *Technical constraints* item 4).

## Decomposition

| # | Task | Files | Depends on |
|---|------|-------|------------|
| 1 | New shared package `internal/backoff`: `Exponential` and `EqualJitter`, package comment, exported doc comments, table + monotonicity tests | `internal/backoff/backoff.go`, `internal/backoff/backoff_test.go` | — |
| 2 | Adopt `internal/backoff` in the two shipped packages per D2, in this order. **(a) First, the call-site gate**, against the still-shipped one-based `backoff`: in `internal/scheduler/failure_test.go` and `internal/scheduler/deadline_test.go`, replace the `backoff(attempt, …)`-derived expected `run_at` with a **literal one-based ramp table** that calls no ramp function, and confirm it is green — the green run is what proves the literals are the durations the shipped ramp already computes, which is why this substitution is the sole carve-out from (c)'s expected-value rule rather than a break in it. **(b) Then prove the instrument**: apply the wrong delay at both `settle.go` sites over a `cp` backup — `backoff(k+1, …)`, in the shipped ramp's own units, which is the omitted translation's arithmetic and, unlike `Exponential(k, …)`, compiles while `func backoff` still stands — run `go test ./internal/scheduler/ -run 'TestFailurePolicy_oneShotAttemptsGrowAndGiveUp\|TestDeadline_successiveBreaches_growingDelay'`, require **RED**, restore from the backup (never `git checkout -- <file>`). A green probe is a STOP for the orchestrator. **(c) Then the adoption**: delete `backoffDelay` and `backoff`, re-point **every** call site — production and test — with the one-based→zero-based translation written at `settle.go`'s call sites **and** at `cadence_test.go`'s, per D2's per-file disposition; every *pre-existing* assertion and expected value stays byte-identical, an argument may carry the translation and nothing else in those files moves. `cadence_test.go`'s `TestBackoff_exactTable` and `TestBackoff_strictlyGrowingUntilCeiling` stay in place under that rule — a re-point that leaves their argument standing reds the one-based table (measured in D2), and re-indexing that table's `failures` column instead of the argument is a scope-boundary item to surface, never the fix. The listed sites are a floor enumerated by `rg -U`; `make verify` is re-run after they clear, and any newly revealed site outside this contract is surfaced, not absorbed | `internal/tg/retry.go`, `internal/tg/caller.go`, `internal/tg/retry_test.go`, `internal/scheduler/cadence.go`, `internal/scheduler/settle.go`, `internal/scheduler/cadence_test.go`, `internal/scheduler/failure_test.go`, `internal/scheduler/deadline_test.go` | 1 |
| 3 | `config.Ingest` + the `LAB_GAME_INGEST_` reader; `EnvKeys()` append; **both** `Load` cross-checks on `LAB_GAME_INGEST_LONG_POLL_TIMEOUT` per D16 — strictly below `AttemptTimeout`, and a whole number of seconds — each a `*KeyError` naming that variable; `.env.example` lines | `internal/config/ingest.go`, `internal/config/config.go`, `internal/config/env.go`, `internal/config/ingest_test.go`, `.env.example` | — |
| 4 | Forward migration `00004_ingest.sql`: `ingest_offset` (guarded singleton, seeded) and `ingest_dead_update`. **The two new tables break `internal/store`'s exact base-table assertion**, which enumerates the whole set and compares with `slices.Equal` [measured febae63:internal/store/migrate_test.go:41-47 · `sed -n '41,47p' internal/store/migrate_test.go` → `want := []string{` … `"account", "account_balance", "account_definition",` … `"scope", "scope_definition",` … `}`]; that set is extended with both new names in this subtask, and the new tables' constraints get their cases in `schema_test.go`, so Group A leaves `internal/store` green on its own rather than relying on its consumer in Group B | `internal/store/migrations/00004_ingest.sql`, `internal/store/migrate_test.go`, `internal/store/schema_test.go` | — |
| 5 | `store.Queryer` and `store.PlayerExists` — the read-only owner lookup, with its tests; **and the delete-and-re-point of the package's shipped test-local near-duplicate** per D11: `queryRower` is removed and `balanceOf` takes `store.Queryer`, so one method set has one name in this package | `internal/store/owner.go`, `internal/store/owner_test.go`, `internal/store/post_test.go` | — |
| 6 | `internal/ingest` core types: package comment, `Kind` with D4's table and its date/chat extractors, `IDSpace` and the unexported `operation_id` builder, `Handler`/`Update`, `Route`/`Router`/`NewRouter`/`Kinds`, the package's sentinels. **The `Handler` doc comment carries more than the ctx-propagation obligation**: propagate the handed `ctx` to every call on `tx` (D7's risk row); use that handed `ctx` and never the one riding inside the update, which for an update off this poll path is a `context.Background()` (D9, spec *Technical constraints* item 5); and issue no outbound Bot API call whose permission rests on a row this transaction has not committed (D18). `Update` holds the raw `telego.Update` in a named field, never embedded (D9) | `internal/ingest/doc.go`, `internal/ingest/kind.go`, `internal/ingest/operation.go`, `internal/ingest/router.go`, `internal/ingest/errors.go`, plus their `_test.go` files | — |
| 7 | Offset and give-up storage: read/guarded-advance statements, `DeadUpdate`, `DeadUpdates` over a caller-owned `pgx.Tx` with a deterministic order. **The package's `TestMain` over `testdb.Main` lands here**, in the first subtask whose scenarios take a schema: `testdb.Schema` fails the calling test when `Main` has not provisioned a database [measured c37e642:internal/testdb/testdb.go:112-116 · `sed -n '112,116p' internal/testdb/testdb.go` → `func Schema(tb testing.TB) *pgxpool.Config {` … `tb.Fatalf("testdb: Schema called before testdb.Main provisioned a database")`], so leaving it to a later subtask would red every database-backed scenario of subtasks 7, 9 and 10 until that subtask landed — against this design's own each-step-leaves-its-package-green norm, stated for subtasks 4 and 5 | `internal/ingest/offset.go`, `internal/ingest/dead.go`, plus their `_test.go` files, and `internal/ingest/main_test.go` | 4, 6 |
| 8 | The observation seam: `Outcome`, `Observation`, `LoopObservation`, `Observer`, and the nil-checked report helpers | `internal/ingest/observe.go`, `internal/ingest/observe_test.go` | 6 |
| 9 | The loop: `Options`/`New`/`OptionError`, the requested-kinds set with D3's sentinel, `PollOnce`, `Run` with D19's poll-error policy (observe through `LoopObservation.Err` and continue at the poll interval; return non-nil only on cancellation, as `ctx.Err()`), per-attempt transaction, `recover`, sentinel classification, bounded retry, settlement and offset advance (writing `update_id + 1` per D14), give-up row. **The Files column names this row's home, not a one-file mandate**: the responsibilities listed here are a plausible run at the soft file-size bands [measured df9b1a2:AGENTS.md:128 · `grep -n 'File size' AGENTS.md` → `- **File size:** soft 500/800; hard 1000, and 1500 for \`_test.go\` — both gated; exemptions and the don't-over-split counter-rule are in \`code-style.md\`.`], so split along the seams this row already names — the poll cycle, the per-update attempt loop, the settlement — deliberately at authoring time rather than reactively at the gate, after reading `code-style.md`'s don't-over-split counter-rule | `internal/ingest/loop.go` (splitting as above), `internal/ingest/loop_test.go`, `internal/ingest/retry_test.go` | 1, 3, 6, 7, 8 |
| 10 | The gate: `PlayerLookup`, the pool-backed implementation, `Gate`, `NewGate`, the positive-only cache | `internal/ingest/gate.go`, `internal/ingest/gate_test.go` | 5, 6 |
| 11 | Guard tests: no `panic(`/`log.Fatal`/`os.Exit` in the package's non-test source, no metrics-library import, the `telego.Update` field-vs-kind-table drift check, and the structural source walks § Test Design specifies — AC2's ctx-first walk with D9's riding-context clauses, AC8's no-transaction-handed-out walk, and AC38's no-second-Bot-API-path walk | `internal/ingest/guards_test.go` | 6, 9, 10 |
| 12 | Propagation: the layout paragraph and the code inventory in `ai-docs/context.md`; the allowlist and sanitisation bullets in `ai-docs/domain-invariants.md` — the player carve-out, the cache-invalidation obligation, D18's no-outbound-call-on-an-uncommitted-row obligation, and the concrete §12.5 targets this task creates per D14 (reset `ingest_offset`, which is what the existing «reset the updates offset» clause now names; rewrite `ingest_dead_update`'s chat id; blank its `last_error` free text); the new Key-Decision entries; the `INDEX.md` row. **`ai-docs/context-status.md` is written for this task, and not by this subtask** — spec *Technical constraints* item 8 names it among the propagation class, and it receives its entry the way it receives every entry: appended by `/task` Step 9.5 after implementation, since it is that step's own append-only per-task log [measured 07e5177:ai-docs/context-status.md:3 · `sed -n '3p' ai-docs/context-status.md` → `The detailed, append-only implementation log: one entry per completed task … Written by /task Step 9.5, read on demand when touching the area an entry covers.`]. This row therefore leaves that append to Step 9.5 rather than duplicating it here, and touches none of the file's past entries. **The reason is timing, not a standing exemption**, and the distinction matters because row 16 later depends on it: `AGENTS.md` § Propagation Rule step 4 names `ai-docs/learnings.md` and `ai-docs/plans/done/**` as the history surfaces it leaves untouched and does **not** name this file [measured c222623:AGENTS.md:246 · `sed -n '246p' AGENTS.md` → `… Completeness test: every LIVE doc must agree; history surfaces (\`ai-docs/learnings.md\`, \`ai-docs/plans/done/**\`) are left untouched.`], so no rule exempts `context-status.md` from a propagation sweep. What is true at *this* row's moment is narrower: the entry does not exist yet, because Step 9.5 has not run. Once it does exist it is an ordinary live claim like every other, which is precisely why row 16 owns a clause of it. AC32's class is satisfied by that division of labour, not by an exclusion from the class | `ai-docs/context.md`, `ai-docs/domain-invariants.md`, `ai-docs/key-decisions.md`, `ai-docs/plans/INDEX.md` | 1–11 |
| 13 | **The two shared names, with no signature change yet**, so the module stays green at this step: `internal/backoff` exports `DefaultFactor` — the named constant D20 lifts the hard-coded `2` into, documented as the value that reproduces the shipped ramp exactly — and `ValidFactor`, the single definition of the legal-factor boundary (finite and strictly greater than `1`) that row 14's reader and row 15's constructors all call instead of each restating it (D20's `≥ 3`-site argument). The package comment gains the boundary sentence, and each new exported name carries the doc comment starting with its own name that `revive`'s `exported` rule requires [measured fc5e6dd:.golangci.yml:45-48 · `sed -n '45,48p' .golangci.yml` → `revive:` / `rules:` / `- name: exported` / `- name: package-comments`]; the tests pin `DefaultFactor`'s value and `ValidFactor`'s answers, the `NaN`, `±Inf` and exactly-`1` rows included | `internal/backoff/backoff.go`, `internal/backoff/backoff_test.go` | — |
| 14 | The configuration surface (D20): `LAB_GAME_TG_RETRY_FACTOR`, `LAB_GAME_SCHEDULER_RETRY_FACTOR` and `LAB_GAME_INGEST_RETRY_FACTOR`, each declared last in its scope's retry family and appended to that scope's `*EnvKeys()` in declaration order — which is what puts each key in the aggregate `config.EnvKeys()` AC39 names, since that function is the concatenation of the three per-scope lists [measured ef69c56:internal/config/env.go:53-58 · `sed -n '53,58p' internal/config/env.go` → `func EnvKeys() []string {` / `keys := append(envKeys(), envBalancePath, envWorldPath)` / `keys = append(keys, transportEnvKeys()...)` / `keys = append(keys, schedulerEnvKeys()...)` / `return append(keys, ingestEnvKeys()...)`]; a `RetryFactor float64` field on `config.Transport`, `config.Scheduler` and `config.Ingest`, each defaulting to `backoff.DefaultFactor` — which gives `internal/config` its first import of another package in this module, in the one direction that cannot cycle (D20); a `lookupFactor` helper beside `lookupPositiveInt` and `lookupPositiveDuration`, same `(value, present, error)` shape and same unconditional query, refusing anything `ValidFactor` refuses with a `*KeyError` naming the key; one `.env.example` line per key, placed beside its own scope's other retry keys as AC39 requires and carrying its default as the value, because the disjointness test asserts set equality between that file, `EnvKeys()` and the keys the loader actually consulted; and the count-bearing `*EnvKeys()` doc comments in the files this row already edits. `defaultIngest`'s worst-case-stall arithmetic gains its "under the default factor" qualifier, because that sum is now conditional on a configured value. **Three live `RetryMaxDelay` doc comments in the same three files say the ramp *doubles*, which is false at any non-default factor**, and this row rewrites each to name the configured factor instead [measured 515f03d:internal/config/transport.go:105, internal/config/scheduler.go:59 and internal/config/ingest.go:67 · `grep -rn 'doubling' internal/config/*.go` → `RetryMaxDelay caps the backoff scale's doubling` at each] | `internal/config/transport.go`, `internal/config/scheduler.go`, `internal/config/ingest.go`, their `_test.go` files, `.env.example` | 13 |
| 15 | **The signature change and every call site in one step, because Go admits no intermediate state that compiles** — and because a second, factor-taking entry point beside the current one is exactly the two-APIs-side-by-side row `AGENTS.md` § API Stability deletes (D20). `Exponential` and `EqualJitter` gain `factor float64` after `ceiling` (`jitter` stays last) and are re-expressed over `math.Pow`, with the clamp as the last step before the float→`time.Duration` conversion and the `NaN` case guarded by an explicit `math.IsNaN` test; the package comment states D20's contract table in full. Each adopter's constructor gains its factor check **last in its existing chain**, so every shipped invalid-config row still fails naming its own field; each production call site — `internal/tg/caller.go`'s retry wait, `internal/scheduler/settle.go`'s two persisted-`run_at` computations, `internal/ingest/attempt.go`'s between-attempt delay — passes **its own `cfg.RetryFactor`**, never `backoff.DefaultFactor`; each package's test config builder and each invalid-config table row gains a legal factor; and each *direct* test call passes a literal `2` rather than `DefaultFactor`, so a later default move cannot silently carry a pinned table with it. Every pre-existing assertion and expected value stays byte-identical — inside a call expression the argument list is the only thing that moves — and D2's three-class rule governs the rest of the diff unchanged: **call expressions** re-point and gain the argument; **comment prose** and **`t.Errorf` format strings** are neither assertions nor expected values, so where one describes the ramp's *contract* or the deleted *loop's structure* it is re-worded to stay true, while one describing what a specific default-factor row computes stays as it is. D20's closing checklist names the sites of that second class in `internal/backoff` and `internal/tg`; it names them as a checklist, and the decision rule beside it is what settles a site the list does not carry. The literal one-based ramps in `failure_test.go` and `deadline_test.go` are untouched, still literal, still at the default. `internal/backoff`'s table gains D20's factor rows, and each **production call site** gains D20's non-default-factor scenario as a **new** test function beside the shipped ones, never as an edit to one — which for `internal/scheduler` is **two** scenarios rather than one, because `settle.go` computes a persisted `run_at` in two separate production functions, each reading `cfg` for itself (D20). **A constructor's factor check reds every *inline* config literal too, not only the test config builders**, so this row's Files carry `internal/ingest/gate_test.go`, whose `tg.New` call builds its `config.Transport` as a literal in place rather than through a builder [measured 515f03d:internal/ingest/gate_test.go:227-238 · `sed -n '227,238p' internal/ingest/gate_test.go` → `client, err := tg.New(tg.Options{` … `Transport: config.Transport{` / `RetryMaxAttempts: 1,` / `RetryBaseDelay:   time.Millisecond,` / `RetryMaxDelay:    time.Millisecond,` / `AttemptTimeout:   5 * time.Second,`]. As in row 2, **the listed sites are a floor enumerated by `rg`, not a closed set**: `make verify` is re-run after they clear, and a newly revealed site is surfaced to the orchestrator, not absorbed | `internal/backoff/backoff.go`, `internal/backoff/backoff_test.go`, `internal/tg/client.go`, `internal/tg/caller.go`, `internal/tg/client_test.go`, `internal/tg/retry_test.go`, `internal/scheduler/worker.go`, `internal/scheduler/settle.go`, `internal/scheduler/worker_test.go`, `internal/scheduler/cadence_test.go`, `internal/scheduler/failure_test.go`, `internal/scheduler/deadline_test.go`, `internal/ingest/loop.go`, `internal/ingest/attempt.go`, `internal/ingest/loop_test.go`, `internal/ingest/retry_test.go`, `internal/ingest/gate_test.go` | 13, 14 |
| 16 | Propagation of the amendment: KD-31's amendment clause, KD-27's per-scope key enumeration, and `ai-docs/context.md`'s layout and inventory sentences — D20's closing checklist names each claim that goes stale and, for each, whether the repair is to update it or to strike it. **`ai-docs/context-status.md` is this row's too, for one clause of one sentence.** The entry `/task` Step 9.5 already appended for *this* task — its heading names this PR and this task's own date — asserts a tally for the `LAB_GAME_INGEST_*` key family in its *What landed* bullet, and row 14 makes that assertion false inside the very PR the entry describes [measured c222623:ai-docs/context-status.md:147 · `grep -n 'the six \`LAB_GAME_INGEST_\*\` tuning keys' ai-docs/context-status.md` → one hit, inside `## Update ingestion — long polling, dispatch, operation idempotency, the chat allowlist (PR #66, 2026-09-07)`]. Step 9.5 has already run for this task and only ever *appends*, so no later step of the plan reaches that clause — this row does. The repair is to **strike the quantifier**, leaving the key family named and uncounted, never to write the amendment's new number: `/task`'s Step 9.5 forbids a count in this file outright [measured c222623:.claude/skills/task/SKILL.md:178 · `grep -n 'No counts here' .claude/skills/task/SKILL.md` → **No counts here — name the things, do not tally them.** … `none of it goes into \`context-status.md\` or \`context.md\``], and `ai-docs/context.md`'s inventory sentence takes the identical treatment for the same reason (D20's closing checklist). Nothing else in the file moves: every other entry, and every other clause of this one, is left exactly as Step 9.5 wrote it | `ai-docs/key-decisions.md`, `ai-docs/context.md`, `ai-docs/context-status.md` | 13–15 |

## Handoff plan

Grouping is required for every design with `M ≥ 1`, and this design's `M` is the count of rows in
§ Decomposition. Each group below is homogeneous by change-type — **code** (`*.go`, migrations, and
the code-gated `.env.example`) or **instructions/harness** (`*.md`, `.claude/**`, `AGENTS.md`,
`ai-docs/**`) — never both; the maximum group size is `10` consecutive subtasks and each group ends
at whichever comes first, the size cap, a change-type switch, or a dependency-forced boundary; the
terminal group's size stays within `1..=10`; the groups are minimised (the code subtasks are
clustered rather than interleaved with the documentation one); and the rounds-1-5 total — Groups A,
B and C below — stays at or under the default maximum of `4` design-defined groups. **The round-6
amendment adds two more**, taking the total across the task's whole life past that maximum, which
§ *Amendment groups (round 6)* below surfaced to the owner rather than treating as self-approved —
and which the owner then approved.

- **Handoff into Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § Compaction recovery (re-entry). The handoff binds at the start of **every** group, the first
  included.
- **Group A** — model `sonnet`, effort `medium` (pinned) via the `code-writer` subagent, 1M-token
  window — subtasks 1–6 (code change-type: `*.go`, `*.sql`, `.env.example`). The code subtasks as a
  block exceed the `10` cap, so they split into two same-model code groups; the boundary is placed
  where the dependency graph already puts a seam — everything through 6 is standalone or
  dependency-satisfied within the group, and 7 opens the database-backed half once 4 and 6 have
  landed.
  - **Spawn contract, stated because subtask 2 is the highest-consequence work in this `sonnet`
    group.** It rewrites persisted-`run_at` arithmetic in two shipped packages that no AC of this
    task names, so the group's prompt must carry all of the following, in this order.
    - **The call-site gate of D2, as a deliverable and not as a check** — the literal one-based ramp
      in `internal/scheduler/failure_test.go` and `internal/scheduler/deadline_test.go`, landed and
      green **before** the re-point. Stated explicitly because the byte-identical-assertion gate,
      taken alone, does **not** discriminate the omitted one-based→zero-based translation **at
      `settle.go`**. The prompt carries D2's measured account of why, in those words: an
      un-translated re-point reds `cadence_test.go`'s one-based `TestBackoff_exactTable`, so the
      implementor is forced to translate the argument *there*; and once that is done the shipped
      scheduler suite returns the same green whether or not the same translation reaches
      `settle.go` — measured both ways, because the assertions over a persisted `run_at` derive
      their expected value from the very function the call site calls. Neither the byte-identical
      rule nor a green `cadence_test.go` may be read as the whole gate.
    - **The mutation probe, with its refusal condition.** With the literal ramp in place and the
      re-point not yet applied, the wrong delay written over a `cp` backup of `settle.go` as
      `backoff(k+1, …)` — the omitted translation's arithmetic in the shipped ramp's own units, and
      the form that compiles while `func backoff` still stands — must turn
      `go test ./internal/scheduler/ -run
      'TestFailurePolicy_oneShotAttemptsGrowAndGiveUp|TestDeadline_successiveBreaches_growingDelay'`
      **RED**; restore from the backup, never with `git checkout -- <file>`. If the probe comes
      back green, the implementor
      **stops and returns to the orchestrator** rather than proceeding — a gate that cannot go red
      is not evidence about the adoption (`AGENTS.md` § Patterns 2).
    - **`go test ./internal/tg/ ./internal/scheduler/` is green before the group returns**, run as
      its own step rather than folded into a final `make verify`. The point is *where* a changed
      pre-existing expected value surfaces — inside the group, with the adoption diff still in hand
      and the implementor able to escalate it as the scope-boundary item D2 says it is, rather than
      at `make verify` after the rest of the group has landed on top of it. `internal/store`'s suite
      gets the same treatment for subtask 5's `queryRower` re-point (`go test ./internal/store/`),
      for the same reason at a smaller scale.
- **Handoff after Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § Compaction recovery (re-entry). The parent `/task` resumes in Group B with fresh context.
- **Group B** — model `sonnet`, effort `medium` (pinned) via the `code-writer` subagent, 1M-token
  window — subtasks 7–11 (code change-type: `*.go`). Within the `≤ 10` cap.
- **Handoff after Group B:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § Compaction recovery (re-entry).
- **Group C** — model `inherit` (the orchestrator's), effort inherited from the orchestrator
  (typically xHigh) — **not** pinned — via the `general-purpose` subagent with no inline `model=`,
  1M-token window — subtask 12 (instructions/harness change-type: `ai-docs/**`). Terminal group;
  its size is within the `1..=10` range. The change-type switch at the Group B/C boundary is what
  forces this group even though Group B is under the cap.

### Amendment groups (round 6)

Groups A–C above are **closed**: their subtasks are implemented on this branch
[measured fc5e6dd · `git log --oneline -n 12` → `0f31fa5 feat(ingest): add the update-ingestion loop,
gate and idempotency`] under the open PR for this task
[measured fc5e6dd · `gh pr view 66 --json number,state,title` →
`{"number":66,"state":"OPEN","title":"Update ingestion: long polling, dispatch, operation
idempotency, chat allowlist"}`], so they are not re-run. The amendment's rows 13–16 are grouped under
the same rules the preamble above states — homogeneous by change-type, at most `10` subtasks per
group, terminal group within `1..=10`, minimised, and every group entered through a `/context-reset`
handoff.

- **Handoff into Group D:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § Compaction recovery (re-entry). The handoff binds at the start of **every** group, this one
  included.
- **Group D** — model `sonnet`, effort `medium` (pinned) via the `code-writer` subagent, 1M-token
  window — subtasks 13, 14, 15 (code change-type: `*.go`, `.env.example`). Rows 13 and 14 each leave
  the module green on their own; row 15 is atomic by necessity and carries the spawn contract below.
  - **Spawn contract for subtask 15, stated because it is the amendment's highest-consequence step:
    it rewrites arithmetic that three shipped packages call, one of them into persisted data.** The
    prompt must carry all of the following, in this order. **(1) The default does not move.**
    `DefaultFactor` is `2`, every adopter's configured default is `2`, and therefore every pinned
    ramp value in the shipped test files D20's opening names stays valid; a pre-existing expected value that
    has to change is the signal that the amendment was not behaviour-preserving, and is a
    scope-boundary item for the orchestrator rather than something the implementor absorbs — the same
    rule D2 already states, unchanged. **(2) The call sites read the configured field.** Passing
    `backoff.DefaultFactor` at a production call site leaves the whole shipped suite green while the
    feature does nothing, because every shipped test runs at the default (D20); the non-default-factor
    scenario in each adopter is the only instrument that separates the two, and it lands with the
    re-point rather than after it. **(3) The clamp is the last step before the conversion.** No
    `time.Duration` conversion of a value that has not been proven strictly below the ceiling, `NaN`
    guarded explicitly; a `base << attempt`, a loop, or a clamp after the multiplication is a
    regression, not a translation (D20). **(4)** `go test ./internal/backoff/ ./internal/config/
    ./internal/tg/ ./internal/scheduler/ ./internal/ingest/` is green before the group returns, run
    as its own step rather than folded into a final `make verify`, so a moved expected value surfaces
    while the adoption diff is still in hand.
- **Handoff after Group D:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § Compaction recovery (re-entry). The parent `/task` resumes in Group E with fresh context.
- **Group E** — model `inherit` (the orchestrator's), effort inherited from the orchestrator
  (typically xHigh) — **not** pinned — via the `general-purpose` subagent with no inline `model=`,
  1M-token window — subtask 16 (instructions/harness change-type: `ai-docs/**`). Terminal group; its
  size is within the `1..=10` range. The change-type switch at the D/E boundary is what forces it,
  even though Group D is under the cap.
- **Group count — surfaced, then approved.** The amendment's own live count is **two**, within the
  default maximum of four. Counted across the task's whole life this design defines five groups,
  which is past that maximum, so `design-writer` § Rules handoff-grouping (h) made it the owner's
  call rather than the design's — and the owner made it: **five design-defined groups are approved**,
  and the amendment runs as Groups D and E on this branch rather than as an issue of its own
  [measured ef69c56:ai-docs/plans/2026-09-06-update-ingestion-dispatch-idempotency.progress.md:424 ·
  `sed -n '424p' ai-docs/plans/2026-09-06-update-ingestion-dispatch-idempotency.progress.md` → `- Amendment: the owner
  approved **five** design-defined groups over the default maximum of four (handoff-grouping (h)
  makes an overflow the owner's call, never the design's). …`]. The three earlier groups are closed
  and do not run again, so the live count at implementation time is two. Nothing here is left for a
  later reader to decide.

## Risks

- **The `internal/tg` and `internal/scheduler` adoption of `internal/backoff` (D2) changes shipped,
  well-tested code that no AC of this task names, and its blast radius reaches production and test
  files in both packages.** The production reach is `internal/tg/caller.go`'s retry wait and
  `internal/scheduler/settle.go`'s persisted `run_at` computations; the test reach is the direct
  calls in `internal/tg/retry_test.go`, `internal/scheduler/cadence_test.go`,
  `internal/scheduler/failure_test.go` and `internal/scheduler/deadline_test.go`
  `[measured febae63 · `rg -U --type go -n 'backoffDelay\s*\(' internal/tg/` and
  `rg -U --pcre2 --type go -n '(?<![A-Za-z_.])backoff\s*\(' internal/scheduler/` → those files and
  no others, each package's declaration included]`.
  Mitigation, in two parts, because the first part alone is refuted by measurement. **(1)** After a
  delete-and-re-point those test files do **not** compile unchanged, so "they pass unchanged" is a
  mitigation that cannot hold; what holds is that **every *pre-existing* assertion and expected value
  in them stays byte-identical, while the call expression is re-pointed and its argument may carry
  the one-based→zero-based translation** — an argument may carry the translation, an expected value
  may not move — so a changed pre-existing expected value is the signal that the adoption was not
  behaviour-preserving and is a scope-boundary item for the orchestrator, not something the
  implementor absorbs. **(2)** That gate
  does **not** discriminate the one defect class it most needs to — the omitted one-based→zero-based
  translation at `internal/scheduler`'s persisted-`run_at` sites — because the shipped assertions
  over a persisted retry `run_at` compute their expected value through the same function the call
  site uses `[measured df9b1a2:internal/scheduler/failure_test.go:154 and
  internal/scheduler/deadline_test.go:252 · `rg -n --pcre2 '(?<![A-Za-z_.])backoff\s*\('
  internal/scheduler/*_test.go` → `want := backoff(attempt, cfg.RetryBaseDelay, cfg.RetryMaxDelay)`
  at each; every other test hit pins the function itself in `cadence_test.go`, or is comment and
  failure-message prose]`. Measured, not reasoned: an un-translated re-point **is** caught, and only
  at `cadence_test.go`'s one-based `TestBackoff_exactTable` `[measured 07e5177 · a scratch copy of
  this tree with `Exponential` added and every call re-pointed with its argument untouched ·
  `go test ./internal/scheduler/` → `--- FAIL: TestBackoff_exactTable`, `backoff(1, 1s, 30s) = 2s,
  want 1s` … `backoff(5, 1s, 30s) = 30s, want 16s`, the persisted-`run_at` tests green]`; translating
  the argument there — the minimal fix for the only reported failure — and leaving `settle.go` and
  both `want :=` lines zero-based is **green** `[measured 07e5177 · the same scratch tree, that
  translation applied at `cadence_test.go` only · `go test ./internal/scheduler/` → `ok
  github.com/maratik123/lab-game/internal/scheduler`]`, and so is the correct translation
  `[measured 07e5177 · the same scratch tree with `Exponential(k-1, …)` at both `settle.go` sites and
  `Exponential(attempt-1, …)` at both `want :=` lines · `go test ./internal/scheduler/` → `ok
  github.com/maratik123/lab-game/internal/scheduler`]`. The suite's verdict is invariant to the
  call-site translation while every persisted retry delay doubles under one of them. D2
  therefore adds a **call-site** gate ahead of the adoption: a literal one-based ramp in
  `failure_test.go` and `deadline_test.go`, green against the shipped ramp first, then proven to go
  RED under the wrong delay (`backoff(k+1, …)`) before the re-point lands —
  `[derived → subtask 2's literal-ramp gate and its mutation probe]`. The tests being preserved are
  the ones already shipped
  `[measured 1fce8b5:internal/scheduler/cadence_test.go:8,33; internal/tg/retry_test.go:127,682 ·
  `grep -n 'func Test.*[Bb]ackoff' internal/scheduler/cadence_test.go internal/tg/retry_test.go` →
  `TestRetry_JitterOptionThreadedThroughToBackoff`, `TestBackoffDelay_JitterBoundsExactly`,
  `TestBackoff_exactTable`, `TestBackoff_strictlyGrowingUntilCeiling`]`.
- **A shared `Exponential` written the obvious way overflows where the ramps it replaces do not, and
  the damage lands in persisted data.** Neither shipped ramp doubles a value it has not first tested
  against the ceiling `[measured c221784:internal/tg/retry.go:33-34 and
  internal/scheduler/cadence.go:46-47 · `sed -n '33,34p' internal/tg/retry.go; sed -n '46,47p'
  internal/scheduler/cadence.go` → `if d > maxDelay/2 {` / `d = maxDelay` and `if d >= ceiling {` /
  `return ceiling`]`, so the doubling each does is one the ceiling has already bounded. A
  `base << attempt`, or a loop whose ceiling test comes after the doubling, has no such bound — and
  the attempt is operator-reachable, since the
  attempt caps are parsed by a helper that bounds only from below `[measured
  c221784:internal/config/transport.go:207-217 · `sed -n '207,217p' internal/config/transport.go` →
  `func lookupPositiveInt(…)` whose only rejection is `if err != nil || n <= 0`]` and
  `internal/scheduler` feeds `consecutive_failures` straight in `[measured
  c221784:internal/scheduler/settle.go:142,242 · `grep -n 'backoff(k,' internal/scheduler/settle.go`
  → the same `s.Add(backoff(k, cfg.RetryBaseDelay, cfg.RetryMaxDelay))` at both lines]`. A negative
  delay there writes a `run_at` in the past — a hot-looping dead task, inside a change billed as
  behaviour-preserving. Mitigation: D2 makes clamp-before-overflow part of `Exponential`'s stated
  contract rather than an implementation accident, and subtask 1's table asserts it at attempts far
  past any a naive shift survives — `[derived → AC23 and subtask 1's overflow cases]`.
- **The gate cannot see a row the handler's own open transaction wrote, and §1's onboarding is
  exactly that shape.** A handler that creates a player's `owner` row and DMs that player before
  returning is refused on every attempt and lands in `ingest_dead_update`. Mitigation: D18 makes this
  a `Handler`-contract rule rather than a discovery — the obligation is on the `Handler` doc comment
  (subtask 6), pinned by a negative scenario (subtask 10) and written into
  `ai-docs/domain-invariants.md` (subtask 12) — and the designed route for a post-commit send is
  #43's notification queue, which is out of this task's scope. The residual risk is that #30 writes
  the naive shape anyway; the doc comment and the invariants bullet are what a reviewer of #30 reads
  — `[derived → subtask 10's uncommitted-owner-row scenario and subtask 12]`.
- **`Run` never returning on a persistently failing Bot API is the deliberate choice, so the failure
  is visible only through the observation seam.** D19 keeps the loop polling at
  `LAB_GAME_INGEST_POLL_INTERVAL` and reports each failure through `LoopObservation.Err`; with no
  exporter in this task (#23 owns exposition), a sustained outage is silent to an operator until #23
  lands. That is the same standing `internal/scheduler` already ships under, and §13.2's alerting
  pairs canary failures with rising update lag rather than watching this loop's return value.
  Mitigation: the poll error is reported, not swallowed, and `PollOnce` still returns it, so a caller
  that wants to stop on one has the primitive — `[derived → AC17 and subtask 9's poll-error
  scenario]`.
- **`00004_ingest.sql` reds `internal/store`'s own suite the moment it lands**, because that suite
  asserts the whole base-table set by equality rather than by containment, so any new table is a
  failure until the expectation is extended `[measured febae63:internal/store/migrate_test.go:41-49 ·
  `sed -n '41,49p' internal/store/migrate_test.go` → the `want := []string{…}` base-table list
  followed by `if !slices.Equal(tables, want)`]`. Mitigation: subtask 4 owns the migration and that
  expectation in one step, so its group leaves `internal/store` green without waiting for its
  consumer in the next group — `[derived → AC7, AC30]`.
- **The sentinel kind of D3 is a claim about what this bot can receive, and only the shipped code can
  keep it true.** If a future mechanic ever sends an invoice with a flexible price, the sentinel
  becomes a live update kind. Mitigation: the sentinel is transmitted only while the route set is
  empty, so the first registered route removes it; the loop settles it as unrouted if it ever
  arrives — `[derived → AC5, AC9]`.
- **A first start on a database seeded at offset 0 asks the Bot API for the earliest unconfirmed
  update, not for "new updates from now".** `getUpdates` with no offset returns "updates starting
  with the earliest unconfirmed update"
  `[measured 1fce8b5:telego@v1.11.2/methods.go:11-18 · `sed -n '11,18p'
  $(go env GOMODCACHE)/github.com/mymmrac/telego@v1.11.2/methods.go` → `Offset - Optional. Identifier
  of the first update to be returned. … By default, updates starting with the earliest unconfirmed
  update are returned.`]`, so a first start can face whatever backlog the server still holds, drained
  one transaction at a time. Mitigation: the batch limit bounds one cycle, the loop is restart-safe
  because each update is settled with its own advance, and the give-up path bounds a poisoned
  update — `[derived → AC6, AC24, AC25]`.
- **A handler that ignores its context stalls the loop indefinitely** (D7 ships no per-update
  deadline). Mitigation: the contract states the obligation on the `Handler` doc comment, as
  `scheduler.Handler`'s already does; no production handler exists in this task to violate it, and
  the deadline is recorded in § Open questions rather than built speculatively —
  `[measured 1fce8b5:internal/scheduler/task.go:92-101 · `sed -n '92,101p'
  internal/scheduler/task.go` → `A Handler MUST propagate the ctx it is handed to every call it makes
  on tx`]`.
- **The gate's positive cache is unbounded for the process's lifetime and is never invalidated.**
  Nothing in this task deletes an owner row, so the cache cannot go stale in the wrong direction;
  a later mechanic that deletes or re-keys a player owner row must invalidate it. Mitigation: the
  obligation is written into `ai-docs/domain-invariants.md` in subtask 12, not only into a code
  comment — `[derived → AC35 and subtask 12]`.
- **`AllowCall` is a concurrent seam, so the cache's shape is a correctness question, not a
  performance one.** An unsynchronised map there is a `concurrent map writes` fatal error on the
  component whose whole job is to stop a write to an unintended chat — the worst possible place for
  a crash — and the outbound path is concurrent by construction, which is why the limiter
  immediately downstream keeps its own state under a mutex `[measured
  febae63:internal/tg/limit.go:202-207 · `sed -n '202,207p' internal/tg/limit.go` → `type Limiter
  struct {` / `mu     sync.Mutex` / `chats  map[chatKey]*schedule`]`. Mitigation: D10 fixes the
  concrete shape (a mutex-guarded `map[int64]struct{}`, with the lookup issued outside the lock),
  and subtask 10 carries a scenario driving `AllowCall` from concurrent goroutines under
  `go test -race`, which `AGENTS.md` § Go Test Conventions makes a required gate for shared state —
  `[derived → AC35 and subtask 10's concurrency scenario]`.
- **Timing tests cannot run inside a `testing/synctest` bubble here.** KD-26 makes `synctest` this
  project's clock, but the loop's every path touches a real Postgres connection, which is not durably
  blockable inside a bubble. Mitigation: the retry-timing cases use small real durations from
  `config.Ingest`, exactly as `internal/scheduler`'s failure and deadline tests already do; the
  bubble is used only where a case touches no database — `[measured 1fce8b5:internal/scheduler/failure_test.go:150-154
  · `sed -n '150,154p' internal/scheduler/failure_test.go` → `want := backoff(attempt,
  cfg.RetryBaseDelay, cfg.RetryMaxDelay)` inside a real-duration assertion]`.
- **`golangci-lint`'s `exhaustive` is enabled, so every switch on `Kind`, `Outcome` or `IDSpace` must
  be total.** `default-signifies-exhaustive: true` is configured, so a `default` clause satisfies it.
  Mitigation: the design's derivations are table lookups, not switches, and the one place a switch is
  natural — rendering an `Outcome`'s name — carries a `default` —
  `[measured 1fce8b5:.golangci.yml:40-41 · `grep -n -A2 'exhaustive:' .golangci.yml` →
  `40:    exhaustive:` / `41:      default-signifies-exhaustive: true`]`.
- **The build and lint gates truncate, so any "N sites" count taken from a first run is a floor**
  `[measured 1fce8b5:.claude/agents/design-writer.md:129 · `grep -n 'too many errors'
  .claude/agents/design-writer.md` → `Measured on this toolchain: `go build ./...` prints at most
  **10 errors per package** … `golangci-lint run` defaults to `--max-issues-per-linter 50` and
  `--max-same-issues 3``]`. Mitigation: subtask 2's adoption re-runs
  `make verify` after the enumerated sites clear, and a newly revealed out-of-contract class is
  surfaced to the orchestrator rather than absorbed — `[derived → AC30]`.
- **The `.env.example` set-equality test fails by name in both directions**, so an ingest key added
  to the loader without a matching line (or the reverse) reds the config package. Mitigation: subtask
  3 owns both edits in one step — `[measured 1fce8b5:internal/config/disjoint_test.go:12-17 ·
  `sed -n '12,17p' internal/config/disjoint_test.go` → `This is AC16's "disjoint in fact, not only in
  prose": the recorded set is what the loader actually consulted, not what a reader believes it
  consults.`]`.
- **The amendment reopens KD-31's invariance shape one argument to the right: a production call site
  that passes `backoff.DefaultFactor` instead of its own `cfg.RetryFactor` leaves the whole shipped
  suite green while the feature does nothing.** Every shipped test runs at the default, so the
  byte-identical ramp values cannot see it, `internal/backoff`'s own table cannot see it, and D2's
  literal one-based ramps — which are also at the default — cannot see it either. Mitigation: D20
  makes a **non-default-factor** scenario part of the same subtask that re-points the call sites,
  **one per production call site rather than one per adopter**, each asserting the delay its own
  production path produces at a factor that is not `2`; that is the only instrument in this design
  that separates "reads the configured field" from "reads the default" — `[derived → AC41, and
  subtask 15's non-default-factor scenarios]`.
- **Counting those scenarios per adopter instead of per call site reopens the same hole one function
  to the side.** `internal/scheduler` writes a persisted `run_at` from two separate production
  functions, each reading `cfg` for itself
  `[measured ef69c56:internal/scheduler/settle.go:133,143,222,243 · `grep -n
  'backoff\.Exponential\|^func ' internal/scheduler/settle.go` → the call expression at `143` inside
  `deferredFailedStatement` declared at `133`, and the same expression at `243` inside `settleFailed`
  declared at `222`]`, so a single scheduler scenario leaves the uncovered one free to keep passing
  `backoff.DefaultFactor` with the whole suite green and the defect in persisted data. Mitigation:
  the scheduler takes **two** scenarios, one per settlement path, landing beside the two literal
  one-based ramps D2 already built for the same reason — `[derived → AC41, and subtask 15's two
  scheduler scenarios]`.
- **A comment or a `t.Errorf` string that narrates the deleted loop survives every gate in
  `make verify`.** `Exponential`'s own doc comment states the shape as a doubling clamped "before the
  doubling", `internal/backoff/backoff_test.go` says "no doubling, no lower clamp", and
  `internal/tg/retry_test.go` attributes two branches to line ranges of a loop this rewrite removes
  `[measured ef69c56:internal/backoff/backoff.go:37, internal/backoff/backoff_test.go:89,124,142 and
  internal/tg/retry_test.go:709,712,716-721,729 · `grep -rn
  'doubling\|base<<\|in-loop\|post-loop\|POST-LOOP\|IN-LOOP\|line 5' internal/backoff/
  internal/tg/retry_test.go` → those lines among its hits, each carrying a doubling or a
  loop-structure claim; the same sweep also surfaces `internal/tg/retry_test.go:126`, whose
  `(base/2, doubling)` describes what that test's own default-factor rows compute and therefore
  stays as it is]`. None
  of it compiles differently and none of it prints on a green run, and the recurrence is on record:
  self-review R1-8 already cost a round for false branch-attribution prose in that same file.
  Mitigation: D2's three-class rule is restated in row 15 with the decision that separates a stale
  *contract* claim from a still-true *default-factor row* claim, and D20's closing checklist names
  the sites — `[derived → subtask 15's three-class rule and D20's closing checklist]`.
- **Moving the arithmetic from integer doubling to `math.Pow` could move a pinned value, inside a
  change billed as behaviour-preserving.** Mitigation: measured rather than argued — over a grid of
  bases from `1ns` to `5m`, ceilings from `1ns` to `1h` and attempts from `-1000` to `1000`, the float
  form at `factor = 2` returns the shipped integer form's value in every row
  `[measured fc5e6dd · a probe module under tmp/ carrying a verbatim copy of the shipped Exponential
  and the float form D20 specifies · `go run .` → `factor=2 grid: mismatches=0`]`, and the two forms
  diverge only for a base above `2^53` nanoseconds — past a hundred days, unreachable from any
  default — and there by at most a few nanoseconds `[measured fc5e6dd · the same probe · `go run .` →
  `worst absolute deviation=128ns … at attempt=7 base=9007199254740993`]`. **That residue is
  recorded, not closed**, because AC40 quantifies over every attempt while the grid does not: such a
  base is reachable by configuration even though no default approaches it, so D20 states the bound
  AC40's exact-equality reading is discharged over, and subtask 15's behaviour-preservation rows are
  held below it — a red above the bound is a scope-boundary item, never a tolerance to widen
  `[derived → AC40, and D20's behaviour-preservation measurement]`.
- **A `time.Duration` conversion of an out-of-range or `NaN` float is a silent negative, not a
  crash**, so a test case that merely *ran* the function would pass against the very defect the
  clamp exists to prevent `[measured fc5e6dd · the same probe · `go run .` →
  `time.Duration(math.Pow(1.3,1000)) = -2562047h47m16.854775808s` and `time.Duration(math.NaN()) =
  -2562047h47m16.854775808s`]`. Mitigation: D20 puts the clamp immediately before the conversion and
  guards `NaN` explicitly, and subtask 15's overflow rows assert **the value** — exactly `ceiling`,
  and strictly positive — never "it did not panic" — `[derived → subtask 15's overflow rows]`.
- **Adding a validated field to the adopters' option structs reds their invalid-config tables, and
  reds them misleadingly if the new check runs first.** `internal/ingest`'s table builds each row's
  `config.Ingest` as a full literal omitting exactly one field and asserts the error names that field
  `[measured fc5e6dd:internal/ingest/loop_test.go:477-493 · `sed -n '477,493p'
  internal/ingest/loop_test.go` → rows such as `{"zero_retry_base_delay", Options{… Config:
  config.Ingest{PollInterval: …, LongPollTimeout: …, BatchLimit: 1, RetryMaxAttempts: 1, RetryMaxDelay:
  …}}, "Config.RetryBaseDelay"}`]`, so with the factor checked first every one of them would fail
  naming the factor instead. Mitigation: D20 puts each constructor's factor check **last** in its
  existing chain and subtask 15 gives every baseline literal a legal factor — `[derived → subtask
  15]`.
- **`strconv.ParseFloat` accepts `"NaN"`, `"Inf"`, `"+inf"` and `"infinity"`**, so a reader that only
  checked "parses" would admit `NaN` into the ramp and one that only checked "greater than 1" would
  admit `+Inf` `[measured fc5e6dd · the same probe · `go run .` → `ParseFloat("NaN") = NaN,
  err=<nil>`, `ParseFloat("Inf") = +Inf, err=<nil>`, `ParseFloat("+inf") = +Inf, err=<nil>`]`, and
  the spec's *Technical constraints* item 10 re-derived the same fact independently, `"infinity"`
  included. Mitigation: `ValidFactor` is finite-and-greater-than-one
  by definition and is the single predicate every reader and constructor calls; subtask 14 carries
  every one of those spellings as an invalid row, which is why the spec splits the refusal into AC42
  and AC43 rather than stating it once — `[derived → AC42, AC43, and subtask 14's invalid rows]`.
- **KD-31 states the doubling as a property, in the signatures, in the `min(base·2^i, ceiling)` shape
  and in the clamp's own wording; KD-27 and `ai-docs/context.md` state each scope's tuning-key set by
  count.** All of those become false the moment subtask 15 lands
  `[measured fc5e6dd:ai-docs/key-decisions.md:83 · `grep -n 'KD-31' ai-docs/key-decisions.md` → the
  entry carrying `min(base·2^i, ceiling)` and `clamps at the ceiling *before* the doubling`]`.
  Mitigation: subtask 16 owns each of them, and D20's closing paragraph names every claim and what it
  becomes, so the propagation is a checklist rather than a re-derivation — `[derived → subtask 16]`.

## Test Design

`internal/ingest`'s database-backed cases take a fresh schema from `testdb.Schema(t)` and apply
`store.Migrate`, and its Bot API cases point `tg.New` at a `tgtest.Server` through
`Options.HTTPClient`, `tgtest.BaseURL` and `tgtest.Token` — no mock of this package's own making
anywhere `[derived → AC29]`.

**Subtask 1 — `internal/backoff`** (`internal/backoff/backoff_test.go`)
- Entry points: `Exponential`, `EqualJitter`.
- Scenarios: an exact table of `(attempt, base, ceiling) → delay` including the zeroth attempt, the
  attempt at which the ceiling is first reached, and every attempt past it; strict positivity and
  non-decrease across a run of attempts; the ceiling never exceeded; a negative attempt clamped to
  the zeroth; **a `base` already greater than `ceiling`, at the zeroth attempt, returning `ceiling`
  exactly** — the row that makes this table cover the adopters' full input domain rather than leaving
  that half to subtask 2, because `internal/tg` ships an assertion on precisely that input and
  `internal/scheduler`'s ramp answers it at its own post-loop clamp (D2); `EqualJitter` bounded
  within `[d/2, d)` for a jitter stub returning 0 and one returning a value just under 1, its
  `base > ceiling` counterpart at `ceiling/2`, and its monotonicity for a fixed jitter
  `[derived → AC23 and D2's `base > ceiling` contract clause]`.
- **Overflow cases, the ones D2's contract exists for.** Explicit table rows at attempts far past any
  a naive `base << attempt` survives — `64` (where a shift of a nanosecond base has already consumed
  `int64`'s range) and `1000` (where no shift is even defined) — asserting for each that the result
  is **exactly** `ceiling` and **strictly positive**, and the same pair for `EqualJitter` asserting
  the result stays within `[ceiling/2, ceiling)`. The assertions are on the value, never on "it did
  not panic": a wrapped `time.Duration` is a silent negative, not a crash, so a case that only ran
  the function would pass against the defect it was written to catch. Repeated for a base at the
  small end (`1ns`, where doubling reaches the wrap soonest) and at an operationally realistic one
  (`1s`) `[derived → AC23 and D2's clamp-before-overflow contract]`.
- **The out-of-domain rows D2 decides, so the answer is pinned rather than discovered.** `base <= 0`
  returns `base` unchanged — asserted at `0` and at a negative base, at the zeroth attempt and at a
  large one, which is what pins "no doubling and no lower clamp" rather than merely "not positive" —
  and, for a positive `base`, `ceiling <= 0` returns `ceiling`; `EqualJitter` answers each with its
  equal-jitter half of the same value. Neither input is reachable from any adopter (D2), so these
  rows exist to stop an implementor reading the in-domain contract as licence to clamp from below
  `[derived → AC23 and D2's out-of-domain rows]`.
- Fixtures: a deterministic `jitter func() float64` stub. No database, no bubble.

**Subtask 2 — the adoption** (no new test file; the shipped test files listed in § Decomposition
row 2 are edited)
- Entry points: unchanged — the production behaviour under test is `internal/tg`'s retry wait and
  `internal/scheduler`'s persisted `run_at`, now computed through `internal/backoff`.
- The first half of the gate, stated as what a delete-and-re-point can achieve: in
  `internal/tg/retry_test.go`, `internal/scheduler/cadence_test.go`,
  `internal/scheduler/failure_test.go` and `internal/scheduler/deadline_test.go`, **every
  *pre-existing* assertion and expected value is byte-identical to the shipped one; the call
  expression is re-pointed and its argument may carry the one-based→zero-based translation**; the
  suites are then green. "Passing unchanged" is *not* the gate and
  must not be recorded as one — after the local ramps are deleted those files do not compile until
  their call expressions move, so a test that has not been edited has not passed `[derived → AC30]`.
- **`cadence_test.go`'s ramp tests, and what becomes of them.**
  `TestBackoff_exactTable` and `TestBackoff_strictlyGrowingUntilCeiling` are re-pointed at
  `backoff.Exponential` with the translation in their *arguments*; their one-based `cases` table,
  every expected duration and every asserted bound are untouched, and after the adoption they pin
  `internal/scheduler`'s one-based `failures → delay` mapping with no database. Leaving the argument
  un-translated is not an option the byte-identical rule permits — it reds the table, which D2
  measures — and re-indexing the table instead of the argument is a scope-boundary item to surface
  `[derived → AC30 and D2's per-file disposition]`.
- **The second half — a new assertion, and the one that can go red.** New test text in
  `internal/scheduler/failure_test.go` and `internal/scheduler/deadline_test.go`: the persisted
  `run_at` bracket is computed from a **literal one-based ramp table** indexed by the loop's
  `attempt`, containing no call to `backoff`, to `backoff.Exponential`, or to any shift or doubling
  expression. It lands and is green **against the shipped one-based ramp** before the re-point, and
  it is the assertion an omitted one-based→zero-based translation reds. **It substitutes for the
  `backoff(attempt, …)`-derived expected value at those brackets, and that is the sole carve-out from
  the expected-value half of the byte-identical rule above** — not a real exception, because the
  literal durations it writes are the ones the shipped ramp already computes, which is what its green
  run against the shipped ramp establishes before the re-point; from that point on it is itself a
  pre-existing expected value the re-point must leave byte-identical. Nowhere else in these files
  does new assertion text land `[derived → AC30 and D2's call-site gate]`.
- **The instrument is proven before it is trusted.** With the literal ramp in place and the re-point
  not yet applied, the wrong delay is written at both `settle.go` sites over a `cp` backup as
  `backoff(k+1, …)` — the omitted translation's arithmetic in the shipped ramp's own units, and the
  form that compiles while `func backoff` still stands (D2) — and
  `go test ./internal/scheduler/ -run
  'TestFailurePolicy_oneShotAttemptsGrowAndGiveUp|TestDeadline_successiveBreaches_growingDelay'`
  must report **FAIL**; the
  backup is then copied back (never `git checkout -- <file>`, which discards every other uncommitted
  edit to the file). A probe that passes means the gate does not discriminate and is a STOP for the
  orchestrator, not a result to reason past — `AGENTS.md` § Patterns 2 `[derived → D2's mutation
  probe]`.
- **What the `internal/backoff` table of subtask 1 does *not* cover, said plainly so no later reader
  re-derives the false version.** That table pins the shared function's own contract in zero-based
  units, independently of either adopter — and says nothing about *which argument a call site
  passes*. Once `cadence_test.go`'s argument carries the translation, the shipped scheduler suite
  returns the same green whether or not `settle.go`'s does `[measured 07e5177 · D2's scratch-tree
  runs · `go test ./internal/scheduler/` → `ok  	github.com/maratik123/lab-game/internal/scheduler`
  with the call-site translation present, and the same `ok` with it dropped]`; and subtask 1's own
  table, which pins `Exponential` in zero-based units and never reaches `settle.go`, cannot separate
  them either `[derived → subtask 1's table]`. The call-site gate above is the only instrument in
  this design that does `[derived → AC23, AC30]`.

**Subtask 3 — `config.Ingest`** (`internal/config/ingest_test.go`)
- Entry points: `Load`, and the package's existing disjointness and required-variable suites.
- Scenarios: every ingest key absent takes its documented default; each key present and valid is
  parsed; each key present and malformed produces a `*KeyError` naming that variable and wrapping
  `ErrInvalidValue`; a batch limit outside the Bot API's accepted range is rejected; a long-poll
  timeout at or above `AttemptTimeout` is rejected naming the long-poll variable, and one below it is
  accepted; **a long-poll timeout that is not a whole number of seconds — `25500ms`, and `1500ms` as
  a second value below any plausible `AttemptTimeout` so the case cannot pass for the neighbouring
  check's reason — is rejected with a `*KeyError` naming `LAB_GAME_INGEST_LONG_POLL_TIMEOUT`, while
  `25s` is accepted** (D16: `Load` owns this check, the loop never truncates); the recorded-lookup
  disjointness suite and the `.env.example` set-equality suite still
  pass with the new keys `[derived → AC27]`.
- Fixtures: the package's existing map-backed `Lookup`; no process environment is touched.

**Subtask 4 — the migration** (`internal/store/migrate_test.go`, `internal/store/schema_test.go`)
- Entry point: `store.Migrate` against a fresh schema.
- Scenarios: the shipped exact base-table assertion — which compares the whole set with
  `slices.Equal`, so it fails on any addition until extended — is extended with `ingest_offset` and
  `ingest_dead_update`, and nothing else about it changes; a second `Migrate` is a no-op; the offset
  row exists exactly once after migration and a second row is refused by the singleton `CHECK`; a
  give-up row with an empty kind is refused; the migration file carries no `-- +goose Down` section
  and the shipped migration files' contents are unchanged `[derived → AC7]`.
- This subtask's group must leave `internal/store` green on its own — its consumer lands in a later
  group — so the expectation edit belongs here, in the same step as the `.sql` file, not with the
  code that reads the tables `[derived → AC30]`.
- Fixtures: `testdb.Schema(t)`, `slog.New(slog.DiscardHandler)`.

**Subtask 5 — `store.PlayerExists`** (`internal/store/owner_test.go`)
- Entry point: `PlayerExists(ctx, q, telegramID)`.
- Scenarios: a `kind = 'player'` owner with that `telegram_id` reports true; a `kind = 'chat'` owner
  with the *same* `telegram_id` reports false — the pair, not the column, is the predicate; an id
  with no owner row reports false; a closed pool surfaces the error rather than a false; the same
  call through a `pgx.Tx` and through a `*pgxpool.Pool` behaves identically `[derived → AC28, AC33]`.
- **The `queryRower` re-point is gated by compilation, not by a new test.** `post_test.go`'s
  `balanceOf` consumes it, so deleting the near-duplicate and re-pointing every consumer at
  `store.Queryer` either compiles or does not; the package's shipped posting tests are what prove the
  helper still reads the same balances, and — as in subtask 2 — **every assertion and expected value
  in them stays byte-identical, only the parameter type changes**. `go test ./internal/store/` is run
  as its own step before the group returns `[derived → AC30]`.
- Fixtures: `testdb.Schema(t)`, `store.CreateOwner` for both kinds.

**Subtask 6 — kinds, the grammar, the router** (`internal/ingest/kind_test.go`,
`operation_test.go`, `router_test.go`)
- Entry points: the kind derivation, the unexported `operation_id` builder, `NewRouter`, `Kinds`,
  and the router's lookup.
- Scenarios (kind): a table case per row of D4's table, each built by setting only that payload field
  on a `telego.Update` and asserting the derived kind, its date extraction (present or absent) and
  its chat-id extraction (present or absent); an update with no payload field set derives the zero
  kind `[derived → AC4, AC18]`.
- Scenarios (grammar): the `update_id` space and the `callback_query.id` space produce different
  values from the *same* raw identifier; a
  derived value always carries a non-empty space token, the separator and a non-empty identifier; an
  empty space or an empty identifier is refused; no space token contains the separator
  `[derived → AC13, AC14]`.
- Scenarios (router): registering a duplicate kind is refused; registering a kind with no table row
  is refused; `Kinds` returns the registered set in a deterministic order; a lookup for an
  unregistered kind reports absence `[derived → AC4]`.
- Fixtures: hand-built `telego.Update` values; no database.

**Subtask 7 — offset and give-up storage** (`internal/ingest/offset_test.go`, `dead_test.go`,
`main_test.go`)
- Entry points: the offset read, the guarded advance, `DeadUpdates`.
- Scenarios: a fresh schema reads the seeded offset; an advance to a higher value is stored and read
  back by a *new* reader (no in-process value is the sole record); an advance to a value at or below
  the stored one is a no-op; a rolled-back advance leaves the stored value untouched; `DeadUpdates`
  returns rows in a deterministic order under a caller-owned transaction, honours its limit, and
  neither commits nor rolls back `[derived → AC6, AC25, AC37]`.
- Fixtures: `testdb.Schema(t)`, `store.Migrate`, and the package's `TestMain` over `testdb.Main`,
  authored here because this is the first subtask to take a schema and inherited by every
  database-backed scenario subtasks 9 and 10 add `[derived → subtask 7's offset scenarios]`.

**Subtask 8 — the observation seam** (`internal/ingest/observe_test.go`)
- Entry points: the nil-checked report helpers and each observation type's rendering.
- Scenarios: a nil observer is not called; each `Outcome` renders a distinct name and an
  out-of-range value renders the fallback; a lag-absent observation is distinguishable from a
  zero-lag one `[derived → AC18, AC19]`.
- Fixtures: a recording observer collecting observations under a mutex, reused by subtasks 9 and 10.

**Subtask 9 — the loop** (`internal/ingest/loop_test.go`, `retry_test.go`)
- Entry points: `New`, `PollOnce`, `Run`.
- Scenarios:
  - *Request shape.* Every `getUpdates` the loop issues carries an `allowed_updates` field, with an
    empty router and with a populated one; every registered kind appears in it; and the long-poll
    timeout is transmitted on every cycle, the first included — it is a positive value by
    construction (D15's default, D16's checks, and `New`'s refusal of a non-positive tuning field),
    so `omitempty` never reaches it. **The offset is asserted present only for a non-zero stored
    value**, because `GetUpdatesParams.Offset` carries `json:"offset,omitempty"`
    [measured c37e642:telego@v1.11.2/methods.go:18 · `grep -n 'json:"offset'
    $(go env GOMODCACHE)/github.com/mymmrac/telego@v1.11.2/methods.go` →
    `18:	Offset int \`json:"offset,omitempty"\`` in `GetUpdatesParams`] and `encoding/json` erases a
    zero under it — the same erasure D3 turns on for `allowed_updates`. At D14's seeded `0` the field
    is legitimately **absent**, which is what § Risks' first-start row rests on, so the first cycle's
    assertion is that no `offset` is transmitted and the *transmitted offset after a settlement*
    scenario below covers the non-zero case. Asserted by decoding the request body the `tgtest`
    handler receives `[derived → AC3, AC4, AC5]`.
  - *Happy path.* A routed update reaches its handler with the loop's transaction and the derived
    operation key; the handler's writes survive; the offset advances past it; the observer sees a
    handled outcome with a duration `[derived → AC8, AC17]`.
  - *The transmitted offset after a settlement — D14's persisted semantic, pinned on the wire.* After
    an update with a known `update_id` is settled, the **next** `getUpdates` request body carries
    `offset` equal to that `update_id + 1`, decoded from the request the `tgtest` handler received;
    the same assertion is made for each settled outcome the loop has — handled, duplicate, unrouted
    and given-up — since each of them advances. This is the scenario that fails if the column's meaning
    and the transmitted value disagree, which is the live-lock D14 refuses: storing the raw
    `update_id` and transmitting it re-fetches the same update forever while the guarded advance
    silently rejects every re-write, and the only other symptom is the idempotency-hit counter
    §13.2 puts on the health surface — where duplicates are ordinary traffic — with no exporter
    reading it until #23. The poll-failure scenario below asserts the offset is
    *unchanged* across failing cycles, which passes under either semantic, so it does not reach this
    `[derived → AC6, AC25 and D14]`.
  - *Unrouted.* An update whose kind has no route leaves the database unchanged, is reported as
    unrouted, advances the offset, and does not stop the loop `[derived → AC9]`.
  - *Duplicate.* A handler that posts under a `store.PlayerOperation` twice for the same update
    produces exactly one `player_operation`, one `journal_entry` and one `posting` row; the second
    delivery is reported as an idempotency hit, consumes no retry attempt, and advances the offset —
    including when the handler wraps the sentinel with `%w` `[derived → AC12, AC22]`.
  - *Failure and retry.* A handler failing every attempt is retried in a transaction of its own each
    time, with a strictly positive and non-shrinking delay, up to the configured cap; after each
    failed attempt the database holds nothing that attempt wrote; after the cap the loop writes
    exactly one give-up row carrying the identity, kind, chat id, attempt count and last error, keeps
    no raw payload anywhere, advances the offset and continues polling `[derived → AC23, AC24,
    AC36]`.
  - *Panic.* A handler that panics does not terminate the test process, leaves no row behind, is
    reported distinctly from a returned error, and is retried under the same cap `[derived → AC10]`.
  - *Poll failure — D19's claims, asserted separately.* A `tgtest` handler answering `getUpdates`
    with a 5xx for the first N cycles and then with a normal batch, driven through `Run`. The
    assertions are: `Run` **does not exit** while the server is failing (it is still running when the
    handler starts succeeding, and the update from the first successful cycle is handled); it **does
    not busy-loop** — the failing cycles are counted at the `tgtest` handler and their count over a
    window is consistent with the configured poll interval rather than with an unbounded spin, which
    is the assertion that fails on a `continue` with no wait; the **offset does not advance** across
    the failing cycles (read from the row, and the first successful `getUpdates` still carries the
    pre-failure offset — the claim D19 rests on, since a failed poll settles nothing); and the error
    **reaches the observer** through `LoopObservation.Err`, once per failed cycle. Plus the direct
    primitive: `PollOnce` on a failing server returns the error to its caller `[derived → AC17 and
    D19]`.
    - The interval assertion needs a poll interval large enough that a spin is distinguishable from
      the configured cadence and small enough not to slow the suite; `config.Ingest`'s
      millisecond-scale test values give that, and the assertion is a floor on elapsed time across a
      known number of failing cycles rather than an equality on a duration — a wall-clock equality
      here would be the flake `internal/scheduler`'s timing tests already avoid.
  - *Cancellation.* Cancelling the loop's context mid-retry leaves the update unsettled with the
    offset behind it; a long poll in flight when the context is cancelled returns promptly and `Run`
    returns without a goroutine surviving the test; `Run`'s returned error is `ctx.Err()`, and
    cancellation is the **only** input under which `Run` returns non-nil — the poll-failure scenario
    above is the negative half of that same claim `[derived → AC25, AC26]`.
  - *Option validation.* A nil client, a nil pool, a nil router or a non-positive tuning field is
    refused by `New` with an error naming the field `[derived → AC27]`.
- Fixtures: `tgtest.Server` scripting `getUpdates` responses (a helper marshalling a `[]telego.Update`
  into `tgtest.Success`), a handler stub whose behaviour per call is table-driven (succeed, fail,
  panic, post twice), the recording observer of subtask 8, `testdb.Schema(t)` with `store.Migrate`,
  and a `config.Ingest` with millisecond-scale retry delays. No `synctest` bubble in the
  database-backed cases (see § Risks).

**Subtask 10 — the gate** (`internal/ingest/gate_test.go`)
- Entry points: `NewGate`, `Gate.AllowCall`, and the same gate installed into a real `tg.Client`.
- Scenarios: `ChatNone` allowed; `ChatUnknown` refused; a `ChatKnown` id inside the allowlist
  allowed; one outside it allowed when a `kind = 'player'` owner row carries that `telegram_id`;
  one outside it refused when only a `kind = 'chat'` owner row carries it; a token that parses as no
  integer refused; a lookup that errors refuses; a destination refused for want of a player row is
  allowed once the row exists, within the same gate instance and with no restart; a positive result
  is served from the cache without a second lookup (counted through a lookup stub)
  `[derived → AC15, AC28, AC33, AC34, AC35]`.
- **The uncommitted-owner-row scenario — D18's behaviour, pinned rather than incidental.** Against a
  real schema and the pool-backed `PlayerLookup`: open a transaction, `store.CreateOwner` a
  `kind = 'player'` row with a `telegram_id` absent from `AllowedChatIDs` — the shipped constructor
  already takes the transaction, so the fixture needs nothing new
  [measured c221784:internal/store/owner.go:41 · `grep -n '^func CreateOwner' internal/store/owner.go`
  → `41:func CreateOwner(ctx context.Context, tx pgx.Tx, kind OwnerKind, telegramID *int64) (Owner,
  error) {`] — and, **without committing**, call `AllowCall` for that destination on a gate holding
  the pool. The call is
  **refused**. Then commit, and a second `AllowCall` for the same destination is **allowed** with no
  restart and no new gate. The pair is what makes the scenario an assertion about visibility rather
  than about the allowlist: the same id, the same gate, the same pool, and the only variable is the
  commit. It also proves the refusal left nothing negative in the cache, which is AC35's requirement
  read from the other side. A comment on the test names D18, so a later reader meets the rule rather
  than the symptom `[derived → AC34, AC35 and D18]`.
- Concurrency scenario, run under `go test -race`: many goroutines call `AllowCall` on one gate at
  once — a mix of ids that the stub allows and ids it refuses, including several goroutines racing
  on the *same* not-yet-cached allowed id — and every call returns the answer the stub's fixed
  answers imply, with no data race reported and no panic. This is the scenario that would fail on a
  bare `map[int64]bool`, so it is the one that makes D10's mutex load-bearing rather than
  decorative; the same-id racers additionally pin that a duplicate first lookup is benign (both
  writers write the same value) `[derived → AC35 and D10's cache shape]`.
- Integration scenario: with the gate installed at `tg.New`, a refused outbound call reaches the
  `tgtest` server not at all and is reported through `tg.Observation` with no attempt recorded, and a
  subsequent allowed call into the same chat and method class is not delayed by a limiter window the
  refusal would have spent `[derived → AC16]`. This is AC38 by **example** — one refused call, one
  server that never saw it. AC38's **structural** claim, that no production path can skip the gate at
  all, is not an example's to make and is discharged in subtask 11 `[derived → AC38]`.
- Fixtures: a `PlayerLookup` stub (fixed answers, an error mode, and a call counter), the pool-backed
  implementation over `testdb.Schema(t)`, `tgtest.Server`, `config.Transport` defaults.

**Subtask 11 — guards** (`internal/ingest/guards_test.go`)
- Entry points: a source walk over the package's non-test files, and a reflection walk over
  `telego.Update`.
- Scenarios: no `panic(`, `log.Fatal` or `os.Exit` appears in the package's non-test source; no
  metrics library is imported; every exported pointer field of `telego.Update` has its json tag
  either in D4's kind table or in a named exemption set, and every table row's token is a tag some
  field declares; the event-type dictionary migration and its Go mirror are unchanged by this task;
  `internal/store`'s posting entry points are unchanged `[derived → AC11, AC19, AC20, AC21]`.
- **AC8's structural half, which the happy path of subtask 9 does not reach.** That scenario shows
  that *a* handler receives the loop's transaction; AC8's second clause is the wider claim that **no
  exported entry point of the package lets a handler obtain a transaction the loop does not own**.
  That is a structural property an example cannot make — the same shape as AC38's structural half
  below, and it gets the same discharge here rather than a scenario. The `go/ast` walk over the
  package's non-test files asserts that no exported function or method of `internal/ingest`
  **returns** a `pgx.Tx` or a `pgx.Conn`, and that no exported type declares an exported field of
  either type — so the only transaction a handler can reach is the one the loop passes it as the
  `Handler` parameter. The walk distinguishes **parameters** from results and fields, and the two
  shapes that distinction deliberately leaves legal are named in the test so a later reader cannot
  relax the guard by re-deriving them: `DeadUpdates`, which *takes* a caller-owned `pgx.Tx` (AC37) —
  a consumer of a transaction, never a source of one — and `Options.Pool`, the `*pgxpool.Pool` the
  constructor is handed, which is a connection source rather than a transaction and is reachable
  only at wiring time, not from inside a handler. An exported accessor added later that hands the
  loop's `tx` out reds the suite `[derived → AC8]`.
- **AC38's structural half, which the integration scenario of subtask 10 does not reach.** That
  scenario proves *a* refused call never reaches the server; AC38 is the wider claim that no
  production path outside `internal/tg` can issue an outbound call at all without passing the gate.
  It splits in two, and both halves are discharged structurally rather than by example. The
  `internal/tg` half is **already shipped** and this task adds nothing to that package
  [measured c221784:internal/tg/guards_test.go:216 · `grep -n
  'func TestGuard_RefusingGateBlocksTheAccessor' internal/tg/guards_test.go` →
  `216:func TestGuard_RefusingGateBlocksTheAccessor(t *testing.T) {`], so AC38 points at that guard
  by name rather than re-proving it here. The `internal/ingest` half is this subtask's: a source walk
  asserting the package's non-test files construct no `telego.Bot`, no `http.Client` and no
  `telegoapi` caller of their own, and reach the Bot API only through the `*telego.Bot` that
  `tg.Client.API()` returns — which is AC3's structure and AC38's second half in one walk
  `[derived → AC3, AC38]`.
- Additional scenario, because no enabled linter covers it: no type declared in `internal/ingest` has
  a `context.Context` field, and **every exported method of the package takes `ctx context.Context`
  as its first parameter unless it is listed in a named, commented exemption set in the test file
  itself** — asserted by a `go/ast` walk over the package's non-test files, since `containedctx` is
  not among the enabled linters
  [measured c221784:.golangci.yml:12-37 · `grep -n 'enable:\|containedctx\|bodyclose\|whitespace'
  .golangci.yml` → `12:  enable:`, `14:    - bodyclose`, `37:    - whitespace`, `60:  enable:` (the
  formatters block), and no `containedctx` line] `[derived → AC2]`.
  - **The exemption set is what makes the guard decidable, and the wording is deliberate.** AC2's own
    predicate — "every exported method **that reaches the network or the database**" — is not
    computable from an AST: reachability is a whole-program property, so a walk written to AC2's
    literal words either over-approximates (failing on a legitimate ctx-less accessor such as
    `Router.Kinds`) or gets quietly relaxed at implementation time into something that proves less
    than AC2. Inverting it fixes that: the walk demands `ctx` first from **every** exported method,
    and the only escape is an entry in an exemption set declared in the test file with a comment
    saying why that method reaches neither. The guard is then total and mechanical, the
    over-approximation is discharged once per exemption instead of silently, and an added ctx-less
    method reds the suite until someone writes the reason down — which is strictly stronger than
    AC2's literal reading, since a method that *does* reach the database can only pass by someone
    entering a false justification into the diff a reviewer reads `[derived → AC2]`.
  - **The riding context is part of the same walk, because a `context.Context` field is not the only
    way to hold one.** A `telego.Update` embedded in an exported type of this package would satisfy
    the no-`context.Context`-field clause and still promote `Context()` and `WithContext()` onto that
    type, handing a handler the `context.Background()` D9 measures. The walk therefore also asserts
    that no type declared in `internal/ingest` embeds `telego.Update` — the raw update is a named
    field — and that the package's non-test source contains no selector named `Context` or
    `WithContext`, under the same named-exemption escape as above, so a later legitimate use is
    written down rather than silently allowed
    `[derived → AC2, D9 and spec *Technical constraints* item 5]`.
- Fixtures: the repository-root resolver `internal/tg/guards_test.go` already establishes for this
  shape. These walks need no database, and the package's `TestMain` is subtask 7's.

**Cross-cutting — the gates, not a test file**
- AC1's doc-comment and package-comment obligations are held by `revive`'s `exported` and
  `package-comments` rules, which `make verify` runs
  [measured 1fce8b5:.golangci.yml:45-48 · `grep -n 'revive\|- name: exported\|- name:
  package-comments' .golangci.yml` → `35:    - revive             # incl. exported-symbol doc
  comments`, `45:    revive:`, `47:        - name: exported`, `48:        - name:
  package-comments`] `[derived → AC1]`.
- AC30 is `make verify` in full, and AC31 is the coverage ratchet the pre-commit hook and CI both
  run against the recorded value; new statements this task adds are covered by the suites above
  `[derived → AC30, AC31]`.
- AC32 is subtask 12's whole job, and the sweep is `AGENTS.md` § Propagation Rule step 4's — a
  case-insensitive `grep -rni` over `.claude/`, `AGENTS.md` and `ai-docs/` for every claim this diff
  falsifies, plus the repo-root user-facing docs, not only the members the spec names
  `[derived → AC32]`.

**Subtask 13 — the shared default and the shared boundary** (`internal/backoff/backoff_test.go`)
- Entry points: `DefaultFactor`, `ValidFactor`.
- Scenarios: `DefaultFactor` is exactly `2`, pinned as an assertion of its own — every
  behaviour-preserving claim in this amendment rests on that value and nothing else in the suite
  would notice it moving; `ValidFactor` true for `2`, for `1.3`, for the smallest float64 strictly
  above `1` and for a large finite factor, and false for exactly `1`, for a value just below `1`, for
  `0`, for a negative, for `NaN`, for `+Inf` and for `-Inf`. Exact booleans and an exact constant —
  no tolerance is involved anywhere in this row. **No monotonicity case belongs here**: the ramp's
  arithmetic does not move until subtask 15, which is where the non-decrease case is re-run at a
  factor that is not the default `[derived → AC40's single named constant, and the shared boundary
  AC42 and AC43 are each checked against]`.
- Fixtures: none.

**Subtask 14 — the configuration surface** (`internal/config/transport_test.go`,
`internal/config/scheduler_test.go`, `internal/config/ingest_test.go`)
- Entry points: `loadTransport`, `loadScheduler` and `loadIngest` through their existing table
  suites, and `Load` through the shipped set-equality suites.
- Scenarios, per scope: the key absent → the default, asserted **equal to `backoff.DefaultFactor`**
  rather than to a re-typed `2`, so the config default and the shared constant cannot drift apart;
  the key present and valid (`1.3`) → parsed onto `RetryFactor`; the key present and invalid — `1`,
  a value just below `1`, `0`, a negative, `NaN`, `Inf`, `infinity`, a non-numeric string and an
  empty value → a `*KeyError` naming that key, matched the way the shipped duration rows are
  matched. The `NaN` row and the infinity spellings are what AC43 exists for, and the `1`/below-`1`
  rows are AC42's: a reader passing only one of the two clauses passes one group and reds the other,
  which is the discrimination the spec's *Technical constraints* item 10 asks for. Beside them, one
  `Load`-level case that sets **one** scope's key and asserts the other two scopes' `RetryFactor`
  still equal `backoff.DefaultFactor` — the isolation half of AC41, which is a property of the
  per-scope key rather than of any ramp and is therefore cheapest to pin here. The disjointness and
  `.env.example` set-equality suites need no new case: they are what fails, by name and in both
  directions, if either side of a new key is missing `[derived → AC39, AC41's isolation clause, AC42
  and AC43]`.
- Fixtures: the existing recording `Lookup` stubs.

**Subtask 15 — the factor parameter, and the gate that it reaches the call sites**
- Entry points: `backoff.Exponential`, `backoff.EqualJitter`, and each adopter's own production path.
- **`internal/backoff`, behaviour preservation — and the base bound every row of this class sits
  under.** Every shipped row re-run with a literal `2` in the
  new argument and its expected value byte-identical — a literal, not `DefaultFactor`, so that a
  later move of the default cannot silently carry a pinned table with it. Beside it, one case whose
  expectation is an independent **integer** `base << k` for the attempts where that shift stays below
  the ceiling: exact, not the implementation's own formula, and red if the factor argument stops
  meaning what it means. **Its base, and every base in this class, stays below `2^53` nanoseconds** —
  the range D20 measures the float form and the shipped integer form identical over, and therefore
  the range AC40's exact-equality reading is discharged over. The bound is written into the test
  rather than left to whoever picks the numbers, because the failure it prevents is a misread red:
  above it the two forms diverge by the bounded amount D20 records, so an exact `base << k` row built
  on such a base would fail for a float64 rounding artefact and look like a defect in the amendment.
  A row above the bound is a scope-boundary item for the orchestrator, not a tolerance to widen
  `[derived → AC40, and D20's behaviour-preservation measurement]`.
- **`internal/backoff`, the new domain rows:** `factor = +Inf` → `base` at the zeroth attempt and
  `ceiling` from the first on; the out-of-domain factors D20's table decides — exactly `1`, below
  `1`, and `NaN` → `base` at every attempt, clamped by the ceiling, asserted at the zeroth attempt
  and at a large one, which is what pins "treated as `1`, with no lower clamp" rather than merely
  "not growing"; and D2's existing out-of-domain rows re-asserted **at a non-default factor**, so the
  amendment's own claim that they keep their answers is checked rather than assumed. All exact
  `[derived → D20's contract table]`.
- **`internal/backoff`, the growth clause — asserted as non-decrease, and deliberately not as strict
  increase.** Subtask 1's monotonicity case is re-run at a factor that is not the default and at a
  factor barely above `1`, asserting only that the delay never shrinks and never exceeds the ceiling.
  A strict-increase assertion is **forbidden here**, and the reason is named so a later reader does
  not "tighten" it: it would pass at every default this tree ships while encoding a claim D20's
  contract table refutes at config-reachable inputs — a small base with a near-`1` factor stands
  still for many attempts because the conversion truncates to a nanosecond. Beside it, one case that
  pins the truncation edge itself: at a base and factor whose step is below a nanosecond, two
  consecutive attempts return the **same** delay, asserted as equality — the row that would red if an
  implementor rounded instead of truncating, or added a lower clamp this package does not have
  `[derived → D20's contract table's first row]`. `internal/scheduler`'s shipped
  `TestBackoff_strictlyGrowingUntilCeiling` keeps its name and every assertion in it under row 15's
  byte-identical rule: its name overpromises, but what it actually asserts is already non-decrease at
  the default factor, where every step of the ramp is at least the base and so never below the
  nanosecond the conversion truncates to
  `[measured 515f03d:internal/scheduler/cadence_test.go:47-63 · `sed -n '47,63p'
  internal/scheduler/cadence_test.go` → `if cur < prev {` … `want non-decreasing`]`. Renaming it is
  neither required by this amendment nor forbidden by it, and is a scope-boundary item to surface
  rather than a silent edit.
- **`internal/backoff`, overflow:** at a non-integer factor, rows at attempts far past any a
  doubling survives — including one at `math.MaxInt` — asserting **exactly** `ceiling` and strict
  positivity, never "it did not panic", because a wrapped `time.Duration` is a silent negative rather
  than a crash. The `math.MaxInt` row's failure mode is named here in advance so a later reader does
  not misread it: if that row *times out* rather than failing, the implementation has reintroduced an
  `O(attempt)` loop `[derived → D20's overflow contract]`.
- **`internal/backoff`, the values that may carry a tolerance:** rows at `1.3` and `1.5` written as
  **literal durations** with the `max(1ns, 1e-9 × want)` tolerance D20 bounds — never as a
  re-computation of the implementation's own formula, which would pin nothing. `EqualJitter` keeps
  its shipped bounds cases at a literal `2`, unchanged, and gains one non-integer-factor case
  asserting the `[d/2, d)` bracket against `Exponential`'s own return, which needs no tolerance
  because it is an inequality `[derived → D20's epsilon bound]`.
- **The non-default-factor scenarios — one per production call site, and the only instrument that
  sees a call site passing the default.** In `internal/tg`, a case shaped like the shipped
  `TestRetry_JitterOptionThreadedThroughToBackoff` — a `synctest` bubble, a jitter stub returning `0`,
  attempt timestamps off the fake server — with the transport's factor set away from `2` and the
  inter-attempt gaps asserted against literal durations computed for that factor
  `[measured fc5e6dd:internal/tg/retry_test.go:128-153 · `sed -n '128,153p' internal/tg/retry_test.go`
  → `synctest.Test(t, func(t *testing.T) {` … `o.Jitter = func() float64 { return 0 }` …
  `want := []time.Duration{50 * time.Millisecond, 100 * time.Millisecond, 200 * time.Millisecond}`]`.
  In `internal/scheduler`, **two** new test functions rather than one, because that package computes
  a persisted `run_at` at two production call sites and a scenario is per call site (D20): one beside
  `TestFailurePolicy_oneShotAttemptsGrowAndGiveUp` in `failure_test.go`, driving the **inline
  one-shot** settlement, and one beside `TestDeadline_successiveBreaches_growingDelay` in
  `deadline_test.go`, driving the **deferred drain** settlement — the two paths those shipped tests
  already exercise, and the two the literal one-based ramps already watch. Each is a **new**
  function, never an edit to either shipped one, whose literal ramps stay literal and stay at the
  default; each sets `cfg.RetryFactor` away from `2` on the config its own path receives, and
  brackets the persisted `run_at` against a literal ramp at that factor: D2's two instruments, each
  re-aimed at the new argument at the site its own comment names. Splitting them is what makes the
  pair discriminating rather than redundant — with one scenario only, an implementor who threads the
  configured field at the covered site and `backoff.DefaultFactor` at the uncovered one ships a
  half-dead feature with the whole suite green, and the defect lands in persisted data.
  In `internal/ingest`, a retry case with a non-default factor and a base chosen so the summed delay
  at that factor is far above the summed delay at the default,
  asserted as a **lower** bound on elapsed wall time so the case cannot flake on a slow machine —
  the same small-real-durations approach the scheduler's timing tests already take, since this
  package's paths touch a real database and cannot run inside a bubble. Each of these reds if its call
  site passes `backoff.DefaultFactor`; no shipped test does `[derived → AC41]`.
- Fixtures: the existing test config builders, each carrying a legal factor; the existing jitter
  stubs and fake Bot API server; no new fixture kind.

**Subtask 16 — propagation** (no new test; CI's harness job is the only automated check that touches
it)
- That job runs the citation guard and a relative-link check over the repository's markdown
  `[measured fc5e6dd:.github/workflows/ci.yml:163-207 · `sed -n '163,207p' .github/workflows/ci.yml` →
  the `citation namespaces resolve` step running
  `.claude/skills/ai-audit/scripts/check-citations.sh` and the `relative markdown links resolve`
  step]`, neither of which can see a stale *claim*. The check for this row is therefore a reader's,
  and D20's closing paragraph is its checklist: every sentence it names must agree with the shipped
  code once subtask 15 has landed `[derived → subtask 16]`.

## Open questions

- **A per-update execution deadline.** D7 ships none. A handler that ignores its context stalls the
  sequential loop with no bound; the scheduler solved the same problem with a transaction-local
  `statement_timeout` and a hijackable connection. Nothing in this task's ACs requires it and no
  production handler exists yet, so it is recorded rather than built.
- **Whether `cmd/bot` becomes a running process here.** The spec says no, on #19's precedent; this
  design follows the spec and records the composition order (D17) instead. Changing that is a scope
  change, not a design amendment.

**Settled in round 2 and no longer open** — recorded here so a later reader does not reopen them:
the `internal/tg` half of D2's adoption stays in scope (`internal/backoff` ships and **both**
adopters are re-expressed in terms of it — D2), and the offset row stays the singleton D14
specifies, with the per-bot widening left to a later forward migration (D14).

**Settled in round 6 by the owner's amendment, recorded so a later reader does not reopen them:** the
ramp's growth factor is configuration, its compiled-in default stays `2`, and the literal is lifted
into `backoff.DefaultFactor` (D20); a configured factor at or below `1`, or one that is not finite,
is a start-up `*KeyError` naming its variable, never a silent clamp and never a fallback to the
default (D20); the key is **per adopter** rather than shared (D20); and `1.3` — the value that
started this round — is reached by setting `LAB_GAME_TG_RETRY_FACTOR`, not by a code change. At the
ingestion loop's default base and ceiling a factor of `1.3` ramps `1s`, `1.3s`, `1.69s`, `2.197s`,
`2.8561s`, `3.71293s` over attempts `0`–`5` `[measured fc5e6dd · a probe module under tmp/ carrying
the float form D20 specifies · `go run .` → those values at `base = 1s`, `ceiling = 8s`,
`factor = 1.3`]`, which is what an operator would actually get.

**Settled after design-review round 8 — both questions the amendment raised are now answered, and
neither is reopened:**

- **The five-group overflow is approved.** The owner approved five design-defined groups over the
  default maximum of four, so the amendment runs as Groups D and E on this branch rather than as an
  issue of its own `[measured ef69c56:ai-docs/plans/2026-09-06-update-ingestion-dispatch-idempotency.progress.md:424 ·
  `sed -n '424p' <that file>` → `- Amendment: the owner approved **five** design-defined groups over
  the default maximum of four (handoff-grouping (h) makes an overflow the owner's call, never the
  design's). …`]`. § Handoff plan records the approval at the
  group count itself; nothing about the grouping is left open.
- **The spec's acceptance criteria now record the configurable factor.** `spec-writer` amended the
  spec after round 8, adding Scope 13, two *Key decisions* rows, *Technical constraints* item 10, the
  factor to AC27's enumeration, and **AC39–AC43**
  `[measured ef69c56:ai-docs/plans/2026-09-06-update-ingestion-dispatch-idempotency.spec.md:5,443-447 ·
  `sed -n '5p;443,447p' <that file>` → `**Amended:** 2026-09-07 (design-review round 8)` and the
  criterion rows `AC39` through `AC43`]`. D20 therefore
  traces to criteria rather than to prose alone: the three keys with their compiled-in defaults,
  their `config.EnvKeys()` membership and their `.env.example` lines → **AC39**; the unchanged
  default carried in one named constant, with no factor a literal at its point of use → **AC40**;
  the call-site gate, and the isolation of one scope's configured factor from the other two →
  **AC41**; the not-strictly-greater-than-`1` refusal → **AC42**; and the non-finite, `NaN` and
  unparseable refusals → **AC43**. No clause of D20 changes as a result: the criteria record the
  decision the owner had already made, and this design still does not edit the spec. AC23 also
  stands word for word — the ramp never shrinks, and D20's contract table declines the
  strict-increase reading AC23 never asked for
  `[measured ef69c56:ai-docs/plans/2026-09-06-update-ingestion-dispatch-idempotency.spec.md:427 · `grep -n 'AC23' <that
  file>` → `A handler error that is not a duplicate is retried, each attempt in a transaction of its
  own, with a strictly positive delay between attempts that does not shrink, up to a configured
  attempt cap.`]`.

One item round 7 surfaced is **routed to the owner rather than decided here**:

- **`internal/scheduler`'s `TestBackoff_strictlyGrowingUntilCeiling` is named for a property it does
  not assert.** Its body compares `cur < prev` and reports `want non-decreasing`
  `[measured ef69c56:internal/scheduler/cadence_test.go:47-63 · `sed -n '47,63p'
  internal/scheduler/cadence_test.go` → `if cur < prev {` … `want non-decreasing`]`. What it asserts
  is right and is what D20's contract table promises; only the name overpromises. Renaming it is
  neither required by this amendment nor forbidden by it, and row 15's byte-identical rule keeps
  every assertion in it standing, so the rename is a scope-boundary item for the owner — never a
  silent edit inside this task.
