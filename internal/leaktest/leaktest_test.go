package leaktest

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/maratik123/lab-game/internal/repotest"
)

// fakeModule is a module path used by cases that exercise the refusal and
// per-entry logic without depending on this repository's own module path.
const fakeModule = "example.com/gl"

func fakeModulePath() (string, bool) { return fakeModule, true }
func noModulePath() (string, bool)   { return "", false }

func unfiltered() bool { return false }
func isFiltered() bool { return true }

func runnerReturning(code int) func(*testing.M) int {
	return func(*testing.M) int { return code }
}

// blockOnChannelA and blockOnChannelB each block on their own channel,
// under a name distinct from the other, so a report naming the function
// at the top of a goroutine's stack tells the two apart.
func blockOnChannelA(ch <-chan struct{}) { <-ch }
func blockOnChannelB(ch <-chan struct{}) { <-ch }

// startBlocked starts a goroutine blocked on a channel receive and
// returns a release function that unblocks it and waits for it to exit.
func startBlocked(t *testing.T) func() {
	t.Helper()
	done := make(chan struct{})
	started := make(chan struct{})
	exited := make(chan struct{})
	go func() {
		close(started)
		<-done
		close(exited)
	}()
	<-started
	return func() {
		close(done)
		<-exited
	}
}

func TestCheck_CleanRun(t *testing.T) {
	var out bytes.Buffer
	code := check(nil, runnerReturning(0), &out, fakeModulePath, unfiltered, nil)
	if code != 0 {
		t.Errorf("check() = %d, want 0; output: %s", code, out.String())
	}
	if out.Len() != 0 {
		t.Errorf("output = %q, want empty", out.String())
	}
}

func TestCheck_RunnerNonZeroSkipsTheLeakCheck(t *testing.T) {
	release := startBlocked(t)
	defer release()

	var out bytes.Buffer
	const wantCode = 7
	code := check(nil, runnerReturning(wantCode), &out, fakeModulePath, unfiltered, nil)
	if code != wantCode {
		t.Errorf("check() = %d, want %d", code, wantCode)
	}
	if out.Len() != 0 {
		t.Errorf("output = %q, want empty — the leak check must not run when the runner itself failed", out.String())
	}
}

func TestCheck_TwoGoroutinesLeftRunning(t *testing.T) {
	chA := make(chan struct{})
	chB := make(chan struct{})
	doneA := make(chan struct{})
	doneB := make(chan struct{})
	go func() { blockOnChannelA(chA); close(doneA) }()
	go func() { blockOnChannelB(chB); close(doneB) }()
	defer func() {
		close(chA)
		close(chB)
		<-doneA
		<-doneB
	}()

	var out bytes.Buffer
	code := check(nil, runnerReturning(0), &out, fakeModulePath, unfiltered, nil)
	if code == 0 {
		t.Fatalf("check() = 0, want non-zero; output: %s", out.String())
	}
	report := out.String()
	if !strings.Contains(report, "blockOnChannelA") {
		t.Errorf("report does not name blockOnChannelA:\n%s", report)
	}
	if !strings.Contains(report, "blockOnChannelB") {
		t.Errorf("report does not name blockOnChannelB:\n%s", report)
	}
	if strings.Count(report, "created by") != 2 {
		t.Errorf("report does not carry a created by line for each goroutine:\n%s", report)
	}
}

func TestCheck_EntryInUseExcusesStdlibGoroutine(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	entries := []Ignore{
		{Function: "net/http.(*Server).Serve", Anywhere: true, Reason: "an httptest.Server's own serve goroutine"},
	}
	var out bytes.Buffer
	code := check(nil, runnerReturning(0), &out, fakeModulePath, unfiltered, entries)
	if code != 0 {
		t.Errorf("check() = %d, want 0; output: %s", code, out.String())
	}
}

func TestCheck_EntryNotNeededAlongsideUsedEntry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	entries := []Ignore{
		{Function: "net/http.(*Server).Serve", Anywhere: true, Reason: "an httptest.Server's own serve goroutine"},
		{Function: "no.such/pkg.Function", Reason: "not on any stack"},
	}
	var out bytes.Buffer
	code := check(nil, runnerReturning(0), &out, fakeModulePath, unfiltered, entries)
	if code == 0 {
		t.Fatalf("check() = 0, want non-zero; output: %s", out.String())
	}
	if !strings.Contains(out.String(), "no.such/pkg.Function") || !strings.Contains(out.String(), "not needed") {
		t.Errorf("report does not name the unused entry as not needed:\n%s", out.String())
	}
}

func TestCheck_TwoEntriesMatchingSameGoroutine(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	entries := []Ignore{
		{Function: "net/http.(*Server).Serve", Anywhere: true, Reason: "first"},
		{Function: "net/http.(*Server).Serve", Anywhere: true, Reason: "second"},
	}
	var out bytes.Buffer
	code := check(nil, runnerReturning(0), &out, fakeModulePath, unfiltered, entries)
	if code == 0 {
		t.Fatalf("check() = 0, want non-zero; output: %s", out.String())
	}
	if strings.Count(out.String(), "not needed") != 2 {
		t.Errorf("report does not name both entries as not needed:\n%s", out.String())
	}
}

