# The single entry point every runner shares.
#
# `make verify` is the local aggregate: it runs every gate this project owns, in
# the order in which a failure is cheapest to read. CI never runs `verify` — it
# invokes the same sub-targets from its paths-filtered jobs, so a local run and a
# CI run cannot disagree about what any gate's command is.
#
# One gate CI reaches by another route, deliberately:
#   * actionlint — CI uses `reviewdog/action-actionlint@v1`, because the binary is
#     not preinstalled on `ubuntu-latest`. `make actionlint` is the local path.
#
# No recipe swallows a failure: SHELL/.SHELLFLAGS below put `pipefail` in force for
# every recipe, and no recipe absorbs a non-zero exit status.

SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
.NOTPARALLEL:

# `tmp/` is pruned from the two find-based recipes: it is the ignored scratch
# directory every gate log and throwaway probe is written to, so a local
# `make verify` and a CI run — which never sees it — agree. The
# Go toolchain is NOT taught to skip it: a stray Go source file there breaks
# `go build ./...` loudly, which is the cheap direction.

# Hard file-size limits — raw lines, comments and blanks included, part of a
# wider four-band ladder.
GO_MAX_LINES ?= 1000
GO_MAX_TEST_LINES ?= 1500

# CLIENTS sizes test-db-up's server for the number of whole-module test runs
# it must admit AT THE SAME TIME; the default of 1 is a server for one gate
# run at a time. CONTENTION_PARALLEL pins the parallelism test-contention's
# two concurrent children share with the ceiling that sizes their server —
# the ceiling arithmetic's product form is correct only while every client
# shares one parallel value.
CLIENTS ?= 1
CONTENTION_PARALLEL ?= $(shell nproc 2>/dev/null || echo 4)

# Bounds how long test-contention waits for its load loop to notice the stop
# request before falling back to killing the loop's process group.
CONTENTION_LOAD_STOP_TIMEOUT ?= 120

.PHONY: verify fmt-check build vet lint file-limits test test-race tidy-check actionlint shellcheck cover-ratchet comment-refs import-guard test-db-up test-db-down test-fallback test-contention test-contention-stop-probe

verify: fmt-check build vet lint file-limits test test-race tidy-check actionlint shellcheck comment-refs import-guard

fmt-check:
	golangci-lint fmt -d

build:
	go build ./...

vet:
	go vet ./...

lint:
	golangci-lint run

file-limits:
	find . -path ./.git -prune -o -path ./tmp -prune -o -name '*.go' -exec awk \
	  '{n[FILENAME]++} END{rc=0; for (k in n) {lim=(k ~ /_test\.go$$/)?$(GO_MAX_TEST_LINES):$(GO_MAX_LINES); if (n[k]>lim) {printf "%s: %d lines exceeds hard limit %d\n", k, n[k], lim; rc=1}} exit rc}' {} +

# Both route through the provisioning wrapper: it reuses an already-set DSN
# or an already-running long-lived server (test-db-up) unchanged, and
# otherwise provisions and removes its own sized, anonymous container —
# a bare `go test` still starts one container per database-backed binary,
# which is test-fallback's own gate.
test:
	go run ./cmd/testpg -- go test ./...

test-race:
	go run ./cmd/testpg -- go test -race ./...

# Creates (or reuses) the long-lived shared server sized for CLIENTS
# concurrent whole-module runs, and prints its DSN.
test-db-up:
	go run ./cmd/testpg --up --clients $(CLIENTS)

# Removes only a server this project's own target started.
test-db-down:
	go run ./cmd/testpg --down

# The fallback path's own gate: with the DSN variable explicitly cleared, a
# whole-module test run still provisions one container per database-backed
# test binary and passes, exercising the path every other target here has
# stopped exercising. `-count=1` is what makes that a gate rather than a
# report: the DSN variable is read before m.Run() opens the log the go command
# reads to decide cache validity, so it is in no cache key and this run shares
# cache entries with the shared-server gates. Without the flag a second
# invocation returns green having started no container at all — measured.
test-fallback:
	LAB_GAME_TEST_DSN= go test -count=1 ./...

