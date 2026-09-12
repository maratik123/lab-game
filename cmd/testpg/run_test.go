package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/maratik123/lab-game/internal/testdb"
)

// stubSeam records whether provision was called and lets each test control
// probe/locate/persist/forget/workDir without touching a runtime or the
// process's real working directory. provision starts no container: it
// returns a synthetic DSN and a stop closure that only records that it ran.
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

	forgetCalled bool

	// workDirDir stands in for the process's working directory; a valid
	// stub project directory by default, so tests that do not care about
	// the derivation still reach a valid container name. workDirErr, when
	// set, makes the lookup itself fail instead.
	workDirDir string
	workDirErr error
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
		forget: func() error {
			s.forgetCalled = true
			return nil
		},
		workDir: func() (string, error) {
			if s.workDirErr != nil {
				return "", s.workDirErr
			}
			dir := s.workDirDir
			if dir == "" {
				dir = "/stub/lab-game"
			}
			return dir, nil
		},
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
	// The granted ceiling is echoed on the path that grants it: a run's own
	// log is the only place the arithmetic it relied on can be read
	// afterwards, since the terms come from the host's core count.
	want, err := testdb.Ceiling(1, 1)
	if err != nil {
		t.Fatalf("Ceiling: %v", err)
	}
	if !strings.Contains(stderr.String(), "ceiling "+strconv.Itoa(want)) {
		t.Errorf("stderr = %q, want the granted ceiling %d echoed", stderr.String(), want)
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

func TestRunChild_provisionFails_reportsAndRunsNoChild(t *testing.T) {
	t.Parallel()

	stub := &stubSeam{provisionErr: errors.New("no container runtime")}
	var stdout, stderr bytes.Buffer

	code := runChild(t.Context(), exitChild(0), noLookup, stub.seam(), 1, 1, &stdout, &stderr)

	if code == 0 {
		t.Fatalf("runChild = 0 when provisioning failed; want non-zero")
	}
	// One failure that names its cause beats every database-backed package
	// timing out on its own wait strategy and describing the host instead.
	if !strings.Contains(stderr.String(), "no container runtime") {
		t.Errorf("stderr = %q, want the provisioner's own cause named", stderr.String())
	}
	if stub.stopCalled {
		t.Errorf("stop was called although nothing was provisioned")
	}
}

func TestRunChild_stopFails_reportsButKeepsTheChildsStatus(t *testing.T) {
	t.Parallel()

	stub := &stubSeam{stopErr: errors.New("container already gone")}
	var stdout, stderr bytes.Buffer

	code := runChild(t.Context(), exitChild(3), noLookup, stub.seam(), 1, 1, &stdout, &stderr)

	// Teardown trouble is reported, never substituted for the gate's verdict.
	if code != 3 {
		t.Fatalf("runChild = %d, want the child's own exit code 3 despite the teardown error", code)
	}
	if !strings.Contains(stderr.String(), "container already gone") {
		t.Errorf("stderr = %q, want the teardown error reported", stderr.String())
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

// Not parallel, and neither is the undersized-reuse case below: both reach the
// code that disables the reaper, which is a process-wide setting written to the
// environment. Two such tests running at once would race each other's writes.
func TestRun_upDown_useTheSeamWithNoRuntime(t *testing.T) {
	const dir = "/stub/lab-game"
	wantName, err := containerNameForDir(dir)
	if err != nil {
		t.Fatalf("containerNameForDir(%q): %v", dir, err)
	}

	// probeMaxConns stands for a server whose capacity admits the need: --up
	// reads the capacity back rather than reporting the one it asked for.
	stub := &stubSeam{provisionDSN: "postgres://shared/db", probeMaxConns: 100000, workDirDir: dir}
	var stdout, stderr bytes.Buffer
	code := run([]string{"--up"}, noLookup, stub.seam(), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(--up) = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "capacity 100000") {
		t.Errorf("--up stderr = %q, want the capacity read back from the server", stderr.String())
	}
	if !stub.provisionCalled {
		t.Errorf("--up did not call provision")
	}
	if stub.provisionOpts.ContainerName != wantName {
		t.Errorf("--up provisioned with ContainerName=%q, want %q", stub.provisionOpts.ContainerName, wantName)
	}
	if !strings.Contains(stderr.String(), wantName) {
		t.Errorf("--up stderr = %q, want the report line to name the container %q", stderr.String(), wantName)
	}

	stub2 := &stubSeam{locateDSN: "postgres://shared/db", locateOK: true, probeMaxConns: 100, workDirDir: dir}
	var stdout2, stderr2 bytes.Buffer
	code = run([]string{"--down"}, noLookup, stub2.seam(), &stdout2, &stderr2)
	if code != 0 {
		t.Fatalf("run(--down) = %d, want 0; stderr: %s", code, stderr2.String())
	}
	if !stub2.provisionCalled || !stub2.stopCalled {
		t.Errorf("--down did not stop the located server (provisionCalled=%v, stopCalled=%v)", stub2.provisionCalled, stub2.stopCalled)
	}
	if stub2.provisionOpts.ContainerName != wantName {
		t.Errorf("--down provisioned with ContainerName=%q, want %q", stub2.provisionOpts.ContainerName, wantName)
	}
}

func TestRun_upOnAnUndersizedExistingServer_failsNamingTheCapacity(t *testing.T) {
	// A container already running under the shared name keeps the capacity it
	// was created with, so provisioning "succeeds" while granting less than
	// this invocation asked for. Reporting the requested number here would
	// have the caller believe a larger client count was granted.
	stub := &stubSeam{provisionDSN: "postgres://shared/db", probeMaxConns: 8}
	var stdout, stderr bytes.Buffer
	code := run([]string{"--up", "--clients", "2"}, noLookup, stub.seam(), &stdout, &stderr)
	if code == 0 {
		t.Fatalf("run(--up --clients 2) = 0, want non-zero; stderr: %s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "capacity is 8") {
		t.Errorf("stderr = %q, want the server's own capacity named", stderr.String())
	}
}

func TestRun_downWithNoLocator_isANoOp(t *testing.T) {
	t.Parallel()

	// The working directory carries an invalid name on purpose: the no-op
	// exit is decided by sm.locate() alone, so a derivation that ran before
	// it would turn this case into a failure it must not become.
	stub := &stubSeam{workDirDir: "/stub/-invalid"}
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

func TestContainerNameForDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dir     string
		want    string
		wantErr bool
	}{
		{name: "first checkout", dir: "/home/dev/lab-game", want: "lab-game-test-postgres"},
		{name: "sibling checkout", dir: "/home/dev/lab-game2", want: "lab-game2-test-postgres"},
		{name: "underscore and dot survive", dir: "/home/dev/lab_game.2", want: "lab_game.2-test-postgres"},
		{name: "space is refused", dir: "/home/dev/lab game", wantErr: true},
		{name: "leading dash is refused", dir: "/home/dev/-lab-game", wantErr: true},
		{name: "leading dot is refused", dir: "/home/dev/.lab-game", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := containerNameForDir(tc.dir)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("containerNameForDir(%q) = %q, nil; want an error", tc.dir, got)
				}
				if !strings.Contains(err.Error(), filepath.Base(tc.dir)) {
					t.Errorf("error %q does not name the offending directory %q", err, filepath.Base(tc.dir))
				}
				return
			}
			if err != nil {
				t.Fatalf("containerNameForDir(%q) = _, %v; want no error", tc.dir, err)
			}
			if got != tc.want {
				t.Errorf("containerNameForDir(%q) = %q, want %q", tc.dir, got, tc.want)
			}
		})
	}
}

