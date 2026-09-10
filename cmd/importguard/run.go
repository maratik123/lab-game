package main

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"
)

// Exit codes: 0 = every rule holds, 1 = at least one violation, 2 =
// the gate could not run at all — an instrument failure, never read
// as a clean tree.
const (
	exitClean    = 0
	exitFindings = 1
	exitFailure  = 2
)

// Rule pairs a package import path with the module prefixes its
// non-test dependency graph may never carry.
type Rule struct {
	// Package is the import path whose non-test dependency graph this
	// rule checks.
	Package string
	// ForbiddenPrefixes are the module path prefixes Package's
	// transitive non-test dependencies may not contain.
	ForbiddenPrefixes []string
}

// productionRules is the table this gate checks against the real
// tree. The command that provisions a test-only database container
// carries the container-runtime dependency by design and is
// deliberately absent from this table — provisioning containers is
// its job, not a leak.
func productionRules() []Rule {
	return []Rule{
		{
			Package:           "github.com/maratik123/lab-game/cmd/bot",
			ForbiddenPrefixes: []string{"github.com/testcontainers/testcontainers-go"},
		},
	}
}

// Violation is one rule's refusal: Rule's own non-test dependency
// graph reached Module through a forbidden prefix.
type Violation struct {
	Rule   string
	Module string
}

// Check classifies deps — a package's own transitive non-test
// dependency list, one import path per entry — against rule,
// returning every entry that carries one of rule's forbidden
// prefixes, sorted for a deterministic report.
func Check(rule Rule, deps []string) []Violation {
	var out []Violation
	for _, d := range deps {
		for _, prefix := range rule.ForbiddenPrefixes {
			if strings.HasPrefix(d, prefix) {
				out = append(out, Violation{Rule: rule.Package, Module: d})
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Module < out[j].Module })
	return out
}

// listDeps runs `go list -deps <pkg>` with dir as the working
// directory and returns each reported import path. A package that
// does not exist is reported as an error rather than an empty,
// silently-clean list — a gate that quietly examines nothing is the
// failure mode this whole gate exists to prevent.
func listDeps(ctx context.Context, dir, pkg string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "go", "list", "-deps", pkg) //nolint:gosec // G204: pkg is a Rule.Package value from this command's own compiled-in rule table or its own test, never external input
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list -deps %s: %w", pkg, err)
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil, nil
	}
	return strings.Split(trimmed, "\n"), nil
}

// run checks every rule in rules against dir's real dependency graph,
// reporting each violation as "<rule package>: forbidden dependency
// <module>" on stdout and returning the exit code.
func run(dir string, rules []Rule, stdout, stderr io.Writer) int {
	ctx := context.Background()

	var anyViolation bool
	for _, rule := range rules {
		deps, err := listDeps(ctx, dir, rule.Package)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "importguard: %v\n", err)
			return exitFailure
		}
		for _, v := range Check(rule, deps) {
			anyViolation = true
			_, _ = fmt.Fprintf(stdout, "%s: forbidden dependency %s\n", v.Rule, v.Module)
		}
	}
	if anyViolation {
		return exitFindings
	}
	return exitClean
}
