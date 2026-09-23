// Command contentionverdict classifies one run of the race gate under
// induced cross-package load, so that the run's status is classified
// before it is believed. It reports an instrument failure — the shared
// server died, so the run says nothing about contention in either
// direction — distinctly from the gate's own verdict, which it passes
// through unaltered.
package main

import (
	"context"
	"os"
	"time"

	"github.com/maratik123/lab-game/internal/testdb"
)

// realProbe reads the shared server's own last start instant under
// probeTimeout. The connection answers liveness and the value answers
// continuity: a server that died and came back reports a start instant
// inside the run it was supposed to have survived. The reading is the
// postmaster's, so a crash recovery the postmaster itself outlived leaves
// it unchanged — that failure announces itself in the child logs instead.
func realProbe(ctx context.Context, dsn string) (time.Time, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	return testdb.PostmasterStartTime(ctx, dsn)
}

func main() {
	os.Exit(run(os.Args[1:], withRetry(realProbe, probeAttempts, probeRetryDelay), os.Stdout, os.Stderr))
}
