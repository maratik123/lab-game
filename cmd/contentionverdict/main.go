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

	"github.com/maratik123/lab-game/internal/testdb"
)

// realProbe checks the shared server named by dsn under probeTimeout,
// discarding the connection count the underlying probe also returns:
// this classifier only needs to know the server is answering.
func realProbe(ctx context.Context, dsn string) error {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	_, err := testdb.Probe(ctx, dsn)
	return err
}

func main() {
	os.Exit(run(os.Args[1:], withRetry(realProbe, probeAttempts, probeRetryDelay), os.Stdout, os.Stderr))
}
