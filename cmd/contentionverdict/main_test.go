package main

import (
	"context"
	"os"
	"testing"

	"github.com/maratik123/lab-game/internal/leaktest"
)

func TestMain(m *testing.M) {
	os.Exit(leaktest.Main(m, (*testing.M).Run))
}

// TestRealProbe_unreachableServer_reportsAnError confirms the real
// liveness probe reports a server it cannot reach as an error rather than
// swallowing it: the classification reads nothing else about the server's
// state, so a swallowed dial failure would be published as a sound
// instrument.
func TestRealProbe_unreachableServer_reportsAnError(t *testing.T) {
	t.Parallel()

	// Port 1 is reserved and nothing listens on it, so the dial is refused
	// at once rather than waiting out probeTimeout.
	if err := realProbe(context.Background(), "postgres://nobody:nobody@127.0.0.1:1/postgres?sslmode=disable"); err == nil {
		t.Error("realProbe() = nil, want an error for a server that is not listening")
	}
}
