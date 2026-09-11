// Package leaktest wraps a test binary's entry point with a check that
// no goroutine survives its package's tests. TestMain hands the check
// the package's testing.M and the function that used to own the entry
// point — a database provisioner, or the standard library's own M.Run —
// and, once that function returns zero, verifies that no goroutine
// outside an explicitly declared ignore set is still running. A leak, a
// malformed ignore entry, or an ignore entry that excuses nothing makes
// the check fail the package's test binary.
package leaktest

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"testing"

	"go.uber.org/goleak"
)

// Ignore excuses one goroutine that neither this module's code nor its
// test fixtures can stop — one a dependency starts for the life of the
// process. A keep-alive connection a test opened, a pool a test did not
// close, a handler still running — each has an owner that can end it,
// and ending it is the fix, not an Ignore entry.
type Ignore struct {
	// Function is the fully qualified function goleak matches against a
	// goroutine's stack — the same name goleak.IgnoreTopFunction and
	// goleak.IgnoreAnyFunction take.
	Function string
	// Anywhere makes Function match at any frame of the stack rather
	// than only at the top.
	Anywhere bool
	// Reason states why the goroutine Function matches is not a leak.
	Reason string
}

// Main hands m to run and, once run returns zero, checks that no
// goroutine outside ignore is still running; a leak, or an ignore entry
// this run cannot admit, makes it return non-zero instead of run's own
// exit code. Callers pass its result to os.Exit:
//
//	func TestMain(m *testing.M) {
//		os.Exit(leaktest.Main(m, run))
//	}
func Main(m *testing.M, run func(*testing.M) int, ignore ...Ignore) int {
	return check(m, run, os.Stderr, modulePath, func() bool { return filtered(flag.Lookup) }, ignore)
}

// check is Main's body, with every dependency it needs passed in
// separately so each can be driven on its own.
func check(
	m *testing.M,
	run func(*testing.M) int,
	out io.Writer,
	modulePath func() (string, bool),
	filtered func() bool,
	ignore []Ignore,
) int {
	if run == nil {
		_, _ = fmt.Fprintln(out, "leaktest: refused: no runner was given")
		return 1
	}
	path, ok := modulePath()
	if !ok {
		_, _ = fmt.Fprintln(out, "leaktest: refused: this test binary's module path could not be determined")
		return 1
	}
	for _, e := range ignore {
		if e.Reason == "" {
			_, _ = fmt.Fprintf(out, "leaktest: refused: ignore entry for %q has a blank Reason\n", e.Function)
			return 1
		}
		if e.Function == "" {
			_, _ = fmt.Fprintln(out, "leaktest: refused: an ignore entry has a blank Function")
			return 1
		}
		if namesModuleCode(e.Function, path) {
			_, _ = fmt.Fprintf(out, "leaktest: refused: ignore entry %q names this module's own code\n", e.Function)
			return 1
		}
	}

	code := run(m)
	if code != 0 {
		return code
	}

	if err := goleak.Find(toOptions(ignore)...); err != nil {
		_, _ = fmt.Fprintln(out, "leaktest: goroutines still running after this package's tests finished:")
		_, _ = fmt.Fprintln(out, err)
		return 1
	}

	if filtered() {
		_, _ = fmt.Fprintln(out, "leaktest: filtered run — skipping the per-entry check of the declared ignore set")
		return 0
	}

	bad := false
	for i, e := range ignore {
		err := goleak.Find(optionsExcept(ignore, i)...)
		switch {
		case err == nil:
			_, _ = fmt.Fprintf(out, "leaktest: ignore entry %q is not needed: %s\n", e.Function, e.Reason)
			bad = true
		case namesModuleCodeIn(err.Error(), path):
			_, _ = fmt.Fprintf(out, "leaktest: ignore entry %q excuses this module's own code:\n%s\n", e.Function, err)
			bad = true
		}
	}
	if bad {
		return 1
	}

	return 0
}

// toOption converts one Ignore entry into the matching goleak.Option.
func toOption(e Ignore) goleak.Option {
	if e.Anywhere {
		return goleak.IgnoreAnyFunction(e.Function)
	}
	return goleak.IgnoreTopFunction(e.Function)
}

// toOptions converts every entry of ignore into its matching
// goleak.Option, in order.
func toOptions(ignore []Ignore) []goleak.Option {
	opts := make([]goleak.Option, len(ignore))
	for i, e := range ignore {
		opts[i] = toOption(e)
	}
	return opts
}

// optionsExcept is toOptions with the entry at skip left out — the set
// this package's per-entry check runs goleak.Find with, to tell whether
// that one entry is excusing a goroutine no other entry also excuses.
func optionsExcept(ignore []Ignore, skip int) []goleak.Option {
	opts := make([]goleak.Option, 0, len(ignore)-1)
	for i, e := range ignore {
		if i == skip {
			continue
		}
		opts = append(opts, toOption(e))
	}
	return opts
}

// modulePath returns this test binary's module path, read from its own
// build information rather than a literal, so it holds for every test
// binary this check runs inside.
func modulePath() (string, bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Path == "" {
		return "", false
	}
	return info.Main.Path, true
}

// filtered reports whether this run's flags may have kept some of the
// package's tests, and therefore some of the goroutines they would have
// started, from ever running: test.run, test.skip or test.list set, or
// test.short true. It reads flags through lookup rather than the flag
// package's global state directly, so it can be driven with a fake
// lookup in tests; a flag lookup does not find counts as unset.
func filtered(lookup func(name string) *flag.Flag) bool {
	nonEmpty := func(name string) bool {
		f := lookup(name)
		return f != nil && f.Value.String() != ""
	}
	isTrue := func(name string) bool {
		f := lookup(name)
		return f != nil && f.Value.String() == "true"
	}
	return nonEmpty("test.run") || nonEmpty("test.skip") || nonEmpty("test.list") || isTrue("test.short")
}

// namesModuleCode reports whether name is a function of the module at
// modulePath: modulePath itself followed by "/" (a symbol of a
// subpackage) or "." (a symbol of the module's own root package).
func namesModuleCode(name, modulePath string) bool {
	return strings.HasPrefix(name, modulePath+"/") || strings.HasPrefix(name, modulePath+".")
}

// namesModuleCodeIn reports whether report — goleak's formatted account
// of a set of still-running goroutines — names any function of the
// module at modulePath, in a call frame or in a "created by" line.
func namesModuleCodeIn(report, modulePath string) bool {
	for _, line := range strings.Split(report, "\n") {
		if strings.HasPrefix(line, "\t") {
			continue // a source file:line, not a function name
		}
		name := strings.TrimPrefix(line, "created by ")
		if i := strings.Index(name, " in goroutine "); i >= 0 {
			name = name[:i]
		}
		if i := strings.Index(name, "("); i >= 0 {
			name = name[:i]
		}
		if namesModuleCode(name, modulePath) {
			return true
		}
	}
	return false
}