# Induces cross-package load against one shared server and requires the
# whole-module race gate to stay green under it. One wrapper invocation
# sizes a server for TWO clients at the SAME pinned parallelism (the
# ceiling's product form is correct only while every client shares one
# parallel value); its child backgrounds a load loop of the database-backed
# packages' own tests and foregrounds the race gate, both logging to files
# under the ignored scratch directory.
#
# BOTH children carry -count=1, and for the same reason: the DSN reaches no
# cache key, so a repeat of either is answered from an earlier run that may
# have taken the other provisioning path. Without it the load loop loads
# nothing past its first iteration, and — the sharper half — the foreground
# gate, whose exit status IS this probe's verdict, reports on a run that
# never happened under load. That is not the ordinary-gate case where a
# cached pass is a real pass: this gate asserts a property of the run's
# conditions, which the cache key cannot see.
#
# The load loop stops itself between iterations rather than being signalled
# at an arbitrary instant: a signal landing mid-iteration, inside a test's
# own container provisioning, can arrive after the container is created but
# before its cleanup is registered and before its session's reaper is up,
# and nothing then ever removes it (test-contention-stop-probe exercises
# exactly this window). So the foreground run's completion asks the loop to
# stop by creating a file the loop checks between iterations, and only a
# loop still running past the bound below is signalled at all. Job control
# stays on for that bounded fallback: it puts the loop in its own process
# group so the kill reaches the go test inside it rather than only the
# subshell around it.
#
# The loop's own failure is swallowed on purpose. The subshell inherits -e,
# so without that the loop would stop at its FIRST failing iteration and the
# target would still report the foreground's status — a green verdict whose
# last seconds ran under no load at all, and the trigger correlates with the
# very condition being probed. The loop is a load source, not an assertion.
#
# The classification runs here rather than in whoever reads the logs, and it
# does two things: it scans both child logs for the shared server's own
# failure signatures (exhausted connections, exhausted disk, crash recovery
# or shutdown), and — once the children have exited — it probes the shared
# server directly for liveness, because a way the server dies without
# leaving any of those signatures behind is still a way the run says nothing
# about contention. Either kind of finding is neither a pass nor a finding:
# the target names it and exits 2, distinct from the foreground gate's own
# status. The two binaries below are built rather than run through `go run`
# because a `go run` invocation flattens a non-zero child exit status to 1,
# which would make the exit-2 half of this contract unreachable by its
# caller.
#
# The target's OWN output — the granted ceiling the
# wrapper echoes, the client count and the pinned parallelism — is captured
# to a third file there and replayed to the terminal afterwards, so the
# arithmetic a run relied on outlives the scrollback it was printed in; a
# terminal is not a record. The foreground's status is captured explicitly
# rather than letting the shell's -e abort the script before the load loop
# is stopped, and the wrapper's own status is captured around the capture for
# the same reason; the wrapper reads none of the three logs, and no exit
# status crosses a pipe.
test-contention:
	mkdir -p tmp
	go build -o tmp/testpg ./cmd/testpg
	go build -o tmp/contentionverdict ./cmd/contentionverdict
	status=0; \
	tmp/testpg --clients 2 --parallel $(CONTENTION_PARALLEL) -- bash -c '\
	  set -eu -o pipefail; \
	  set -m; \
	  rm -f tmp/test-contention-load.stop; \
	  ( while [ ! -e tmp/test-contention-load.stop ]; do go test -count=1 -parallel $(CONTENTION_PARALLEL) ./internal/ingest/... ./internal/scheduler/... ./internal/store/... ./internal/testdb/... || true; done ) >tmp/test-contention-load.log 2>&1 & \
	  load_pid=$$!; \
	  fg_status=0; \
	  go test -race -count=1 -parallel $(CONTENTION_PARALLEL) ./... >tmp/test-contention-race.log 2>&1 || fg_status=$$?; \
	  touch tmp/test-contention-load.stop; \
	  waited=0; \
	  while kill -0 "$$load_pid" 2>/dev/null && [ "$$waited" -lt $(CONTENTION_LOAD_STOP_TIMEOUT) ]; do sleep 1; waited=$$((waited + 1)); done; \
	  if kill -0 "$$load_pid" 2>/dev/null; then \
	    echo "test-contention: load loop did not stop within $(CONTENTION_LOAD_STOP_TIMEOUT)s of its stop request; killing its process group, which can leave a test container behind"; \
	    kill -- -"$$load_pid" 2>/dev/null || true; \
	  fi; \
	  wait "$$load_pid" 2>/dev/null || true; \
	  echo "test-contention: clients=2 parallel=$(CONTENTION_PARALLEL)"; \
	  cls=0; \
	  tmp/contentionverdict -status "$$fg_status" -dsn "$${LAB_GAME_TEST_DSN:-}" tmp/test-contention-race.log tmp/test-contention-load.log || cls=$$?; \
	  exit "$$cls" \
	' > tmp/test-contention.log 2>&1 || status=$$?; \
	cat tmp/test-contention.log; \
	exit "$$status"

# Runs test-contention with its load loop stopped, on purpose, inside a load
# binary's container provisioning, and exits non-zero when that stop leaves a
# container or volume of the run's session behind (exit 2 when the window was
# not hit, which proves nothing either way). Needs podman; not part of verify.
test-contention-stop-probe:
	bash ai-docs/scripts/probe-contention-stop.sh

# `git diff -- go.sum` exits 128 while the module has no dependencies and the
# file therefore does not exist, so ask git about worktree state instead — that
# also catches a go.sum that tidy has just created. (Moved here from the CI
# workflow, which now reaches this gate through make.)
tidy-check:
	go mod tidy
	test -z "$$(git status --porcelain -- go.mod go.sum)" \
	  || { echo 'go mod tidy rewrote go.mod or go.sum; commit the result'; exit 1; }

actionlint:
	actionlint .github/workflows/*.yml

shellcheck:
	find . -path ./.git -prune -o -path ./tmp -prune -o -name '*.sh' -exec shellcheck -s bash {} +

# Check-only: never writes the ratchet file, never stages. The pre-commit hook
# runs the same script in raise mode. Not part of `verify` — it re-runs the
# whole suite under coverage instrumentation, and `verify` already ran it twice.
cover-ratchet:
	.githooks/coverage-ratchet.sh --check

# The comment reference gate, over the whole tracked gated set: no comment in
# a gated file carries an outward reference.
comment-refs:
	go run ./cmd/commentrefs

# The transitive-dependency gate: a package named in the rule table's
# non-test dependency graph never carries a forbidden module prefix —
# today, that the bot process never reaches the container-runtime
# module provisioning a test-only database depends on.
import-guard:
	go run ./cmd/importguard