// TestCheck_RunningHalfAlone excuses, via an Anywhere entry, a function
// that appears somewhere on a goroutine's stack while that same
// goroutine also carries a frame of this module's own code below it, so
// the excuse must still fail: a scan that reads only the "created by"
// line would let it through, because the creator here is the standard
// library's own.
func TestCheck_RunningHalfAlone(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	started := make(chan struct{})
	finished := make(chan struct{})
	time.AfterFunc(time.Millisecond, func() {
		close(started)
		wg.Wait()
		close(finished)
	})
	<-started
	defer func() {
		wg.Done()
		<-finished
	}()

	entries := []Ignore{
		{Function: "sync.(*WaitGroup).Wait", Anywhere: true, Reason: "would excuse it, but this goroutine also carries a module frame"},
	}
	var out bytes.Buffer
	code := check(nil, runnerReturning(0), &out, modulePath, unfiltered, entries)
	if code == 0 {
		t.Fatalf("check() = 0, want non-zero — the excused goroutine also runs this module's own code; output: %s", out.String())
	}
	if !strings.Contains(out.String(), "TestCheck_RunningHalfAlone") {
		t.Errorf("report does not carry the module frame:\n%s", out.String())
	}
}

// startServeDirect starts srv.Serve(l) itself as the new goroutine's
// entry function — no closure wraps the call — so every frame the
// goroutine's own stack carries is standard library, while the
// "created by" line naming startServeDirect is unambiguously this
// module's own code.
func startServeDirect(srv *http.Server, l net.Listener) {
	go srv.Serve(l) //nolint:errcheck // Serve's return is exercised by TestCheck_StartedByHalfAlone's goleak assertions, not by its value; a closure to discard it would add a module frame the case must not carry.
}

// TestCheck_StartedByHalfAlone excuses a goroutine whose every frame is
// standard library, but which this module's own code started, so the
// excuse must still fail: a scan that reads only stack frames would let
// it through, because the creator line is the only place this module's
// code appears.
func TestCheck_StartedByHalfAlone(t *testing.T) {
	before := goleak.IgnoreCurrent()
	var lc net.ListenConfig
	l, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	srv := &http.Server{Handler: http.NewServeMux()}
	startServeDirect(srv, l)
	defer func() {
		_ = srv.Close()
		if err := goleak.Find(before); err != nil {
			t.Errorf("startServeDirect's goroutine did not exit: %v", err)
		}
	}()

	entries := []Ignore{
		{Function: "net/http.(*Server).Serve", Anywhere: true, Reason: "would excuse it, but this module's code started it"},
	}
	var out bytes.Buffer
	code := check(nil, runnerReturning(0), &out, modulePath, unfiltered, entries)
	if code == 0 {
		t.Fatalf("check() = 0, want non-zero — this module's own code started the excused goroutine; output: %s", out.String())
	}
	if !strings.Contains(out.String(), "startServeDirect") {
		t.Errorf("report does not carry the module creator line:\n%s", out.String())
	}
}

// TestCheck_Refusals drives every refusal check has and asserts that the
// output names the refusal it expects, not merely that some output was
// written.
func TestCheck_Refusals(t *testing.T) {
	type tc struct {
		name       string
		modulePath func() (string, bool)
		ignore     []Ignore
		nilRunner  bool
		want       string
	}
	cases := []tc{
		{
			name:       "blank reason",
			modulePath: fakeModulePath,
			ignore:     []Ignore{{Function: "some.Func", Reason: ""}},
			want:       `ignore entry for "some.Func" has a blank Reason`,
		},
		{
			name:       "blank function",
			modulePath: fakeModulePath,
			ignore:     []Ignore{{Function: "", Reason: "why"}},
			want:       "an ignore entry has a blank Function",
		},
		{
			name:       "a function under the module path",
			modulePath: fakeModulePath,
			ignore:     []Ignore{{Function: fakeModule + ".Something", Reason: "why"}},
			want:       fmt.Sprintf("ignore entry %q names this module's own code", fakeModule+".Something"),
		},
		{
			name:       "an empty module path",
			modulePath: noModulePath,
			ignore:     nil,
			want:       "this test binary's module path could not be determined",
		},
		{
			name:       "a nil runner",
			modulePath: fakeModulePath,
			ignore:     nil,
			nilRunner:  true,
			want:       "no runner was given",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			called := false
			var runner func(*testing.M) int
			if !c.nilRunner {
				runner = func(*testing.M) int {
					called = true
					return 0
				}
			}
			var out bytes.Buffer
			code := check(nil, runner, &out, c.modulePath, unfiltered, c.ignore)
			if code == 0 {
				t.Fatal("check() = 0, want non-zero")
			}
			if called {
				t.Error("the runner was called, want the refusal to stop before it")
			}
			if !strings.Contains(out.String(), c.want) {
				t.Errorf("output = %q, want it to contain %q", out.String(), c.want)
			}
		})
	}
}

