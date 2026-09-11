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
- **D5 — Only `runUp` and `runDown` derive a name; `runChild` never does.** A gate run must not be
  able to fail on a fact it does not use, and the fallback container stays anonymous because only a
  server this invocation started may be removed (KD-20). `runUp` derives before it disables the
  reaper or provisions anything; `runDown` derives after its locator and reachability checks, so
  "nothing to remove" stays a side-effect-free exit
  [measured 42792be:cmd/testpg/run.go:286-300 · `sed -n '286,300p' cmd/testpg/run.go` → `sm.locate()`
  returning not-ok prints "nothing to remove" and returns 0 before anything else runs].
- **D6 — A derived name the container runtime would refuse fails cleanly; it is never sanitised.**
  Measured on this host: podman 5.8.2 refuses a name outside `[a-zA-Z0-9][a-zA-Z0-9_.-]*`
  [measured 42792be · `podman create --name 'lab game' docker.io/library/alpine:latest true` →
  `Error: running container create option: names must match [a-zA-Z0-9][a-zA-Z0-9_.-]*: invalid
  argument`, rc 125; the same rejection for `lab-game@2`, `.lab-game`, `-lab-game` and a Cyrillic
  name, while `lab_game.2-x` was accepted]. Sanitising would map distinct project directories onto
  one name and silently re-create the contention this task removes, which contradicts D1. So: a
  non-zero exit whose message names the offending directory and states the rule, on `--up` and
  `--down` alike [derived → the invalid-directory cases in § Test Design].
- **D7 — The validity check is a package-level compiled pattern.** Established precedent in this
  module [measured 42792be:internal/commentref/classify.go:33-42 · `grep -n MustCompile internal/commentref/classify.go`
  → a block of package-level `regexp.MustCompile` values]. No `panic(` or `log.Fatal` call is written,
  so the `panic-gate` hook's pattern is not matched and the panic index gains no row
  [measured 42792be:ai-docs/panic-index.md · `tail -3 ai-docs/panic-index.md` → the table body is the
  row `| — | — | — |`].
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
  read than one 400-line file"]. Both touched files stay inside the reasonable band after the change
  [derived → the `make file-limits` gate named in § Test Design → Gates].
