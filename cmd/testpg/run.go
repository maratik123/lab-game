package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"syscall"

	"github.com/maratik123/lab-game/internal/testdb"
)

// Exit codes: 0 = the child (or --up/--down) succeeded; exitUsage = the
// wrapper's own arguments were wrong; exitFailure = the wrapper could not
// provision or reach a server, or the child could not even be started. A
// child that ran and exited non-zero propagates ITS OWN exit code, read
// from the process's exit status rather than assumed.
const (
	exitUsage   = 2
	exitFailure = 1
)

// envLookup mirrors os.LookupEnv's signature, so run's caller-visibility
// scenario is testable without touching the process environment.
type envLookup func(key string) (string, bool)

// logf writes a report or diagnostic line, discarding the write error: a
// broken stdout/stderr pipe leaves nothing more this command can do about
// it, and the exit code already reflects what happened.
func logf(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

// seam bundles what run needs from testdb — starting and stopping a server,
// probing one's capacity, reading/writing/removing the locator file, and
// reading the process's working directory — behind values run depends on
// rather than functions it calls directly, so tests can substitute a stub
// that starts no container, dials nothing and names no real directory.
type seam struct {
	provision func(ctx context.Context, opts testdb.ServerOptions) (dsn string, stop func(context.Context) error, err error)
	probe     func(ctx context.Context, dsn string) (maxConns int, err error)
	locate    func() (dsn string, ok bool)
	persist   func(dsn string) error
	forget    func() error
	workDir   func() (dir string, err error)
}

// productionSeam wraps testdb's real provisioning, the on-disk locator file
// under the repository's ignored scratch directory, and the process's
// actual working directory.
func productionSeam() seam {
	return seam{
		provision: func(ctx context.Context, opts testdb.ServerOptions) (string, func(context.Context) error, error) {
			server, err := testdb.StartServer(ctx, opts)
			if err != nil {
				return "", nil, err
			}
			return server.DSN(), server.Stop, nil
		},
		probe:   testdb.Probe,
		locate:  readLocator,
		persist: writeLocator,
		forget:  removeLocator,
		workDir: os.Getwd,
	}
}

// run implements the wrapper's whole behaviour over a testable signature:
// the argument vector, an environment lookup and the provisioning seam.
// Wrapper flags precede a "--" separator; everything after it is the child
// command's own argument vector.
func run(argv []string, lookup envLookup, sm seam, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("testpg", flag.ContinueOnError)
	fs.SetOutput(stderr)
	up := fs.Bool("up", false, "create the long-lived shared server and leave it running")
	down := fs.Bool("down", false, "remove the long-lived shared server this project's own target started")
	clients := fs.Int("clients", 1, "concurrent whole-module test runs the provisioned server must admit at once")
	parallel := fs.Int("parallel", runtime.GOMAXPROCS(0), "tests running simultaneously inside one binary; pass the child's own -parallel/-p when a target pins it")

	wrapperArgs, childArgv := splitAtSeparator(argv)
	if err := fs.Parse(wrapperArgs); err != nil {
		return exitUsage
	}

	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	switch {
	case *up && *down:
		logf(stderr, "testpg: --up and --down are mutually exclusive\n")
		return exitUsage
	case *up:
		return runUp(ctx, *clients, *parallel, sm, stdout, stderr)
	case *down:
		return runDown(ctx, sm, stdout, stderr)
	case len(childArgv) == 0:
		logf(stderr, "testpg: usage: testpg [--clients N] [--parallel N] -- <command> [args...]\n")
		return exitUsage
	default:
		return runChild(ctx, childArgv, lookup, sm, *clients, *parallel, stdout, stderr)
	}
}

// testServerSuffix is appended to a project directory's base name to derive
// the long-lived test-server container name, so each checkout on a host
// addresses its own server.
const testServerSuffix = "-test-postgres"

// containerNamePattern is this project's own copy of the container
// runtime's naming rule: a name must start with a letter or digit and
// continue with letters, digits, underscores, dots or dashes. It exists to
// fail fast, naming the offending directory, rather than let a doomed name
// reach the runtime and come back as an unattributed refusal.
var containerNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// containerNameForDir derives the long-lived test-server container name
// from a project directory: its base name plus the fixed test-server
// suffix, so two checkouts sharing a base name address one server and two
// checkouts with distinct base names address distinct ones. It returns an
// error naming dir's base name and the rule when that base name is empty or
// contains a character the container runtime refuses.
func containerNameForDir(dir string) (string, error) {
	base := filepath.Base(dir)
	if !containerNamePattern.MatchString(base) {
		return "", fmt.Errorf("project directory %q must start with a letter or digit and contain only letters, digits, underscores, dots or dashes to name a test-server container", base)
	}
	return base + testServerSuffix, nil
}

// splitAtSeparator returns argv split at its first bare "--" element: the
// wrapper's own flags before it, the child's argument vector after. With no
// "--", every element is a wrapper flag and the child vector is empty.
func splitAtSeparator(argv []string) (wrapperArgs, childArgv []string) {
	for i, a := range argv {
		if a == "--" {
			return argv[:i], argv[i+1:]
		}
	}
	return argv, nil
}

// runChild implements the wrapper's decision order: a caller-supplied server is used
// unconditionally with a reported (not fatal) capacity shortfall; a
// discovered long-lived server is used only when it admits the computed
// need, falling through with a message otherwise; the fallback is an
// anonymous container sized by the ceiling and torn down after the child
// exits, on every exit path.
func runChild(ctx context.Context, childArgv []string, lookup envLookup, sm seam, clients, parallel int, stdout, stderr io.Writer) int {
	if dsn, ok := lookup(testdb.DSNEnv); ok && dsn != "" {
		reportShortfall(ctx, sm, dsn, clients, parallel, "the caller-supplied server", stderr)
		return execChild(ctx, childArgv, dsn, stdout, stderr)
	}

	if dsn, ok := sm.locate(); ok {
		if admits(ctx, sm, dsn, clients, parallel, "the long-lived server", stderr) {
			// Found, not started: no stop is registered for this server.
			return execChild(ctx, childArgv, dsn, stdout, stderr)
		}
	}

	ceiling, err := testdb.Ceiling(clients, parallel)
	if err != nil {
		logf(stderr, "testpg: %v\n", err)
		return exitFailure
	}

	dsn, stop, err := sm.provision(ctx, testdb.ServerOptions{ConnCeiling: ceiling})
	if err != nil {
		logf(stderr, "testpg: could not start a server: %v\n", err)
		return exitFailure
	}
	// Echoed on the path that grants it, so the arithmetic a run relied on
	// is readable afterwards from that run's own log rather than recomputed
	// from a core count nobody recorded.
	logf(stderr, "testpg: started a server, ceiling %d (clients=%d, parallel=%d)\n", ceiling, clients, parallel)
	//nolint:contextcheck // fresh context by design: teardown must survive a cancelled signal context, not be cancelled itself the instant it starts
	defer func() {
		if err := stop(context.Background()); err != nil {
			logf(stderr, "testpg: stopping the container: %v\n", err)
		}
	}()

	return execChild(ctx, childArgv, dsn, stdout, stderr)
}

// admits reports whether dsn's server answers and its max_connections
// admits the computed need for clients/parallel, printing a message
// naming label when it does not (unreachable or undersized alike).
func admits(ctx context.Context, sm seam, dsn string, clients, parallel int, label string, stderr io.Writer) bool {
	needed, needErr := testdb.Ceiling(clients, parallel)
	maxConns, probeErr := sm.probe(ctx, dsn)
	switch {
	case probeErr != nil:
		logf(stderr, "testpg: %s is unreachable (%v); falling through\n", label, probeErr)
		return false
	case needErr != nil:
		logf(stderr, "testpg: %s: this run's own need cannot be computed (%v); falling through\n", label, needErr)
		return false
	case maxConns < needed:
		logf(stderr, "testpg: %s admits %d connections, this run needs %d; falling through\n", label, maxConns, needed)
		return false
	default:
		return true
	}
}

// reportShortfall prints, but never blocks on, a capacity shortfall found
// on a server the wrapper did not choose to use — the caller-supplied path,
// where the contract is that the gates execute against the caller's named
// server regardless.
func reportShortfall(ctx context.Context, sm seam, dsn string, clients, parallel int, label string, stderr io.Writer) {
	needed, needErr := testdb.Ceiling(clients, parallel)
	if needErr != nil {
		logf(stderr, "testpg: %s: this run's own need cannot be computed: %v\n", label, needErr)
		return
	}
	maxConns, probeErr := sm.probe(ctx, dsn)
	if probeErr != nil {
		logf(stderr, "testpg: could not read %s's capacity: %v\n", label, probeErr)
		return
	}
	if maxConns < needed {
		logf(stderr, "testpg: %s admits %d connections, this run needs %d; running against it anyway\n", label, maxConns, needed)
	}
}

// execChild runs childArgv under ctx with dsn exported through testdb's DSN
// environment variable, streams stdin/stdout/stderr through, and returns
// the child's own exit code — never a code assumed from zero-vs-non-zero,
// so this function's caller sees the real number even though a go-run
// invocation of this command later flattens it.
func execChild(ctx context.Context, childArgv []string, dsn string, stdout, stderr io.Writer) int {
	cmd := exec.CommandContext(ctx, childArgv[0], childArgv[1:]...) //nolint:gosec // childArgv is this wrapper's own argument vector, taken verbatim from its caller after the -- separator, exactly as any other gate command's argv is
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = os.Stdin
	cmd.Env = append(os.Environ(), testdb.DSNEnv+"="+dsn)

	err := cmd.Run()
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	logf(stderr, "testpg: running the child command: %v\n", err)
	return exitFailure
}

// runUp sizes and creates the long-lived named server, reusing one already
// running under that name, disables the reaper for the rest of this
// process so the container survives it, and writes the locator file.
func runUp(ctx context.Context, clients, parallel int, sm seam, stdout, stderr io.Writer) int {
	dir, err := sm.workDir()
	if err != nil {
		logf(stderr, "testpg: reading the working directory: %v\n", err)
		return exitFailure
	}
	containerName, err := containerNameForDir(dir)
	if err != nil {
		logf(stderr, "testpg: %v\n", err)
		return exitFailure
	}

	ceiling, err := testdb.Ceiling(clients, parallel)
	if err != nil {
		logf(stderr, "testpg: %v\n", err)
		return exitFailure
	}

	// The reaper is controlled process-wide, read once at first use; this
	// process must disable it before provisioning so the container it
	// creates carries no reap label and outlives this process.
	if err := os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true"); err != nil {
		logf(stderr, "testpg: disabling the reaper: %v\n", err)
		return exitFailure
	}

	dsn, _, err := sm.provision(ctx, testdb.ServerOptions{
		ContainerName: containerName,
		ConnCeiling:   ceiling,
	})
	if err != nil {
		logf(stderr, "testpg: could not start the shared server: %v\n", err)
		return exitFailure
	}

	if err := sm.persist(dsn); err != nil {
		logf(stderr, "testpg: writing the locator file: %v\n", err)
		return exitFailure
	}

	logf(stdout, "%s\n", dsn)

	// Report the capacity the server HAS, not the one this invocation asked
	// for: an existing container is reused under its name and keeps the
	// ceiling it was created with, so a larger client count asked for here
	// would otherwise be reported as granted while the server stayed the
	// size it was. Reading it back is one round trip on a connection the
	// probe opens anyway.
	actual, err := sm.probe(ctx, dsn)
	switch {
	case err != nil:
		logf(stderr, "testpg: shared server up, but its capacity could not be read: %v\n", err)
	case actual < ceiling:
		logf(stderr, "testpg: the shared server's capacity is %d, below the %d needed for clients=%d "+
			"parallel=%d; a server already running under this name keeps the capacity it was created "+
			"with, so take it down and bring it up again to resize\n", actual, ceiling, clients, parallel)
		return exitFailure
	default:
		logf(stderr, "testpg: shared server %q up, capacity %d admits the %d needed (clients=%d, parallel=%d)\n",
			containerName, actual, ceiling, clients, parallel)
	}
	return 0
}