func TestCheck_StaleEntrySkippedOnlyWhenFiltered(t *testing.T) {
	entries := []Ignore{{Function: "no.such/pkg.Function", Reason: "not on any stack"}}

	var filteredOut bytes.Buffer
	if code := check(nil, runnerReturning(0), &filteredOut, fakeModulePath, isFiltered, entries); code != 0 {
		t.Errorf("filtered run: check() = %d, want 0; output: %s", code, filteredOut.String())
	}
	if !strings.Contains(filteredOut.String(), "filtered run") {
		t.Errorf("filtered run: no skip line in the output: %s", filteredOut.String())
	}

	var unfilteredOut bytes.Buffer
	if code := check(nil, runnerReturning(0), &unfilteredOut, fakeModulePath, unfiltered, entries); code == 0 {
		t.Fatal("unfiltered run: check() = 0, want non-zero — a stale entry must fail an unfiltered run")
	}
}

// fakeLookup builds a flag.Flag lookup exactly like flag.Lookup, over a
// private flag.FlagSet carrying the given values for testing's own
// filter-relevant flags.
func fakeLookup(run, skip, list string, short bool) func(string) *flag.Flag {
	fs := flag.NewFlagSet("fake", flag.ContinueOnError)
	fs.String("test.run", run, "")
	fs.String("test.skip", skip, "")
	fs.String("test.list", list, "")
	fs.Bool("test.short", short, "")
	return fs.Lookup
}

func TestFiltered(t *testing.T) {
	cases := []struct {
		name                           string
		run, skip, list                string
		short, noFlagsRegistered, want bool
	}{
		{name: "nothing set", want: false},
		{name: "test.run set", run: "TestFoo", want: true},
		{name: "test.skip set", skip: "TestFoo", want: true},
		{name: "test.list set", list: "Test.*", want: true},
		{name: "test.short true", short: true, want: true},
		{name: "a lookup that finds no flag at all", noFlagsRegistered: true, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lookup := fakeLookup(tc.run, tc.skip, tc.list, tc.short)
			if tc.noFlagsRegistered {
				lookup = flag.NewFlagSet("empty", flag.ContinueOnError).Lookup
			}
			if got := filtered(lookup); got != tc.want {
				t.Errorf("filtered() = %v, want %v", got, tc.want)
			}
		})
	}
}

// moduleLine returns the module path go.mod's own "module" line declares.
func moduleLine(t *testing.T, goMod string) string {
	t.Helper()
	for _, line := range strings.Split(goMod, "\n") {
		if after, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(after)
		}
	}
	t.Fatal("go.mod: no module line found")
	return ""
}

// TestModulePath_MatchesGoMod drives the real module-path source: its
// answer must agree with go.mod's own "module" line.
func TestModulePath_MatchesGoMod(t *testing.T) {
	got, ok := modulePath()
	if !ok {
		t.Fatal("modulePath: ok = false")
	}
	data, err := os.ReadFile(repotest.RootPath(t, "go.mod"))
	if err != nil {
		t.Fatalf("ReadFile go.mod: %v", err)
	}
	if want := moduleLine(t, string(data)); got != want {
		t.Errorf("modulePath() = %q, want %q (go.mod's module line)", got, want)
	}
}

// TestMain_Wiring drives Main itself — not the check behind it — with a
// nil *testing.M and a stale ignore entry, redirecting os.Stderr to a
// temporary file for the call. It computes its own expectation from the
// real testing flags rather than through the filter predicate under
// test, so a predicate hard-wired to always answer "filtered" or
// "unfiltered" would fail this case on the gate it disagrees with.
func TestMain_Wiring(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "leaktest-wiring-*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer func() { _ = f.Close() }()
	old := os.Stderr
	os.Stderr = f

	entries := []Ignore{{Function: "no.such/pkg.Function", Reason: "not on any stack"}}
	code := Main(nil, runnerReturning(0), entries...)

	os.Stderr = old
	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	wantFiltered := testing.Short()
	for _, name := range []string{"test.run", "test.skip", "test.list"} {
		if f := flag.Lookup(name); f != nil && f.Value.String() != "" {
			wantFiltered = true
		}
	}

	if wantFiltered {
		if code != 0 {
			t.Errorf("Main() = %d, want 0 on a filtered run; output: %s", code, data)
		}
		return
	}
	if code == 0 {
		t.Fatalf("Main() = 0, want non-zero on an unfiltered run — the stale entry must fail it; output: %s", data)
	}
	if !strings.Contains(string(data), "not needed") {
		t.Errorf("output does not name the entry as not needed: %s", data)
	}
}
