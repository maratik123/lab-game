package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/maratik123/lab-game/internal/config"
)

// exitUsage is the exit code for an unrecognised subcommand — distinct
// from the all-fatal start-up code (1) so a supervisor or a script can
// tell "this binary was invoked wrong" from "this binary tried and
// failed".
const exitUsage = 2

// usageText is the -h/--help and unknown-subcommand output. The one
// path that writes to stdout is an explicit -h/--help; every other
// path — serving, migrate-only, and every failure — leaves stdout
// empty.
const usageText = `Usage: lab-game bot [migrate]

With no arguments, serve until a signal asks the process to drain.
migrate applies pending database migrations and exits.
`

// run writes the build identity to stderr as its first action — before
// argv dispatch and before configuration are even read — so every
// invocation says which binary it is, including one that dies at the
// configuration step with nothing else to go on. It then dispatches on
// argv: no arguments serves until a signal asks the process to drain;
// "migrate" applies pending migrations and exits; -h or --help prints
// usage to stdout and exits zero — the one path this design lets write
// to stdout; anything else is an unrecognised subcommand, usage on
// stderr and the usage exit code. run never terminates the process
// itself: main exiting non-zero is not a panic.
func run(argv []string, lookup config.Lookup, stderr, stdout io.Writer) int {
	if _, err := fmt.Fprintf(stderr, "lab-game bot %s\n", version); err != nil {
		return 1
	}

	if len(argv) == 1 && (argv[0] == "-h" || argv[0] == "--help") {
		if _, err := fmt.Fprint(stdout, usageText); err != nil {
			return 1
		}
		return 0
	}

	ctx := context.Background()
	opts := assembleOptions{Lookup: lookup, Stderr: stderr, Version: version, StartedAt: time.Now()}

	switch {
	case len(argv) == 0:
		a, err := assemble(ctx, opts)
		if err != nil {
			if _, writeErr := fmt.Fprintf(stderr, "lab-game bot: %v\n", err); writeErr != nil {
				return 1
			}
			return 1
		}
		return a.serve(ctx, a.cfg.Process.ShutdownTimeout, stderr)
	case len(argv) == 1 && argv[0] == "migrate":
		return migrateOnly(ctx, opts)
	default:
		_, _ = fmt.Fprint(stderr, usageText)
		return exitUsage
	}
}
