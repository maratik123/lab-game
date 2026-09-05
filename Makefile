# Makefile — the single entry point every runner shares (AGENTS.md § Build & Test).
#
# `make verify` is the local aggregate: it runs every gate this project owns, in
# the order in which a failure is cheapest to read. CI never runs `verify` — it
# invokes the same sub-targets from its paths-filtered jobs, so a local run and a
# CI run cannot disagree about what any gate's command is.
#
# Two gates CI reaches by another route, deliberately (KD-10):
#   * actionlint — CI uses `reviewdog/action-actionlint@v1`, because the binary is
#     not preinstalled on `ubuntu-latest`. `make actionlint` is the local path.
#   * shellcheck over `.claude/**` — the Harness-guards job keeps its inline step,
#     so the harness guards stay outside this file.
#
# No recipe swallows a failure: SHELL/.SHELLFLAGS below put `pipefail` in force for
# every recipe, and no recipe absorbs a non-zero exit status.

SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
.NOTPARALLEL:

# `tmp/` is pruned from the two find-based recipes: it is the ignored scratch
# directory every gate log and throwaway probe is written to (AGENTS.md § Build &
# Test), so a local `make verify` and a CI run — which never sees it — agree. The
# Go toolchain is NOT taught to skip it: a stray `.go` file there breaks
# `go build ./...` loudly, which is the cheap direction.

# Hard file-size limits — raw lines, comments and blanks included.
# See ai-docs/code-style.md § File size for the full four-band ladder.
GO_MAX_LINES ?= 1000
GO_MAX_TEST_LINES ?= 1500

.PHONY: verify fmt-check build vet lint file-limits test test-race tidy-check actionlint shellcheck cover-ratchet

verify: fmt-check build vet lint file-limits test test-race tidy-check actionlint shellcheck

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

test:
	go test ./...

test-race:
	go test -race ./...

# `git diff -- go.sum` exits 128 while the module has no dependencies and the
# file therefore does not exist, so ask git about worktree state instead — that
# also catches a go.sum that tidy has just created. (Moved here from
# .github/workflows/ci.yml, which now reaches this gate through make.)
tidy-check:
	go mod tidy
	test -z "$$(git status --porcelain -- go.mod go.sum)" \
	  || { echo 'go mod tidy rewrote go.mod or go.sum; commit the result'; exit 1; }

actionlint:
	actionlint .github/workflows/*.yml

shellcheck:
	find . -path ./.git -prune -o -path ./tmp -prune -o -name '*.sh' -exec shellcheck -s bash {} +
	shellcheck -s bash .githooks/pre-commit

# Check-only: never writes the ratchet file, never stages. The pre-commit hook
# runs the same script in raise mode. Not part of `verify` — it re-runs the
# whole suite under coverage instrumentation, and `verify` already ran it twice.
cover-ratchet:
	.githooks/coverage-ratchet.sh --check
