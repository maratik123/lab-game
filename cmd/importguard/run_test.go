package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/maratik123/lab-game/internal/repotest"
)

func TestCheck_SyntheticListContainingForbiddenModule(t *testing.T) {
	t.Parallel()
	rule := Rule{
		Package:           "example.com/pkg",
		ForbiddenPrefixes: []string{"github.com/testcontainers/testcontainers-go"},
	}
	deps := []string{
		"fmt",
		"github.com/testcontainers/testcontainers-go",
		"github.com/testcontainers/testcontainers-go/modules/postgres",
	}

	got := Check(rule, deps)
	if len(got) != 2 {
		t.Fatalf("Check() = %v, want 2 violations", got)
	}
	for _, v := range got {
		if v.Rule != rule.Package {
			t.Errorf("violation.Rule = %q, want %q", v.Rule, rule.Package)
		}
		if !strings.HasPrefix(v.Module, "github.com/testcontainers/testcontainers-go") {
			t.Errorf("violation.Module = %q, want a testcontainers-go path", v.Module)
		}
	}
}

func TestCheck_SameListAgainstARuleThatDoesNotForbidIt(t *testing.T) {
	t.Parallel()
	rule := Rule{
		Package:           "example.com/pkg",
		ForbiddenPrefixes: []string{"example.com/some-other-forbidden-module"},
	}
	deps := []string{
		"fmt",
		"github.com/testcontainers/testcontainers-go",
	}

	if got := Check(rule, deps); len(got) != 0 {
		t.Errorf("Check() = %v, want no violations", got)
	}
}

func TestCheck_EmptyList(t *testing.T) {
	t.Parallel()
	rule := Rule{
		Package:           "example.com/pkg",
		ForbiddenPrefixes: []string{"github.com/testcontainers/testcontainers-go"},
	}

	if got := Check(rule, nil); len(got) != 0 {
		t.Errorf("Check() = %v, want no violations on an empty list", got)
	}
}

func TestRun_RealTreeIsClean(t *testing.T) {
	t.Parallel()
	root := repotest.Root(t)

	var stdout, stderr bytes.Buffer
	code := run(root, productionRules(), &stdout, &stderr)

	if code != exitClean {
		t.Errorf("run() = %d, want %d (clean); stdout = %q, stderr = %q", code, exitClean, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty on a clean run", stdout.String())
	}
}

func TestRun_RuleNamingAPackageThatDoesNotExist(t *testing.T) {
	t.Parallel()
	root := repotest.Root(t)
	rules := []Rule{{
		Package:           "github.com/maratik123/lab-game/cmd/this-package-does-not-exist",
		ForbiddenPrefixes: []string{"anything"},
	}}

	var stdout, stderr bytes.Buffer
	code := run(root, rules, &stdout, &stderr)

	if code != exitFailure {
		t.Errorf("run() = %d, want %d (failure) — a rule naming a nonexistent package must error, not silently pass", code, exitFailure)
	}
	if stderr.Len() == 0 {
		t.Error("stderr empty, want a diagnostic naming the failure")
	}
}

func TestRun_SyntheticViolationEndToEnd(t *testing.T) {
	t.Parallel()
	root := repotest.Root(t)
	// The test-server provisioning wrapper genuinely depends on the
	// container-runtime module by design — using it as the rule's own
	// target proves the red case end to end, through a real go list
	// -deps call, without touching the tree.
	rules := []Rule{{
		Package:           "github.com/maratik123/lab-game/cmd/testpg",
		ForbiddenPrefixes: []string{"github.com/testcontainers/testcontainers-go"},
	}}

	var stdout, stderr bytes.Buffer
	code := run(root, rules, &stdout, &stderr)

	if code != exitFindings {
		t.Errorf("run() = %d, want %d (findings); stdout = %q", code, exitFindings, stdout.String())
	}
	if !strings.Contains(stdout.String(), "testcontainers") {
		t.Errorf("stdout = %q, want it to name the forbidden module", stdout.String())
	}
}
