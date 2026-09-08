package main

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/maratik123/lab-game/internal/testdb"
)

// stubSeam records whether provision was called and lets each test control
// probe/locate/persist/forget without touching a runtime. provision starts
// no container: it returns a synthetic DSN and a stop closure that only
// records that it ran.
type stubSeam struct {
	provisionCalled bool
	provisionDSN    string
	provisionErr    error
	provisionOpts   testdb.ServerOptions

	stopCalled bool
	stopErr    error

	probeMaxConns int
	probeErr      error

	locateDSN string
	locateOK  bool
}

func (s *stubSeam) seam() seam {
	return seam{
		provision: func(_ context.Context, opts testdb.ServerOptions) (string, func(context.Context) error, error) {
			s.provisionCalled = true
			s.provisionOpts = opts
			if s.provisionErr != nil {
				return "", nil, s.provisionErr
			}
			dsn := s.provisionDSN
			if dsn == "" {
				dsn = "postgres://stub/anon"
			}
			return dsn, func(context.Context) error {
				s.stopCalled = true
				return s.stopErr
			}, nil
		},
		probe: func(_ context.Context, _ string) (int, error) {
			return s.probeMaxConns, s.probeErr
		},
		locate:  func() (string, bool) { return s.locateDSN, s.locateOK },
		persist: func(string) error { return nil },
		forget:  func() error { return nil },
	}
}

// noLookup answers every lookup as unset, the "no caller-supplied DSN" case.
func noLookup(string) (string, bool) { return "", false }

// dsnLookup answers the DSN environment variable's key with dsn and
// everything else as unset.
func dsnLookup(dsn string) envLookup {
	return func(key string) (string, bool) {
		if key == testdb.DSNEnv {
			return dsn, true
		}
		return "", false
	}
}

// readDSNChild is a child command that prints the DSN environment variable
// to stdout, so a test can assert exactly what the child saw.
var readDSNChild = []string{"sh", "-c", `printf '%s' "$` + testdb.DSNEnv + `"`}

func exitChild(code int) []string {
	return []string{"sh", "-c", "exit " + strconv.Itoa(code)}
}