func TestContainerNameForDir_rootPath_hasNoBaseName(t *testing.T) {
	t.Parallel()

	if got, err := containerNameForDir("/"); err == nil {
		t.Fatalf("containerNameForDir(\"/\") = %q, nil; want an error", got)
	}
}

func TestContainerNameForDir_sameBaseName_differentParents_isEqual(t *testing.T) {
	t.Parallel()

	a, errA := containerNameForDir("/home/alpha/lab-game")
	b, errB := containerNameForDir("/var/beta/lab-game")
	if errA != nil || errB != nil {
		t.Fatalf("containerNameForDir errors: %v, %v", errA, errB)
	}
	if a != b {
		t.Errorf("containerNameForDir(.../alpha/lab-game) = %q, containerNameForDir(.../beta/lab-game) = %q; want equal", a, b)
	}
}

// Not parallel: reaches the code that disables the reaper, a process-wide
// environment write.
func TestRun_up_differentDirs_differentContainerNames(t *testing.T) {
	stubA := &stubSeam{workDirDir: "/stub/lab-game", probeMaxConns: 100000}
	var stdoutA, stderrA bytes.Buffer
	if code := run([]string{"--up"}, noLookup, stubA.seam(), &stdoutA, &stderrA); code != 0 {
		t.Fatalf("run(--up) [a] = %d, want 0; stderr: %s", code, stderrA.String())
	}

	stubB := &stubSeam{workDirDir: "/stub/lab-game2", probeMaxConns: 100000}
	var stdoutB, stderrB bytes.Buffer
	if code := run([]string{"--up"}, noLookup, stubB.seam(), &stdoutB, &stderrB); code != 0 {
		t.Fatalf("run(--up) [b] = %d, want 0; stderr: %s", code, stderrB.String())
	}

	if stubA.provisionOpts.ContainerName == stubB.provisionOpts.ContainerName {
		t.Errorf("both directories provisioned the same container name %q", stubA.provisionOpts.ContainerName)
	}
}

