---
name: verify-change
description: "Run `go test ./...`; pass an optional filter to run a subset of tests."
argument-hint: "[test-filter]"
disable-model-invocation: true
allowed-tools: Bash(go test *)
---

> Near-stateless: no `.progress.md` discipline applies; re-entry consists of re-invoking the skill.

Run `go test ./... $ARGUMENTS`. If no arguments, runs the full test suite. Report any failures.