- **D10 — `cmd/testpg`'s test binary stays non-database-backed.** An in-suite test that really starts
  two named servers would have to call `testdb.Main`, which would move the `Binaries` manifest
  constant and therefore re-size every server this project provisions
  [measured 42792be:internal/testdb/server.go · `grep -n '^const Binaries' internal/testdb/server.go`
  → a `Binaries` manifest constant, which the `Ceiling` formula multiplies into every provisioned
  server's connection ceiling]
  [measured 42792be:internal/testdb/server_test.go:207-227 · `sed -n '207,211p' internal/testdb/server_test.go`
  → `TestBinaries_matchesTree` "keeps the Binaries constant honest against the tree"]. The end-to-end
  property is verified instead by the probe § Test Design specifies, run at verify time.

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
| 2 | Wire it: add the working-directory lookup to `seam` and to `productionSeam`; `runUp` derives the name before it disables the reaper or provisions, `runDown` derives it after its locator and reachability checks; each fails non-zero with the derivation's own message; `--up`'s report names the container (D8); delete `testdb.SharedContainerName` and its doc comment. Update the existing `--up` name assertion, add the `--down` name assertion, the invalid-directory failure cases for both paths, and a case pinning that a child gate run derives no name at all. | `cmd/testpg/run.go`, `cmd/testpg/run_test.go`, `internal/testdb/server.go` | 1 |
| 3 | Amend the live prose: KD-20's parenthetical, which names the container as one fixed name for the host, states the derivation instead; the shared-server section of the test conventions gains the derivation and its consequence for two checkouts on one host. | `ai-docs/key-decisions.md`, `ai-docs/go-test-conventions.md` | 2 |

On subtask 3's second file: the test-conventions page names no container, so AC4 does not compel it
[measured 42792be:ai-docs/go-test-conventions.md:43-44 · `sed -n '43,44p' ai-docs/go-test-conventions.md`
→ "`make test-db-up` creates the named long-lived server and records its DSN in the ignored scratch
directory", with the name itself never written]. It is amended anyway because it is the developer-facing
home for `make test-db-up`, and the parallel-checkout consequence this change delivers has no other live
page to land on — a design decision, within Scope item 1, not an AC4 obligation.

Surfaces deliberately **not** amended, with the evidence:

- `AGENTS.md` names no container
  [measured 42792be · `grep -rn 'lab-game-test-postgres' --include='*' . | grep -v '^\./\.git/'` →
  `internal/testdb/server.go`, `ai-docs/key-decisions.md`, and this task's own spec and interview-state
  files]. Its § Build & Test lines describe `make test-db-up` as bringing up a long-lived server and
  the wrapper as preferring one that answers and admits the run — each still true after the change
  [measured 42792be:AGENTS.md:48-49,76-78 · `sed -n '48,49p;76,78p' AGENTS.md` → "bring up a long-lived
  shared test server (CLIENTS=N sizes it)", "remove it — no reaper will", and "otherwise a long-lived
  server from `make test-db-up` if that one answers and admits the run"].
- `ai-docs/context-status.md` names `SharedContainerName` in its entry for the shared-test-server
  task, and is a history surface, not a live claim
  [measured 42792be:ai-docs/context-status.md:1-5 · `head -5 ai-docs/context-status.md` → "The
  detailed, append-only implementation log: one entry per completed task"]. `AGENTS.md` § Propagation
  Rule step 4 exempts history surfaces by name.
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
    is about the property rather than about the fixture. [derived → AC5]
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
- **Scenarios:**
  - `--up` with a stubbed working directory provisions with the container name the derivation returns
    for that directory — the existing assertion against the deleted constant is replaced by one
    against the derivation's own result for the stub's directory, so the test cannot pass by agreeing
    with a stale literal. [derived → AC1]
  - `--up` with a *different* stubbed working directory provisions with a *different* container name;
    the assertion is inequality between the recorded names, which is the discriminating form for
    "each checkout owns its own server". [derived → AC2, AC3]
  - `--up` with stubbed working directories whose base names are equal but whose parents differ
    provisions with one and the same container name. [derived → AC5]
  - `--down` with a locator present and a reachable server reaches for the container under the name
    the derivation returns for the stub's directory. [derived → AC2]
  - `--down` with no locator provisions nothing and exits 0 even when the stubbed working directory
    would yield an invalid name — pinning D5's ordering, so that the no-op path cannot acquire a new
    failure mode. [derived → AC2]
  - `--up` with a stubbed working directory whose name the runtime refuses exits non-zero, provisions
    nothing, and prints a message containing that directory name. [derived → AC1]
  - `--down` with a locator, a reachable server and such a directory exits non-zero and stops nothing.
    [derived → AC1]
  - `--up`'s report line contains the container name it provisioned. [derived → AC1, D8]
  - `runChild` with a stubbed working directory whose name the runtime refuses still runs its child to
    completion and returns the child's own exit code — the wrapper must not acquire a dependency on a
    fact the gate path does not use. [derived → AC3, D5]
- **Red before green:** for the inequality-shaped assertions, the discriminating check is that they
  fail when the derivation is replaced by a constant — the implementor runs the differing-directory
  `--up` cases against a derivation stubbed to ignore its argument, confirms the mutant **builds**, and
  confirms the failure line names the assertion, before restoring it. A test that passes under a
  constant derivation is testing nothing this task changes.

### Verify-time end-to-end probe (AC2, AC3 — the container-level half)

The seam tests prove that distinct checkouts reach distinct names; they start no container, so they
say nothing about what the runtime then does. The probe below closes that gap without making the
package database-backed (D10). Run it once at `/task` verify time, from the repository root:

1. Build the wrapper: `go build -o tmp/testpg ./cmd/testpg`.
2. Create throwaway project directories under the session scratch directory whose base names differ —
   call them the alpha and beta probe directories — and note that the wrapper needs nothing from them
   but the ability to create its own scratch subdirectory there.
3. From the alpha directory, run the built wrapper with `--up --parallel 1` (pinning parallelism keeps
   each probe server at the smallest ceiling the formula floors to, so the probe does not provision
   large servers). Repeat from the beta directory.
4. **AC1 / AC3 evidence:** list the runtime's running containers by name and confirm a container named
   for each probe directory's base name plus the test-server suffix; confirm each probe directory's
   own locator file holds a DSN, and that the DSNs differ.
5. **AC3 evidence:** from the alpha directory, run the built wrapper with `-- sh -c 'printf %s
   "$LAB_GAME_TEST_DSN"'` and confirm the printed DSN is alpha's, not beta's.
6. **AC2 evidence:** from the alpha directory, run the built wrapper with `--down`. Then confirm
   beta's container is still listed as running, and that a child run from the beta directory still
   prints beta's DSN — a server that is listed but unreachable would pass a listing check and fail
   this one, so both are read.
7. Tear down: run `--down` from the beta directory, then list the runtime's containers again and
   confirm neither probe container survives. A leaked container from this probe is a probe defect, not
   a finding (see § Risks).

What a failure of step 6 would mean: the probe directories did not in fact get distinct servers, i.e.
the derivation reached the provisioning call for one path and not the other — which is precisely the
case the `--down` name assertion in subtask 2 is written to catch first.

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

- **SPEC-REMIT: AC4 — "…states how the name is derived instead, per AGENTS.md § Propagation Rule step
  4" — the row cites a standing workspace rule as its own authority — the outcome it appears to
  protect is that no live document keeps asserting the old fixed name.** That outcome is delivered by
  subtask 3 regardless, and the row binds until the orchestrator restates or strikes it, so nothing in
  this design is weakened to fit it. Flagged only because a criterion that restates a rule holding on
  every branch is re-verified per-AC at `/task` Step 9 and again by `self-review`, at that cost, on
  every run (`spec-writer.md` Rule 1).
- **A surface outside the project root asserts the falsified claim, and this task cannot reach it.**
  The owner's own per-project memory note on the parallel checkout records the test Postgres as shared
  between the checkouts. It lives under the user's Claude configuration directory, not in this
  repository, and `AGENTS.md` § Permissions denies writes outside the project root. The owner is the
  only one who can update it; naming it here is all this task can do
  [measured 42792be · the session's own injected project-memory index → the entry "Параллельный чекаут
  lab-game2 — второй клон рядом с ~/lab-game: что общее (origin, тестовый Postgres), что своё, и что
  выставлять руками"].
- **Should the anonymous fallback container also carry a per-checkout name?** The design says no (D5,
  and KD-20's "anonymous because only a server this invocation started may be removed"), and the
  spec's Out of scope does not reach it. Raised only so that a later reader does not take the
  omission for an oversight; no answer is needed to proceed.