// Not parallel, same reason as above.
func TestRun_up_sameBaseName_differentParents_sameContainerName(t *testing.T) {
	stubA := &stubSeam{workDirDir: "/home/alpha/lab-game", probeMaxConns: 100000}
	var stdoutA, stderrA bytes.Buffer
	if code := run([]string{"--up"}, noLookup, stubA.seam(), &stdoutA, &stderrA); code != 0 {
		t.Fatalf("run(--up) [alpha] = %d, want 0; stderr: %s", code, stderrA.String())
	}

	stubB := &stubSeam{workDirDir: "/var/beta/lab-game", probeMaxConns: 100000}
	var stdoutB, stderrB bytes.Buffer
	if code := run([]string{"--up"}, noLookup, stubB.seam(), &stdoutB, &stderrB); code != 0 {
		t.Fatalf("run(--up) [beta] = %d, want 0; stderr: %s", code, stderrB.String())
	}

	if stubA.provisionOpts.ContainerName != stubB.provisionOpts.ContainerName {
		t.Errorf("provisioned names differ: %q vs %q, want equal", stubA.provisionOpts.ContainerName, stubB.provisionOpts.ContainerName)
	}
}

// Not parallel, same reason as above.
func TestRun_down_staleLocator_invalidDir_stillExitsZero(t *testing.T) {
	stub := &stubSeam{
		locateDSN:  "postgres://stale/db",
		locateOK:   true,
		probeErr:   errUnreachable,
		workDirDir: "/stub/-invalid",
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"--down"}, noLookup, stub.seam(), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(--down) = %d, want 0; stderr: %s", code, stderr.String())
	}
	if !stub.forgetCalled {
		t.Errorf("forget was not called for a stale locator")
	}
	if stub.provisionCalled {
		t.Errorf("provision was called although the locator was stale")
	}
}

// Not parallel, same reason as above.
func TestRun_up_invalidDir_failsNamingTheDirectory(t *testing.T) {
	const dir = "/stub/-invalid"
	stub := &stubSeam{workDirDir: dir}
	var stdout, stderr bytes.Buffer
	code := run([]string{"--up"}, noLookup, stub.seam(), &stdout, &stderr)
	if code == 0 {
		t.Fatalf("run(--up) = 0, want non-zero for an invalid directory name")
	}
	if stub.provisionCalled {
		t.Errorf("provision was called although the directory name is invalid")
	}
	if !strings.Contains(stderr.String(), filepath.Base(dir)) {
		t.Errorf("stderr = %q, want the offending directory name %q", stderr.String(), filepath.Base(dir))
	}
}

// Not parallel: asserts on the process-wide reaper environment variable.
func TestRun_up_invalidDir_leavesReaperSettingUnchanged(t *testing.T) {
	orig, hadOrig := os.LookupEnv("TESTCONTAINERS_RYUK_DISABLED")
	t.Cleanup(func() {
		if hadOrig {
			_ = os.Setenv("TESTCONTAINERS_RYUK_DISABLED", orig)
		} else {
			_ = os.Unsetenv("TESTCONTAINERS_RYUK_DISABLED")
		}
	})
	if err := os.Unsetenv("TESTCONTAINERS_RYUK_DISABLED"); err != nil {
		t.Fatalf("Unsetenv: %v", err)
	}

	stub := &stubSeam{workDirDir: "/stub/-invalid"}
	var stdout, stderr bytes.Buffer
	run([]string{"--up"}, noLookup, stub.seam(), &stdout, &stderr)

	if _, ok := os.LookupEnv("TESTCONTAINERS_RYUK_DISABLED"); ok {
		t.Errorf("TESTCONTAINERS_RYUK_DISABLED was set although the pre-check should fail before it")
	}
}

