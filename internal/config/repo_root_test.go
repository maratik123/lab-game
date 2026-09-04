package config

import (
	"path/filepath"
	"runtime"
	"testing"
)

// repoRootPath resolves rel (a "/"-separated path such as "config/balance.yaml")
// against the repository root, regardless of the test binary's working
// directory. internal/config is exactly two directories below the root, so
// the root is derived from this file's own location rather than assumed
// from os.Getwd (AGENTS.md § Code Style — Determinism: no reliance on ambient state that a
// test runner may not control).
func repoRootPath(t *testing.T, rel string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("repoRootPath: runtime.Caller failed")
	}
	// thisFile: <root>/internal/config/repo_root_test.go
	root := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
	return filepath.Join(root, filepath.FromSlash(rel))
}
