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

.PHONY: verify fmt-check build vet lint file-limits test test-race tidy-check actionlint shellcheck cover-ratchet comment-refs import-guard test-db-up test-db-down test-fallback test-contention

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
# Job control is on so the load loop is its own process group and the kill
# below reaches the go test inside it rather than only the subshell around
# it; an orphaned loop otherwise dies in the container teardown and fills
# its log with failures that the exhaustion scan then has to read past.
#
# The loop's own failure is swallowed on purpose. The subshell inherits -e,
# so without that the loop would stop at its FIRST failing iteration and the
# target would still report the foreground's status — a green verdict whose
# last seconds ran under no load at all, and the trigger correlates with the
# very condition being probed. The loop is a load source, not an assertion.
#
# The exhaustion scan runs here rather than in whoever reads the logs. A run
# that ran the server out of connections says nothing about contention in
# either direction, so it is neither a pass nor a finding: the target names
# it and exits 2, distinct from the foreground gate's own status.
#
# The target's OWN output — the granted ceiling the
# wrapper echoes, the client count and the pinned parallelism — is captured
# to a third file there and replayed to the terminal afterwards, so the
# arithmetic a run relied on outlives the scrollback it was printed in; a
# terminal is not a record. The foreground's status is captured explicitly
# rather than letting the shell's -e abort the script before the load loop
# is killed, and the wrapper's own status is captured around the capture for
# the same reason; the wrapper reads none of the three logs, and no exit
# status crosses a pipe.
test-contention:
	mkdir -p tmp
	status=0; \
	go run ./cmd/testpg --clients 2 --parallel $(CONTENTION_PARALLEL) -- bash -c '\
	  set -eu -o pipefail; \
	  set -m; \
	  ( while true; do go test -count=1 -parallel $(CONTENTION_PARALLEL) ./internal/ingest/... ./internal/scheduler/... ./internal/store/... ./internal/testdb/... || true; done ) >tmp/test-contention-load.log 2>&1 & \
	  load_pid=$$!; \
	  fg_status=0; \
	  go test -race -count=1 -parallel $(CONTENTION_PARALLEL) ./... >tmp/test-contention-race.log 2>&1 || fg_status=$$?; \
	  kill -- -"$$load_pid" 2>/dev/null || true; \
	  wait "$$load_pid" 2>/dev/null || true; \
	  exhausted=0; \
	  grep -qE "sorry, too many clients already|SQLSTATE 53300" tmp/test-contention-race.log tmp/test-contention-load.log || exhausted=$$?; \
	  echo "test-contention: clients=2 parallel=$(CONTENTION_PARALLEL)"; \
	  if [ "$$exhausted" -eq 0 ]; then \
	    echo "test-contention: INSTRUMENT FAILURE — the server ran out of connections, so this run says nothing about contention either way"; \
	    exit 2; \
	  fi; \
	  if [ "$$exhausted" -ne 1 ]; then \
	    echo "test-contention: INSTRUMENT FAILURE — the exhaustion scan itself failed (grep exit $$exhausted), so a clean result would be a claim about the scan"; \
	    exit 2; \
	  fi; \
	  echo "test-contention: exhaustion scan clean"; \
	  exit "$$fg_status" \
	' > tmp/test-contention.log 2>&1 || status=$$?; \
	cat tmp/test-contention.log; \
	exit "$$status"

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
