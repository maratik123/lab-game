# Interview state — Goroutine-leak detection in the test suite: adopt goleak

Handoff between rounds, and the re-entry point for every later return to `spec-writer`. Kept on `ready`.

```yaml
schema_version: 1
spec_path: ai-docs/plans/2026-09-11-goroutine-leak-detection-goleak.spec.md
issue_ref: "#74"
gh_issue:
  title: "Goroutine-leak detection in the test suite: adopt goleak"
  state: open
  labels: ["enhancement", "area:platform"]
  body: |
    ## What

    Make "no goroutine was leaked" a **checked** condition in the test suite rather than a reviewed one, by adopting `go.uber.org/goleak`.

    Several packages already start goroutines that outlive the call that started them — the ingest loop, the scheduler worker, the health metrics server and the canary — and nothing in the module verifies that any of them stop. A leak is invisible in a green suite: the goroutine simply keeps running inside the test binary until the process exits.

    ## Why now

    #24 makes `cmd/bot` a real process with a graceful shutdown, and its spec carries an acceptance criterion that after a signal-driven shutdown no goroutine started by this module's packages remains running. There is no instrument in the module capable of checking that today, so the criterion would be verified by reading the code.

    ## The dependency situation

    Verified rather than recalled — each fact with the command that produces it:

    - Not a requirement: `grep 'goleak' go.mod` → no match.
    - Not imported anywhere: `grep -rn 'go.uber.org/goleak' --include='*.go' .` → no match.
    - **But already in the module graph**, through the Prometheus client's own test binary: `go mod why -m go.uber.org/goleak`.
    - **And already pinned**, with both the module hash and the `go.mod` hash: `grep 'goleak' go.sum` → `v1.3.0` on both lines.
    - `v1.3.0` is also the latest published version: `go list -m -versions go.uber.org/goleak`.

    So promoting it to a direct test dependency is a `require` line — not a version decision, and not a new module tree to audit. That is the whole cost, and it is worth stating in the spec because `AGENTS.md` § *Dependency Versions* requires a new dependency to carry a stated reason.

    ## Scope

    - **Where the detector runs, and how it composes with the existing `TestMain`.** `internal/testdb.Main` already owns `TestMain` in every database-backed package, and a package may declare only one. The composition — wrapping, an option on the existing helper, or a separate entry point — is the design's call, but it has to be made rather than discovered.
    - **The ignore set, with a reason per entry.** The pgx pool, the container runtime, the Prometheus client and HTTP keep-alive connections all hold goroutines across tests. An ignore list wide enough to make the suite green is also wide enough to detect nothing, so each entry carries its justification, and the set needs a way to stay honest as dependencies move.
    - **Which packages are in.** The ones that start goroutines today, or the whole module.
    - **Whether it is a gate.** If it runs in `make verify` and in CI, which job and which paths-filter; if it does not, it is documentation, not a gate.
    - **What a detection reports** — a failure that names the leaked goroutine's stack, not merely that a count is non-zero.

    ## Out of scope

    - **#24's shutdown behaviour.** That task ships the shutdown; this one ships the instrument. The dependency direction is deliberately left open: if #24 merges first it satisfies its own criterion by whatever means its design chooses, and this issue then generalises that into a suite-wide capability. #24 is not blocked on this issue.
    - **Race detection.** `-race` is already a required gate and detects a different class.
    - **Test provisioning and the container-runtime import invariant** (KD-20, #67) — those are their own surfaces, and this issue only has to avoid contradicting them.

    ## Telemetry obligation

    - Metrics: none.
    - Events: none.

    This adds no mechanic and moves no balance.

    ## Open questions to close in the spec

    - **A pre-existing leak surfaced by adoption** — fixed inside this task, or ignored explicitly with its own tracking issue so the detector can land? The answer decides whether this task's size is knowable in advance.
    - **Per-package or module-wide.** A detector in a package that starts no goroutine costs a `TestMain` and catches a future regression; one only where goroutines exist today is smaller and silently stops covering a package that later grows one.
    - **Whether `cmd/bot`'s smoke test is the first real consumer**, given that it is the only surface where a whole assembled process starts and stops.

  comments: []
  linked_issues: ["#24", "#67"]
  issue_body_status: current
  linked_prs: []
round_cap: 4
questions_per_round_cap: 3
round: 2
agent_id: aa4d3b7e2006369e6
prior_qa:
  - round: 1
    question: "A leak the detector surfaces in today's code when it lands: fixed inside this task, or ignored explicitly with its own tracking issue? For sizing: a one-run probe (check at the end of each test binary, shared-server route; per-test checks and the fallback route not probed) found leaks in 3 of the 16 packages (internal/health, internal/tg, internal/ingest), all from one cause: the fake Bot API test helper's delayed-response handler keeps sleeping after its test ends. No production goroutine was left running."
    answer: "Fix in task — Every leak found at adoption is fixed here, so none lands excused. The size stays open until the design has run the detector everywhere; today's probe suggests one test-helper fix."
  - round: 1
    question: "Which packages run the detector? Today 16 packages have tests; 6 start a goroutine with a go statement in non-test code (cmd/bot, health, ingest, scheduler, tg, tgtest); 5 already have a TestMain, all delegating to testdb.Main."
    answer: "Module-wide — Every package with tests runs it. A package that later grows a goroutine is covered from its first test; costs a TestMain in the 11 packages that have none."
  - round: 1
    question: "What keeps the ignore set honest as dependencies move? Each entry carries its reason in every option."
    answer: "don't sure what is better, explain, what consequences of each option"
  - round: 1
    question: "(orchestrator explained each option's consequences — goleak's API reports no per-entry match, so a stale check is own code over it; a goroutine alive only sometimes makes a stale check flaky unless entries are scoped per package or route; a version pin checks that a line was touched, fails on every dependency move and misses stdlib goroutines; reason-only lets dead entries accumulate, and an entry's width matters more than its age — recommended Stale fails — then re-put the same question verbatim)"
    answer: "Stale fails — An entry that matches no goroutine in the runs it applies to fails the suite, so a dependency move that drops or renames a goroutine forces the entry to be updated or deleted."
```