func TestRunChild_callerDSN_seamNeverCalled_childSeesSameDSN(t *testing.T) {
	t.Parallel()
	const dsn = "postgres://caller/db"

	stub := &stubSeam{probeMaxConns: 1000} // ample capacity: no shortfall message needed to prove the point
	var stdout, stderr bytes.Buffer

	code := runChild(t.Context(), readDSNChild, dsnLookup(dsn), stub.seam(), 1, 1, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("runChild = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stub.provisionCalled {
		t.Errorf("provision was called; want the seam untouched for a caller-supplied DSN")
	}
	if got := stdout.String(); got != dsn {
		t.Errorf("child saw DSN %q, want %q", got, dsn)
	}
}

func TestRunChild_callerDSN_undersized_reportsAndRunsAnyway(t *testing.T) {
	t.Parallel()
	const dsn = "postgres://caller/db"

	stub := &stubSeam{probeMaxConns: 1} // far below any computed need
	var stdout, stderr bytes.Buffer

	code := runChild(t.Context(), readDSNChild, dsnLookup(dsn), stub.seam(), 1, 1, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("runChild = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stub.provisionCalled {
		t.Errorf("provision was called; want the seam untouched for a caller-supplied DSN")
	}
	if got := stdout.String(); got != dsn {
		t.Errorf("child saw DSN %q, want %q", got, dsn)
	}
	if !strings.Contains(stderr.String(), "admits") {
		t.Errorf("stderr = %q, want a reported capacity shortfall", stderr.String())
	}
}

func TestRunChild_noDSNNoLocator_seamCalled_stopRunsOnPass(t *testing.T) {
	t.Parallel()

	stub := &stubSeam{}
	var stdout, stderr bytes.Buffer

	code := runChild(t.Context(), exitChild(0), noLookup, stub.seam(), 1, 1, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("runChild = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !stub.provisionCalled {
		t.Errorf("provision was not called; want the anonymous-container fallback")
	}
	if !stub.stopCalled {
		t.Errorf("stop was not called after a passing child")
	}
}

func TestRunChild_noDSNNoLocator_stopRunsOnFailingChild(t *testing.T) {
	t.Parallel()

	stub := &stubSeam{}
	var stdout, stderr bytes.Buffer

	code := runChild(t.Context(), exitChild(3), noLookup, stub.seam(), 1, 1, &stdout, &stderr)

	if code != 3 {
		t.Fatalf("runChild = %d, want the child's own exit code 3", code)
	}
	if !stub.stopCalled {
		t.Errorf("stop was not called after a failing child")
	}
}

func TestRunChild_stopRunsAfterACancelledContext(t *testing.T) {
	t.Parallel()

	stub := &stubSeam{}
	var stdout, stderr bytes.Buffer

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // simulates a delivered interrupt: the context is already done

	code := runChild(ctx, exitChild(0), noLookup, stub.seam(), 1, 1, &stdout, &stderr)

	if code == 0 {
		t.Errorf("runChild = 0 against a cancelled context; want a non-zero status (the child could not run)")
	}
	if !stub.stopCalled {
		t.Errorf("stop was not called after a cancelled context; teardown must run on every exit path")
	}
}

func TestRunChild_locatorUnreachable_ignoredSeamCalled(t *testing.T) {
	t.Parallel()

	stub := &stubSeam{
		locateDSN: "postgres://stale/db",
		locateOK:  true,
		probeErr:  errUnreachable,
	}
	var stdout, stderr bytes.Buffer

	code := runChild(t.Context(), exitChild(0), noLookup, stub.seam(), 1, 1, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("runChild = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !stub.provisionCalled {
		t.Errorf("provision was not called; an unreachable locator must fall through to the anonymous container")
	}
	if !strings.Contains(stderr.String(), "unreachable") {
		t.Errorf("stderr = %q, want a message naming the unreachable locator", stderr.String())
	}
}

func TestRunChild_locatorUndersized_fallsThrough(t *testing.T) {
	t.Parallel()

	stub := &stubSeam{
		locateDSN:     "postgres://small/db",
		locateOK:      true,
		probeMaxConns: 1,
	}
	var stdout, stderr bytes.Buffer

	code := runChild(t.Context(), exitChild(0), noLookup, stub.seam(), 1, 1, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("runChild = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !stub.provisionCalled {
		t.Errorf("provision was not called; an undersized locator must fall through to the anonymous container")
	}
}

func TestRunChild_locatorAdmits_seamNotCalled_noStop(t *testing.T) {
	t.Parallel()
	const dsn = "postgres://longlived/db"

	stub := &stubSeam{
		locateDSN:     dsn,
		locateOK:      true,
		probeMaxConns: 1000,
	}
	var stdout, stderr bytes.Buffer

	code := runChild(t.Context(), readDSNChild, noLookup, stub.seam(), 1, 1, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("runChild = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stub.provisionCalled {
		t.Errorf("provision was called; want the discovered server used as found, not started")
	}
	if stub.stopCalled {
		t.Errorf("stop was called; a found server must not be torn down by this invocation")
	}
	if got := stdout.String(); got != dsn {
		t.Errorf("child saw DSN %q, want %q", got, dsn)
	}
}

func TestRunChild_ceilingMatchesTheFormula(t *testing.T) {
	t.Parallel()
	const clients, parallel = 2, 3

	stub := &stubSeam{}
	var stdout, stderr bytes.Buffer

	code := runChild(t.Context(), exitChild(0), noLookup, stub.seam(), clients, parallel, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runChild = %d, want 0; stderr: %s", code, stderr.String())
	}

	want, err := testdb.Ceiling(clients, parallel)
	if err != nil {
		t.Fatalf("testdb.Ceiling(%d, %d): %v", clients, parallel, err)
	}
	if stub.provisionOpts.ConnCeiling != want {
		t.Errorf("provision was called with ConnCeiling=%d, want %d", stub.provisionOpts.ConnCeiling, want)
	}
}

func TestRun_upDown_useTheSeamWithNoRuntime(t *testing.T) {
	t.Parallel()

	stub := &stubSeam{provisionDSN: "postgres://shared/db"}
	var stdout, stderr bytes.Buffer
	code := run([]string{"--up"}, noLookup, stub.seam(), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(--up) = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !stub.provisionCalled {
		t.Errorf("--up did not call provision")
	}
	if stub.provisionOpts.ContainerName != testdb.SharedContainerName {
		t.Errorf("--up provisioned with ContainerName=%q, want %q", stub.provisionOpts.ContainerName, testdb.SharedContainerName)
	}

	stub2 := &stubSeam{locateDSN: "postgres://shared/db", locateOK: true, probeMaxConns: 100}
	var stdout2, stderr2 bytes.Buffer
	code = run([]string{"--down"}, noLookup, stub2.seam(), &stdout2, &stderr2)
	if code != 0 {
		t.Fatalf("run(--down) = %d, want 0; stderr: %s", code, stderr2.String())
	}
	if !stub2.provisionCalled || !stub2.stopCalled {
		t.Errorf("--down did not stop the located server (provisionCalled=%v, stopCalled=%v)", stub2.provisionCalled, stub2.stopCalled)
	}
}

func TestRun_downWithNoLocator_isANoOp(t *testing.T) {
	t.Parallel()

	stub := &stubSeam{}
	var stdout, stderr bytes.Buffer
	code := run([]string{"--down"}, noLookup, stub.seam(), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(--down) = %d, want 0; stderr: %s", code, stderr.String())
	}
	if stub.provisionCalled {
		t.Errorf("provision was called with no locator; --down must be a no-op")
	}
}

func TestRun_upAndDownTogether_isAUsageError(t *testing.T) {
	t.Parallel()

	stub := &stubSeam{}
	var stdout, stderr bytes.Buffer
	code := run([]string{"--up", "--down"}, noLookup, stub.seam(), &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("run(--up --down) = %d, want %d", code, exitUsage)
	}
	if stub.provisionCalled {
		t.Errorf("provision was called; --up/--down together must be refused before provisioning anything")
	}
}

func TestRun_noChildArgs_isAUsageError(t *testing.T) {
	t.Parallel()

	stub := &stubSeam{}
	var stdout, stderr bytes.Buffer
	code := run(nil, noLookup, stub.seam(), &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("run(nil) = %d, want %d", code, exitUsage)
	}
	if stub.provisionCalled {
		t.Errorf("provision was called with no child command at all")
	}
}

// errUnreachable is a stand-in probe error; its text is deliberately not
// the connection-exhaustion literal, so tests here can never be confused
// with the contention probe's own scan.
var errUnreachable = errStub("dial tcp: connection refused")

type errStub string

func (e errStub) Error() string { return string(e) }
