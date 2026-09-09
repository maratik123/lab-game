// Package repotest resolves the repository root for a test binary,
// regardless of the working directory it was invoked from. Every package
// in this module that needs the repository root during a test imports this
// package rather than declaring its own copy of the ascent — it is the
// module's one file-location-ascent resolver. This package must stay
// exactly two directories below the repository root: Root's ascent
// arithmetic is fixed to that depth.
package repotest

import (
	"path/filepath"
	"runtime"
	"testing"
)

// Root returns an absolute path to the repository root — the directory
// containing go.mod — derived from this file's own location rather than
// the working directory, so it does not depend on how `go test` was
// invoked or which directory the caller started in.
func Root(tb testing.TB) string {
	tb.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		tb.Fatal("repotest.Root: runtime.Caller failed")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
}

// RootPath resolves rel — a "/"-separated path such as a tracked
// configuration file's own path — against the repository root returned by
// Root. It performs no ascent of its own.
func RootPath(tb testing.TB, rel string) string {
	tb.Helper()
	return filepath.Join(Root(tb), filepath.FromSlash(rel))
}