// Not parallel, same reason as above.
func TestRun_down_invalidDir_stopsNothing(t *testing.T) {
	stub := &stubSeam{
		locateDSN:     "postgres://shared/db",
		locateOK:      true,
		probeMaxConns: 100,
		workDirDir:    "/stub/-invalid",
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"--down"}, noLookup, stub.seam(), &stdout, &stderr)
	if code == 0 {
		t.Fatalf("run(--down) = 0, want non-zero for an invalid directory name")
	}
	if stub.stopCalled {
		t.Errorf("stop was called although the directory name is invalid")
	}
}

// Not parallel: asserts on the process-wide reaper environment variable.
func TestRun_down_invalidDir_leavesReaperSettingUnchanged(t *testing.T) {
	orig, hadOrig := os.LookupEnv("TESTCONTAINERS_RYUK_DISABLED")
	t.Cleanup(func() {
		if hadOrig {
			_ = os.Setenv("TESTCONTAINERS_RYUK_DISABLED", orig)
		} else {
			_ = os.Unsetenv("TESTCONTAINERS_RYUK_DISABLED")
		}
	})
	if err := os.Unsetenv("TESTCONTAINERS_RYUK_DISABLED"); err != nil {
		t.Fatalf("Unsetenv: %v", err)
	}

	stub := &stubSeam{
		locateDSN:     "postgres://shared/db",
		locateOK:      true,
		probeMaxConns: 100,
		workDirDir:    "/stub/-invalid",
	}
	var stdout, stderr bytes.Buffer
	run([]string{"--down"}, noLookup, stub.seam(), &stdout, &stderr)

	if _, ok := os.LookupEnv("TESTCONTAINERS_RYUK_DISABLED"); ok {
		t.Errorf("TESTCONTAINERS_RYUK_DISABLED was set although the pre-check should fail before it")
	}
}

// Not parallel, same reason as above.
func TestRun_up_workDirLookupFails(t *testing.T) {
	stub := &stubSeam{workDirErr: errors.New("no such directory")}
	var stdout, stderr bytes.Buffer
	code := run([]string{"--up"}, noLookup, stub.seam(), &stdout, &stderr)
	if code == 0 {
		t.Fatalf("run(--up) = 0, want non-zero when the working directory cannot be read")
	}
	if stub.provisionCalled {
		t.Errorf("provision was called although the working directory lookup failed")
	}
	if !strings.Contains(stderr.String(), "no such directory") {
		t.Errorf("stderr = %q, want the lookup's own error", stderr.String())
	}
}

// Not parallel, same reason as above.
func TestRun_down_workDirLookupFails(t *testing.T) {
	stub := &stubSeam{
		locateDSN:     "postgres://shared/db",
		locateOK:      true,
		probeMaxConns: 100,
		workDirErr:    errors.New("no such directory"),
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"--down"}, noLookup, stub.seam(), &stdout, &stderr)
	if code == 0 {
		t.Fatalf("run(--down) = 0, want non-zero when the working directory cannot be read")
	}
	if stub.stopCalled {
		t.Errorf("stop was called although the working directory lookup failed")
	}
}

func TestRunChild_invalidWorkDir_stillRunsToCompletion(t *testing.T) {
	t.Parallel()

	// runChild must not derive a container name at all: the gate path must
	// not be able to fail on a fact it never uses.
	stub := &stubSeam{workDirDir: "/stub/-invalid"}
	var stdout, stderr bytes.Buffer

	code := runChild(t.Context(), exitChild(3), noLookup, stub.seam(), 1, 1, &stdout, &stderr)

	if code != 3 {
		t.Fatalf("runChild = %d, want the child's own exit code 3; stderr: %s", code, stderr.String())
	}
}

// errUnreachable is a stand-in probe error; its text is deliberately not
// the connection-exhaustion literal, so tests here can never be confused
// with the contention probe's own scan.
var errUnreachable = errStub("dial tcp: connection refused")

type errStub string

func (e errStub) Error() string { return string(e) }
