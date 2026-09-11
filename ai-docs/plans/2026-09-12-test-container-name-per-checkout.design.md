# Design: Test database container named per checkout

**Issue:** #91
**Date:** 2026-09-12

## Approach

### What is there now

The long-lived test server is addressed under a compile-time constant: `internal/testdb` exports
`SharedContainerName`, and `cmd/testpg` passes it to the provisioning options on the `--up` path and
again on the `--down` path
[measured 42792be:internal/testdb/server.go:14-17 · `sed -n '14,17p' internal/testdb/server.go` →
`const SharedContainerName = "lab-game-test-postgres"` under a doc comment calling it "the shared
name a long-lived server is created under"]
[measured 42792be:cmd/testpg/run.go:245,311 · `grep -n SharedContainerName cmd/testpg/run.go` →
`ContainerName: testdb.SharedContainerName` inside `runUp`, and the same field inside `runDown`].

The provisioning option that consumes it reuses an existing container of that name rather than
creating a second
[measured 42792be:internal/testdb/server.go · `grep -n WithReuseByName internal/testdb/server.go` →
`moduleOpts = append(moduleOpts, testcontainers.WithReuseByName(opts.ContainerName))`], and the
upstream option validates nothing but non-emptiness
[measured 42792be · `sed -n '136,166p' $(go env GOMODCACHE)/github.com/testcontainers/testcontainers-go@v0.44.0/options.go`
→ `if containerName == "" { return errors.New("container name must be provided") }`, then
`req.Name = containerName; req.Reuse = true`].

Neither fact carries a per-checkout term, so on one host both checkouts address one container:
`make test-db-up` in the second checkout joins the first checkout's server, and `make test-db-down` in
either removes it from under the other. That sentence is a deduction from the two measurements above,
and it is also the condition the issue opens with
[measured 42792be · `gh issue view 91 --json body` → "The long-lived test-database server is addressed
under one fixed container name for the whole host, so two checkouts of this project on one machine
contend for a single server"].

The **locator** — the file that tells a gate run which long-lived server to use — is already
per-checkout: it is read and written at a path relative to the invoking process's working directory
[measured 42792be:cmd/testpg/locator.go:12 · `sed -n '8,12p' cmd/testpg/locator.go` →
`const locatorPath = "tmp/testpg-dsn"`, documented as "relative to the working directory every target
and hook in this project already invokes its recipes from"]. Every caller of the wrapper does invoke
it from the repository root
[measured 42792be:Makefile · `grep -n 'go run ./cmd/testpg' Makefile` → a bare `go run ./cmd/testpg …`
recipe line at each of the `test`, `test-race`, `test-db-up`, `test-db-down` and `test-contention`
targets, none of them changing directory first]
[measured 42792be:.githooks/coverage-ratchet.sh:75-77,115 · `sed -n '74,78p;115p' .githooks/coverage-ratchet.sh`
→ `root=$(git rev-parse --show-toplevel …)` then `cd "$root"`, and later `go run ./cmd/testpg -- go test …`].

That is the whole gap: the checkout's identity already decides *which DSN a gate reads*, and does not
yet decide *which container a checkout creates and removes*.

### Chosen solution

Derive the container name from the base name of the wrapper's own working directory and append the
fixed test-server suffix, so a project directory named `lab-game` addresses `lab-game-test-postgres`
and one named `lab-game2` addresses `lab-game2-test-postgres`. The derivation is a pure function of
that directory string; the working directory reaches it through the wrapper's existing seam, so the
tests never touch the process-global working directory. Only the paths that name a container —
`runUp` and `runDown` — call it.

### Key decisions

- **D1 — The name is the project directory's base name plus a fixed suffix, and nothing else.** The
  parent path contributes nothing, so two checkouts whose project directories carry the same name
  address one server, which is the owner's decision on the name basis
  [measured 42792be:ai-docs/plans/2026-09-12-test-container-name-per-checkout.spec.md.state.md ·
  `cat ai-docs/plans/2026-09-12-test-container-name-per-checkout.spec.md.state.md` → `prior_qa` round 1
  answer: the owner selected the option labelled "Имя каталога", carrying "The name comes from the
  project directory's name exactly, as in the examples. Two checkouts with the same directory name
  under different parents share one server."]. The suffix is a named constant in the wrapper, not a
  literal spelled twice [derived → AC1's table cases in § Test Design].
- **D2 — The working directory is the checkout, and the wrapper already treats it that way.** The
  derivation reads the same fact the locator already resolves against, so one notion of "this
  checkout" serves both. No new locating mechanism, no git subprocess.
- **D3 — `testdb.SharedContainerName` is deleted outright, and the derivation lives in
  `cmd/testpg`.** The wrapper is the only thing in this module that names a container: the fallback
  path provisions an anonymous one on purpose, and `testdb.Main` names nothing
  [measured 42792be · `grep -rn SharedContainerName --include='*.go' .` → `internal/testdb/server.go`,
  `cmd/testpg/run.go`, `cmd/testpg/run_test.go`]
  [measured 42792be:cmd/testpg/run.go:245,311 · `grep -n ContainerName cmd/testpg/run.go` → the field
  is set inside `runUp` and inside `runDown`, and nowhere else in the wrapper]. No delegating alias and no deprecation layer
  (`AGENTS.md` § API Stability — clean breaks, no compat shims).
- **D4 — The working directory arrives through `seam`, beside the locator accessors.** `productionSeam`
  supplies the real lookup; the stub seam in the package's tests supplies a directory string. The
  alternative — calling the process-global lookup inside `runUp`/`runDown` — would force a test to
  change the process's working directory, which is global state in a package whose tests run in
  parallel — and that package already pays this price once
  [measured 42792be:cmd/testpg/run_test.go:318-320 · `sed -n '318,320p' cmd/testpg/run_test.go` →
  "Not parallel, and neither is the undersized-reuse case below: both reach the code that disables the
  reaper, which is a process-wide setting written to the environment."].

  **Its shape, and what a failing lookup does.** The member carries the same two results the standard
  lookup returns — a directory string and an error — because discarding the error is what the code
  style forbids outright
  [measured d41174f:AGENTS.md · `grep -n 'Never `_ = err`' AGENTS.md` → "Never `_ = err`." in
  § Code Style → Errors]. `productionSeam` supplies the standard library's own working-directory
  lookup unwrapped; every other member of `seam` is supplied the same way
  [measured d41174f:cmd/testpg/run.go:53-67 · `sed -n '53,67p' cmd/testpg/run.go` → `probe`, `locate`,
  `persist` and `forget` are each a bare function value, only `provision` wrapped]. On both `--up` and
  `--down` a failing lookup **folds into the derivation's own failure path**: one non-zero exit, whose
  message says the working directory could not be read and carries the underlying error, in the
  print-and-return-a-code idiom every other failure in the wrapper already uses, exactly as the
  invalid-name exit does for a directory that was read. There is no
  second failure mode and no fallback directory — a wrapper that guessed a name here would address a
  container belonging to no checkout [derived → the failing-lookup cases in § Test Design, subtask 2].
- **D5 — Only `runUp` and `runDown` derive a name; `runChild` never does.** A gate run must not be
  able to fail on a fact it does not use, and the fallback container stays anonymous because only a
  server this invocation started may be removed (KD-20). `runUp` derives before it disables the
  reaper or provisions anything; `runDown` derives after its locator and reachability checks, so
  "nothing to remove" stays a side-effect-free exit
  [measured 42792be:cmd/testpg/run.go:286-300 · `sed -n '286,300p' cmd/testpg/run.go` → `sm.locate()`
  returning not-ok prints "nothing to remove" and returns 0 before anything else runs].
  Residue, so a later reader does not read the omission as an oversight: the anonymous fallback
  container is deliberately left unnamed rather than given a per-checkout name of its own. It is
  already per-invocation — nothing else can reach it, and nothing else can remove it — so a name would
  buy no isolation and would cost the property KD-20 names. The spec's Out of scope does not reach the
  question, and nothing in it needs an answer to proceed.
- **D6 — A derived name the container runtime would refuse fails cleanly *here*; it is never
  sanitised.** Measured on this host: podman 5.8.2 refuses a name outside `[a-zA-Z0-9][a-zA-Z0-9_.-]*`
  [measured 42792be · `podman create --name 'lab game' docker.io/library/alpine:latest true` →
  `Error: running container create option: names must match [a-zA-Z0-9][a-zA-Z0-9_.-]*: invalid
  argument`, rc 125; the same rejection for `lab-game@2`, `.lab-game`, `-lab-game` and a Cyrillic
  name, while `lab_game.2-x` was accepted]. Sanitising would map distinct project directories onto
  one name and silently re-create the contention this task removes, which contradicts D1. So: a
  non-zero exit whose message names the offending directory and states the rule, on `--up` and
  `--down` alike [derived → the invalid-directory cases in § Test Design].

  **Why a pre-check rather than letting the runtime's own refusal through.** The alternative is real
  and was compared: `runUp` already surfaces the provisioning error verbatim, so a bad name would
  reach the developer without any code of ours. It loses on three measured grounds.

  1. **The runtime's refusal names the rule and never the offending string** — on the command line and
     on the API socket testcontainers actually dials
     [measured d41174f · `podman create --name 'lab game' …` captured to a file (exit status read from
     `$?`, not from a pipe) → `Error: running container create option: names must match
     [a-zA-Z0-9][a-zA-Z0-9_.-]*: invalid argument`, rc 125 — the string `lab game` appears nowhere in
     it] [measured d41174f · `curl --unix-socket /run/user/1000/podman/podman.sock -X POST
     'http://d/v1.41/containers/create?name=lab%20game' …` → HTTP 500, body `{"cause":"invalid
     argument","message":"container create: running container create option: names must match
     [a-zA-Z0-9][a-zA-Z0-9_.-]*: invalid argument","response":500}` — again no offending string]. A
     developer in a checkout whose directory name is refused would be shown a character-class rule and
     no hint that the fault is the name of the directory they are standing in — which is precisely the
     diagnosis this change introduces the need for.
  2. **The sentence the wrapper wraps it in misdescribes the fault.** `runUp` renders it as "could not
     start the shared server" and `runDown` as "could not reach the shared server to remove it"
     [measured d41174f:cmd/testpg/run.go:249,313 · `grep -n 'could not start the shared
     server\|could not reach the shared server' cmd/testpg/run.go` → both messages]. Neither is true:
     the server is fine, the checkout's name is not.
  3. **It fails before the side effects.** Both paths write `TESTCONTAINERS_RYUK_DISABLED` into the
     process environment before they provision
     [measured d41174f:cmd/testpg/run.go:239,306 · `grep -n 'TESTCONTAINERS_RYUK_DISABLED'
     cmd/testpg/run.go` → an `os.Setenv` in `runUp` and another in `runDown`; `grep -n 'sm.provision'`
     puts each one above its own path's provision call]. A pre-check ahead of that leaves the process environment untouched on the
     failing path, and on `--down` it also means nothing is reached for at all.

  **The cost, stated rather than waved past.** The pattern is this project's copy of a rule the
  runtime owns, so it can drift. The drift is one-directional by construction: the pre-check is a
  *diagnosis layer*, never the authority — a name it accepts and the runtime refuses still fails, with
  the runtime's own message, exactly as today. Only the opposite drift bites (a runtime whose rule
  loosens would have a legitimate directory name refused here), and that is carried in § Risks rather
  than assumed away. Only podman was measured; no claim is made about any other runtime's rule.
- **D7 — The validity check is a package-level compiled pattern.** The pattern is a compile-time
  constant, so the established shape in this module is a package-level `regexp.MustCompile`
  [measured d41174f:internal/commentref/classify.go:33-42 · `grep -n MustCompile
  internal/commentref/classify.go` → a block of package-level `regexp.MustCompile` values in
  production code]. The panic index carries no row for any of them
  [measured d41174f:ai-docs/panic-index.md · `cat ai-docs/panic-index.md` → the table body is the
  single row `| — | — | — |`, under a header whose scope is "`panic`, `log.Fatal*`, `log.Panic*`,
  `must…` helpers, a deliberate nil-map write" in production code], so this is the practice the index
  and the review together already treat as needing none — not a gap the hook happens to miss. The
  index's own header names review and the hook as *both* keeping it in sync, so what the hook matches
  is not what decides a row, and no argument here rests on it.
- **D8 — `--up`'s report line names the container it brought up.** The derivation is otherwise
  invisible at the terminal, and a developer running two checkouts needs to read which server they
  just got. The existing capacity report goes to stderr — stdout carries only the DSN — and the tests
  over it match on substrings, so an added clause displaces nothing
  [measured 42792be:cmd/testpg/run.go:258-278 · `sed -n '258,278p' cmd/testpg/run.go` → `logf(stdout,
  "%s\n", dsn)` and then a switch whose every arm writes to stderr]
  [measured 42792be:cmd/testpg/run_test.go · `grep -n 'strings.Contains(stderr' cmd/testpg/run_test.go`
  → every stderr assertion in the package is a substring containment check]
  [derived → the `--up` report assertion in § Test Design].
- **D9 — No new file: the derivation joins `run.go` beside `productionSeam`, and its tests join
  `run_test.go`.** The package already holds a command file, a locator file and a wrapper file, and a
  further small file would push it toward the shape the style reference names as the failure mode
  [measured 42792be:ai-docs/code-style.md:68-79 · `sed -n '68,79p' ai-docs/code-style.md` → the
  four-band table (500 reasonable, 800 soft, 1000/1500 hard and gated) and the counter-rule "do not
  over-split — one type per file is not a Go idiom, and a package of ten 40-line files is harder to
  read than one 400-line file"].
- **D10 — `cmd/testpg`'s test binary starts no container; the container-level half of AC2/AC3 is a
  verify-time probe.** The obstacle is **not** the `Binaries` manifest constant: that constant counts
  packages whose test files reference `testdb.Main`, and a test could reach a real named server
  through `testdb.StartServer` — which `cmd/testpg` already imports and calls — without moving it
  [measured d41174f:internal/testdb/server_test.go:177-205 · `grep -n -B5 -A30 callersOfMain
  internal/testdb/server_test.go` → `callersOfMain` "returns the set of package directories … whose
  test files hold a syntactic reference to this package's Main"]
  [measured d41174f:cmd/testpg/run.go:53-61 · `grep -n -A10 'func productionSeam' cmd/testpg/run.go` →
  `provision` calls `testdb.StartServer` directly]. The three grounds that do hold, each executed:

  1. **A real `--up` writes a locator file into the source tree, and the repository's ignore rule does
     not cover it.** The locator path is relative and the test binary's working directory is its own
     package directory, so the file lands beside the package's sources; the ignore rule for the
     scratch directory is anchored at the repository root
     [measured d41174f:.gitignore:18 · `git check-ignore -v tmp/testpg-dsn` → `.gitignore:18:/tmp/`,
     while `git check-ignore -v cmd/testpg/tmp/testpg-dsn` exits 1 — no rule matches]
     [measured d41174f · a scratch module outside this repository whose test logs `os.Getwd()`, run
     with `go test ./sub/ -run TestWD -v -count=1` → the package's own source directory]. Every CI run
     and every developer's `make test` would leave an untracked file the guards then see.
  2. **A real `--up` disables the reaper process-wide and creates a container that outlives the test
     binary — by design.** That is the wrapper's purpose, and it is fatal inside a test process: the
     `os.Setenv` is global to the binary (which is already why the package's `--up` cases are
     non-parallel), and the container it then creates carries no reap label, so an interrupted or
     failing run leaks it on every machine that runs the suite
     [measured d41174f:cmd/testpg/run.go:236-242 · `sed -n '236,242p' cmd/testpg/run.go` → the comment
     "this process must disable it before provisioning so the container it creates … outlives this
     process"].
  3. **The package's suite reaches no container runtime today, and that is a property worth keeping.**
     Its `TestMain` is the non-database-backed form, and every test drives the wrapper through the
     stub seam
     [measured d41174f:cmd/testpg/main_test.go:10-12 · `grep -rn 'TestMain\|leaktest' cmd/testpg/` →
     `os.Exit(leaktest.Main(m, (*testing.M).Run))`, not `testdb.Main`]. A runtime-dependent case here
     would make `go test ./cmd/testpg/` fail on a machine with no container runtime, which it does not
     today.

  Re-confirmed from these grounds: the manual probe is still the right answer for the container-level
  half, and the locator hazard in ground 1 additionally binds subtask 2's new cases — they stay on the
  stub seam, which reaches neither the locator file nor a runtime (§ Test Design). The probe runs once,
  at verify time, from throwaway directories outside the repository.

### Rejected alternatives

- **Derive in `Makefile` and pass the name in as a flag.** Every caller would have to repeat the
  derivation, a hand-run `go run ./cmd/testpg --down` would get none, and the wrapper's behaviour
  would depend on which caller invoked it.
- **Derive from `git rev-parse --show-toplevel`.** Adds a subprocess and a git dependency to a tool
  whose working directory already *is* the checkout root by every caller's construction (D2), and it
  answers a different question from the one the locator answers when a checkout is entered through a
  symlink (see § Risks).
- **Hash the absolute path into the name.** Contradicts the owner's decision that the same directory
  name means one server (D1), and produces a container list a developer cannot read.
- **Keep the constant and add a per-checkout suffix from an environment variable.** Moves the burden
  to the developer, who must then export it in every shell, and leaves the default behaviour broken.

## Decomposition

| # | Task | Files | Depends on |
|---|------|-------|------------|
| 1 | Add the pure derivation: the fixed suffix constant, the compiled validity pattern, and a function mapping a project-directory path to the container name or to an error naming the directory and the rule. Table test first (TDD), covering the spec's worked examples, the same base name under different parents, and each shape the runtime refuses. Nothing is wired yet. | `cmd/testpg/run.go`, `cmd/testpg/run_test.go` | — |
| 2 | Wire it: add the working-directory lookup to `seam` (returning a directory and an error, D4) and to `productionSeam`; `runUp` derives the name **before** it disables the reaper or provisions, `runDown` derives it after its locator and reachability checks; a failing lookup and an invalid name share one non-zero exit carrying the derivation's own message; `--up`'s report names the container (D8); delete `testdb.SharedContainerName` and its doc comment. Update the existing `--up` name assertion, add the `--down` name assertion, the invalid-directory and failing-lookup cases for both paths, the reaper-untouched assertion pinning the pre-check's position, and a case pinning that a child gate run derives no name at all — all on the stub seam (D10 ground 1). | `cmd/testpg/run.go`, `cmd/testpg/run_test.go`, `internal/testdb/server.go` | 1 |
| 3 | Amend the live prose: KD-20's parenthetical, which names the container as one fixed name for the host, states the derivation instead; the shared-server section of the test conventions gains the derivation and its consequence for two checkouts on one host. | `ai-docs/key-decisions.md`, `ai-docs/go-test-conventions.md` | 2 |

**What authorises subtask 3, now that no acceptance criterion does.** The spec's former AC4 — the row
requiring every live surface that states the old fixed name to state the derivation instead — was struck
by the owner during Step 7, leaving criteria that are all about which server a checkout addresses; the
former AC5 renumbered into the vacated slot
[measured 4043659:ai-docs/plans/2026-09-12-test-container-name-per-checkout.spec.md ·
`git show 4043659 -- ai-docs/plans/2026-09-12-test-container-name-per-checkout.spec.md` → the
propagation row deleted, and the same-directory-name row renumbered from AC5 to AC4]. Subtask 3 stands on
two grounds that never rested on that row. **(a)** The ruling that struck it named this subtask as the
delivery in the same breath
[measured 4043659:ai-docs/plans/2026-09-12-test-container-name-per-checkout.spec.md.state.md ·
`git show b53299f` → the round-3 `prior_qa` answer "Spec amendment: AC4 is removed outright. The
propagation of the KD-20 and test-conventions edits is delivered by the design's subtask 3 and by the
standing workspace rule regardless."]. **(b)** That standing rule binds on every branch, this one
included: a change propagating a factual claim sweeps every live document asserting it
[measured 4043659:AGENTS.md:288 · `grep -n -A6 '^4. When the change propagates' AGENTS.md`
→ "Completeness test: every LIVE doc must agree; history surfaces (`ai-docs/learnings.md`,
`ai-docs/plans/done/**`) are left untouched"]. `ai-docs/key-decisions.md` is a live document asserting the
old fixed name, so the sweep reaches it
[measured 4043659:ai-docs/key-decisions.md:53 · `grep -rn 'lab-game-test-postgres' --include='*' .`
excluding `.git` and `tmp` → `internal/testdb/server.go`, `ai-docs/key-decisions.md`, and this task's own
spec and interview-state files]. Neither ground is an acceptance criterion, so subtask 3 is verified at
Step 9 as a design obligation and by the reviewer — not against an AC row.

On subtask 3's second file: the test-conventions page names no container, so neither ground above compels
it
[measured 42792be:ai-docs/go-test-conventions.md:43-44 · `sed -n '43,44p' ai-docs/go-test-conventions.md`
→ "`make test-db-up` creates the named long-lived server and records its DSN in the ignored scratch
directory", with the name itself never written]. It is amended anyway because it is the developer-facing
home for `make test-db-up`, and the parallel-checkout consequence this change delivers has no other live
page to land on — a design decision, within Scope item 1, carried by the owner's ruling above rather
than by any criterion.

Surfaces deliberately **not** amended, with the evidence:

- `AGENTS.md` names no container
  [measured 42792be · `grep -rn 'lab-game-test-postgres' --include='*' . | grep -v '^\./\.git/'` →
  `internal/testdb/server.go`, `ai-docs/key-decisions.md`, and this task's own spec and interview-state
  files]. Its § Build & Test lines describe `make test-db-up` as bringing up a long-lived server and
  the wrapper as preferring one that answers and admits the run — each still true after the change
  [measured 42792be:AGENTS.md:48-49,76-78 · `sed -n '48,49p;76,78p' AGENTS.md` → "bring up a long-lived
  shared test server (CLIENTS=N sizes it)", "remove it — no reaper will", and "otherwise a long-lived
  server from `make test-db-up` if that one answers and admits the run"].
- `ai-docs/context-status.md` is untouched on two independent grounds, neither of them the
  Propagation Rule's exemption list — that list names `ai-docs/learnings.md` and
  `ai-docs/plans/done/**`, and this file is not among them
  [measured d41174f:AGENTS.md:288 · `grep -n -A6 '4\. When the change propagates' AGENTS.md` → "history
  surfaces (`ai-docs/learnings.md`, `ai-docs/plans/done/**`) are left untouched"]. The grounds that do
  hold: **(a)** the file declares itself append-only in its own header, so an entry records what landed
  then, not what is true now
  [measured d41174f:ai-docs/context-status.md:3 · `sed -n '3p' ai-docs/context-status.md` → "The
  detailed, append-only implementation log: one entry per completed task … Written by `/task` Step
  9.5"]; **(b)** what it names is the **symbol** `SharedContainerName`, inside a list of the exported
  API that task added — not a container name, and so not a site the propagation sweep reaches
  [measured d41174f:ai-docs/context-status.md:188 · `grep -n SharedContainerName ai-docs/context-status.md`
  → the symbol appears inside the parenthesised list "(`ServerOptions`, `StartServer`, `Server` with
  `DSN` / `Stop`, the capacity `Probe`, `DSNEnv`, `SharedContainerName`)"].
- `ai-docs/context.md` describes what `internal/testdb` owns without enumerating its exported symbols
  [measured 42792be · `grep -n SharedContainerName ai-docs/context.md` → no match; the same file's
  `internal/testdb` sentence describes "PostgreSQL provisioning for the whole suite … exported as an
  API that `cmd/testpg` imports"].

## Handoff plan

Grouping is required for every `M ≥ 1`, and this design's `M` is the Decomposition table's subtask
count; the contract's sub-points (a)–(h) are applied below. Every group is homogeneous by
change-type, marked with its implementor model and effort, and the group count is the minimum the
change-type split and the dependency order allow.

- **Handoff into Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § Compaction recovery (re-entry). The first group gets a handoff exactly as every later one does.
- **Group A** — model `sonnet`, effort `medium` (pinned) via the `code-writer` subagent, 1M-token
  window — subtasks 1–2 (code change-type: `*.go`). The same-change-type subtasks are clustered into
  one group rather than interleaved with the prose work; the group is within the `≤ 10` size cap, and
  the dependency 2→1 is respected inside it.
- **Handoff after Group A:** spawn `/context-reset` per `.claude/skills/context-reset/SKILL.md`
  § Compaction recovery (re-entry). Parent `/task` resumes in Group B with fresh context.
- **Group B** — model `inherit` (the orchestrator's), effort inherited from the orchestrator
  (typically xHigh) — **not** pinned — via the `general-purpose` subagent with no inline `model=`
  override, 1M-token window — subtask 3 (instructions/harness change-type: `ai-docs/**`). Terminal
  group, sized 1, within the `1..=10` range.

Group-count check: the change-type switch between subtask 2 and subtask 3 forces the boundary, and
subtask 3 depends on subtask 2, so no reordering collapses the groups further. The total is within the
default maximum of 4 design-defined groups, so no user approval is needed.

## Risks

- **A checkout entered through a symlinked path derives a different name from the same checkout
  entered by its real path, while the locator file resolves to the same real directory either way** —
  so one checkout reached under two paths would own two servers and one locator, leaking the first
  container. Mitigation: stated as a residue rather than closed; a checkout is entered by one path,
  and the leak is a container the developer can see and remove. The alternative — resolving symlinks
  before taking the base name — would make the displayed name disagree with the path the developer
  typed, which is the name they will look for in a container listing. —
  [measured 42792be · a scratch Go program printing `os.Getwd()`, `filepath.Base` of it and `$PWD`,
  run by `cd <scratch>/link && go run <scratch>/wd.go` where `link` is a symlink to a real directory →
  `Getwd: …/scratchpad/link`, `Base: link`, `PWD env: …/scratchpad/link`; the Go runtime returns `$PWD`
  when it names the same directory]
- **A locator file written before this change, in a checkout whose directory name is not the old
  constant's prefix, points at the old shared container.** After the change that checkout's `--down`
  probes the stale DSN, finds it reachable, and then reaches for a container under the *new* name —
  creating and immediately removing an empty one while the old container keeps running. Mitigation:
  the scratch directory is ignored local state, so the remedy is to delete the stale locator file (or
  the old container) once, and the design names it here so the first run after the change is not read
  as a defect. —
  [measured 42792be:cmd/testpg/run.go:286-320 · `sed -n '286,320p' cmd/testpg/run.go` → `runDown`
  locates, probes, then provisions by name and stops that container; it never checks that the located
  DSN and the named container are the same server]
- **A project directory whose name the container runtime refuses breaks `--up`/`--down`.** Mitigation:
  the failure is a named, non-zero exit carrying the directory and the rule (D6), and it is confined
  to the two flags that name a container — `make test`, `make test-race`, `make test-fallback`,
  `make test-contention` and the coverage ratchet all reach the child path, which names none, so a
  gate run on such a checkout is unaffected. —
  [measured 42792be:cmd/testpg/run.go:123-159 · `sed -n '123,159p' cmd/testpg/run.go` → `runChild`
  provisions with `testdb.ServerOptions{ConnCeiling: ceiling}`, carrying no container name]
  [derived → the invalid-directory cases and the child-derives-no-name case in § Test Design]
- **Each checkout now holds its own server, so a host running gates in more than one checkout pays
  for more than one in-RAM cluster at once.** The connection budget each asks for is unchanged —
  `CLIENTS` still sizes each server independently. Mitigation: the tmpfs mount is capped per
  container, so the cost is bounded and stated; the sizing advice (`CLIENTS=N`) is unchanged. —
  [measured 42792be:internal/testdb/server.go · `grep -n 'tmpfsOptions =' internal/testdb/server.go` →
  `tmpfsOptions = "rw,size=512m"`, documented as capping the mount]
- **The validity pattern is this project's copy of a rule the container runtime owns, so it can drift
  out of agreement with it.** Only the *loosening* direction bites: if a runtime's rule ever admits a
  character this pattern refuses, a legitimate checkout is refused here and the developer is stopped
  by our copy rather than by the runtime. The tightening direction is harmless — the runtime remains
  the authority and still refuses, with its own message, exactly as it does today. Mitigation: none
  beyond stating it; the pattern is a diagnosis layer and the cost of it being one version stale is a
  message, not a wrong container. Only podman was measured, and no claim is made about any other
  runtime's rule. —
  [measured d41174f · `podman create --name 'lab game' docker.io/library/alpine:latest true` captured
  to a file → `names must match [a-zA-Z0-9][a-zA-Z0-9_.-]*`, rc 125; the same rule returned over the
  Docker-compat API socket testcontainers dials, by `curl --unix-socket … POST
  '/v1.41/containers/create?name=lab%20game'` → HTTP 500 with that rule in the message body]
- **New comments could carry an outward reference and fail the comment gate.** The derivation's doc
  comments must state the rule in prose rather than quoting a path, a markdown file, a decision anchor
  or a package-qualified symbol of this module from outside its own package. Mitigation: `make
  comment-refs` decides the mechanical half and is part of `make verify`; the narration half is
  review-judged. —
  [measured 42792be:Makefile · `grep -n 'comment-refs' Makefile` → a `.PHONY` target running
  `go run ./cmd/commentrefs`, listed in the `verify` aggregate]
- **Deleting an exported symbol from `internal/testdb` breaks any importer not found.** Mitigation:
  the importer set was read rather than remembered, and the build gate decides it. —
  [measured 42792be · `grep -rn SharedContainerName --include='*.go' .` → `internal/testdb/server.go`,
  `cmd/testpg/run.go`, `cmd/testpg/run_test.go`]
- **The verify-time probe starts long-lived containers with the reaper disabled, so an abandoned probe
  leaks them.** Mitigation: the probe recipe in § Test Design ends by taking both servers down and
  listing the runtime's containers to confirm neither survives. —
  [measured 42792be:cmd/testpg/run.go:236-242 · `sed -n '236,242p' cmd/testpg/run.go` → `runUp` sets
  `TESTCONTAINERS_RYUK_DISABLED` "so the container it creates carries no reap label and outlives this
  process"]

## Test Design

Every claim below is about a test that does not exist yet.

### Subtask 1 — the pure derivation

- **Location:** `cmd/testpg/run_test.go`, beside the package's existing wrapper tests (D9).
- **Entry point:** the derivation function, called directly with a project-directory path; it returns
  the container name or an error.
- **Shape:** a table-driven test with `t.Parallel()`, per the project's default test shape — the
  function touches no process-global state, so parallelism is free.
- **Scenarios:**
  - The spec's worked example for the first checkout: a path whose base name is that project directory
    name yields that name with the test-server suffix. [derived → AC1]
  - The spec's worked example for the sibling checkout: its directory name yields its own name.
    [derived → AC1]
  - Paths differing only in their parent directories, sharing one base name, yield **one and the
    same** name — the assertion is equality between the results, not a match against a literal, so it
    is about the property rather than about the fixture. [derived → AC4]
  - A base name carrying a character the runtime refuses returns an error whose message contains the
    offending directory name. [derived → AC1's "the name under which a server is created, found again
    and removed", read as: a name the runtime cannot create is refused here rather than downstream]
  - A base name whose first character the runtime refuses returns an error — a distinct case from the
    one above, because the runtime's rule constrains the first character more tightly than the rest
    (D6).
  - A path whose base name is empty (a root path) returns an error rather than a name that is the bare
    suffix.
- **Fixtures / helpers:** none. The cases are string literals; no directory is created, nothing is
  provisioned.
- **Red before green:** the table test is written first and must fail for every case before the
  function exists — a compile failure is not the red this establishes, so the function is introduced
  returning a fixed wrong value, the table observed failing by case name, and only then implemented.

### Subtask 2 — the wiring

- **Location:** `cmd/testpg/run_test.go`, extending the existing stub-seam fixture.
- **Entry points:** `run` with `--up`, `run` with `--down`, and `runChild`.
- **Fixtures / helpers:** the existing `stubSeam` gains a working-directory field and returns it from
  the new seam member; no container runtime is reached, exactly as the existing wrapper tests avoid
  one. The `--up`/`--down` cases stay non-parallel, joining the existing ones, because they reach the
  code that writes the process-wide reaper setting.
- **Binding constraint on every case below — the stub seam, never the production one (D10 ground 1).**
  Each new `--up` / `--down` case is driven through `stubSeam`, so the locator members are the stub's
  and `persist` writes nothing to disk. A case that reached `productionSeam` would write a locator file
  into the package's own source directory — the path is relative to the working directory, which for a
  test binary is its package directory, and the repository's scratch-directory ignore rule is anchored
  at the root and does not cover it. That would leave an untracked file behind on every run of the
  suite, in CI and locally alike. This is the same constraint D10 ground 1 states; it is repeated here
  because it binds the implementor writing these cases, not only the decision not to write an
  end-to-end one.
- **Scenarios:**
  - `--up` with a stubbed working directory provisions with the container name the derivation returns
    for that directory — the existing assertion against the deleted constant is replaced by one
    against the derivation's own result for the stub's directory, so the test cannot pass by agreeing
    with a stale literal. [derived → AC1]
  - `--up` with a *different* stubbed working directory provisions with a *different* container name;
    the assertion is inequality between the recorded names, which is the discriminating form for
    "each checkout owns its own server". [derived → AC2, AC3]
  - `--up` with stubbed working directories whose base names are equal but whose parents differ
    provisions with one and the same container name. [derived → AC4]
  - `--down` with a locator present and a reachable server reaches for the container under the name
    the derivation returns for the stub's directory. [derived → AC2]
  - `--down` with no locator provisions nothing and exits 0 even when the stubbed working directory
    would yield an invalid name — pinning D5's ordering, so that the no-op path cannot acquire a new
    failure mode. [derived → AC2]
  - `--up` with a stubbed working directory whose name the runtime refuses exits non-zero, provisions
    nothing, and prints a message containing that directory name. The directory-name substring is the
    load-bearing half of the assertion, not decoration: it is the thing the runtime's own refusal does
    not carry, and therefore the thing the pre-check exists to add (D6, ground 1). [derived → AC1]
  - `--up` with such a directory additionally leaves the process-wide reaper setting as it found it —
    the assertion is over the environment variable before and after the call, which is what pins the
    pre-check's *position* rather than merely its existence (D6, ground 3). A pre-check placed after
    the `Setenv` would pass every other case in this list. [derived → AC1, D6]
  - `--down` with a locator, a reachable server and such a directory exits non-zero and stops nothing.
    [derived → AC1]
  - `--up` whose working-directory lookup itself fails exits non-zero, provisions nothing, and prints
    a message carrying the lookup's own error; the stub seam's working-directory member returns an
    error for this case. [derived → AC1, D4]
  - `--down` with a locator and a reachable server, whose working-directory lookup fails, exits
    non-zero and stops nothing — the same single failure path as the invalid-name case, not a second
    one. [derived → AC1, D4]
  - `--up`'s report line contains the container name it provisioned. [derived → AC1, D8]
  - `runChild` with a stubbed working directory whose name the runtime refuses still runs its child to
    completion and returns the child's own exit code — the wrapper must not acquire a dependency on a
    fact the gate path does not use. [derived → AC3, D5]
- **Red before green:** for the inequality-shaped assertions, the discriminating check is that they
  fail when the derivation is replaced by a constant — the implementor runs the differing-directory
  `--up` cases against a derivation stubbed to ignore its argument, confirms the mutant **builds**, and
  confirms the failure line names the assertion, before restoring it. A test that passes under a
  constant derivation is testing nothing this task changes.

### Verify-time end-to-end probe (AC2, AC3, AC4 — the container-level half)

The seam tests prove that distinct checkouts reach distinct names and that same-named checkouts reach
one and the same name; they start no container, so they say nothing about what the runtime then does
with those names — including the half of AC4 that is not about the name at all, namely that a second
`--up` under a name already taken joins that server rather than raising a second. That last step is
the pre-existing reuse behaviour measured in § Approach, unchanged by this task and therefore
plausible; plausible is not observed, and this is the probe that observes it. The probe closes the gap
without making the package database-backed (D10). Run it once at `/task` verify time, from the
repository root:

1. Build the wrapper: `go build -o tmp/testpg ./cmd/testpg`.
2. Create throwaway project directories under the session scratch directory: two whose base names
   differ — call them the alpha and beta probe directories — and, under a *different* parent, one
   whose base name equals alpha's, the alpha twin. The wrapper needs nothing from any of them but the
   ability to create its own scratch subdirectory there.
3. From the alpha directory, run the built wrapper with `--up --parallel 1` (pinning parallelism keeps
   each probe server at the smallest ceiling the formula floors to, so the probe does not provision
   large servers). Repeat from the beta directory.
4. **AC1 / AC3 evidence:** list the runtime's running containers by name and confirm a container named
   for each probe directory's base name plus the test-server suffix; confirm each probe directory's
   own locator file holds a DSN, and that the DSNs differ.
5. **AC4 evidence:** from the alpha twin, run the built wrapper with `--up --parallel 1`. Confirm the
   runtime's container listing is unchanged — no container was raised for the twin — and that the
   twin's own locator file now holds the DSN alpha's holds. The listing and the DSN are both read: a
   run that silently failed to provision would also leave the listing unchanged, and only the DSN
   distinguishes joining alpha's server from reaching nothing.
6. **AC3 evidence:** from the alpha directory, run the built wrapper with `-- sh -c 'printf %s
   "$LAB_GAME_TEST_DSN"'` and confirm the printed DSN is alpha's, not beta's.
7. **AC2 evidence:** from the alpha directory, run the built wrapper with `--down`. Then confirm
   beta's container is still listed as running, and that a child run from the beta directory still
   prints beta's DSN — a server that is listed but unreachable would pass a listing check and fail
   this one, so both are read.
8. Tear down: run `--down` from the beta directory, then list the runtime's containers again and
   confirm neither probe container survives. The twin needs no `--down` of its own — its name is
   alpha's, so alpha's teardown in step 7 removed the container it joined, and what the twin keeps is
   a stale locator file in a throwaway directory. A leaked container from this probe is a probe
   defect, not a finding (see § Risks).

What a failure of step 7 would mean: the probe directories did not in fact get distinct servers, i.e.
the derivation reached the provisioning call for one path and not the other — which is precisely the
case the `--down` name assertion in subtask 2 is written to catch first. What a failure of step 5
would mean is the opposite fault: the same directory name produced two servers, which is the owner's
name-basis decision (D1) breaking at the runtime rather than at the derivation.

### Subtask 3 — prose

No test. The check is the reviewer's, plus the harness guards CI runs over changed markdown
[measured 42792be:AGENTS.md:138 · `sed -n '138p' AGENTS.md` → the CI job list names "Harness guards
(shellcheck on every script and hook body, the citation guard, the guard suites, the link check)"].

### Gates

The change is Go plus markdown, so the run is `make verify` in full
[measured 42792be:Makefile:41 · `sed -n '41p' Makefile` → `verify: fmt-check build vet lint
file-limits test test-race tidy-check actionlint shellcheck comment-refs import-guard`], plus the
coverage ratchet, which the pre-commit hook takes on its own once `.go` files are staged.

## Open questions

- **RESOLVED — the SPEC-REMIT flag raised on the spec's former AC4 was answered by striking that row,
  and the work it named is now authorised directly.** The flag was raised because that row required every
  live surface stating the old fixed name to state the derivation instead, citing AGENTS.md
  § Propagation Rule step 4 as its own authority — and a criterion restating a rule that holds on
  every branch is re-verified per-AC at `/task` Step 9 and again by `self-review`, at that cost, on every
  run (`spec-writer.md` Rule 1). The orchestrator put it to the owner, who struck the row outright and
  named this design's subtask 3 as the delivery instead. Their words, verbatim: "Spec amendment: AC4 is
  removed outright. The propagation of the KD-20 and test-conventions edits is delivered by the design's
  subtask 3 and by the standing workspace rule regardless."
  [measured 4043659:ai-docs/plans/2026-09-12-test-container-name-per-checkout.spec.md.state.md ·
  `git show b53299f` → that answer, under a round-3 `prior_qa` entry whose question is the flag]. No
  part of this design was weakened to fit the row while it stood, and none was dropped when it fell:
  subtask 3 keeps both files, on the grounds recorded under § Decomposition. Nothing is open here —
  the entry is the record that the flag was raised and answered.
- **A surface outside the project root asserts the falsified claim, and this task cannot reach it.**
  The owner's own per-project memory note on the parallel checkout records the test Postgres as shared
  between the checkouts. It lives under the user's Claude configuration directory, not in this
  repository, and `AGENTS.md` § Permissions denies writes outside the project root. The owner is the
  only one who can update it; naming it here is all this task can do
  [measured 42792be · the session's own injected project-memory index → the entry "Параллельный чекаут
  lab-game2 — второй клон рядом с ~/lab-game: что общее (origin, тестовый Postgres), что своё, и что
  выставлять руками"].