// runDown removes the long-lived server this project's own target started,
// never one it merely found: with no locator file there is nothing to
// remove, and an unreachable locator is treated as stale and forgotten
// rather than turned into a fresh container only to delete it again.
func runDown(ctx context.Context, sm seam, stdout, stderr io.Writer) int {
	dsn, ok := sm.locate()
	if !ok {
		logf(stdout, "testpg: no shared server locator found; nothing to remove\n")
		return 0
	}

	if _, err := sm.probe(ctx, dsn); err != nil {
		logf(stderr, "testpg: the shared server is already unreachable (%v); removing the stale locator\n", err)
		if err := sm.forget(); err != nil {
			logf(stderr, "testpg: removing the locator file: %v\n", err)
			return exitFailure
		}
		return 0
	}

	dir, err := sm.workDir()
	if err != nil {
		logf(stderr, "testpg: reading the working directory: %v\n", err)
		return exitFailure
	}
	containerName, err := containerNameForDir(dir)
	if err != nil {
		logf(stderr, "testpg: %v\n", err)
		return exitFailure
	}

	// Disabled for the same reason it is when creating the server, and one
	// more: this invocation only reaches an existing container in order to
	// remove it, so a reaper started here would supervise nothing and
	// outlive the thing it was started for.
	if err := os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true"); err != nil {
		logf(stderr, "testpg: disabling the reaper: %v\n", err)
		return exitFailure
	}

	_, stop, err := sm.provision(ctx, testdb.ServerOptions{ContainerName: containerName})
	if err != nil {
		logf(stderr, "testpg: could not reach the shared server to remove it: %v\n", err)
		return exitFailure
	}
	if err := stop(context.Background()); err != nil { //nolint:contextcheck // fresh context by design, matching runChild's teardown: removal must not inherit an already-cancelled signal context
		logf(stderr, "testpg: stopping the shared server: %v\n", err)
		return exitFailure
	}

	if err := sm.forget(); err != nil {
		logf(stderr, "testpg: removing the locator file: %v\n", err)
		return exitFailure
	}

	logf(stdout, "testpg: shared server down\n")
	return 0
}
