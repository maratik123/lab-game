// Command bot is the Telegram bot process for the lab-game maze game: a
// composition root that assembles the storage pool, the metrics and
// readiness listener, the Telegram client, the ingest loop, the task
// scheduler and the restart-hygiene liveness heartbeat, then serves
// until a signal asks it to drain.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/maratik123/lab-game/internal/config"
)

// version is the build identity reported by the bot; the release
// pipeline overrides it with -ldflags "-X main.version=...". A
// package-level var, not a const: -X only rewrites a var's initial
// value, and a linked-time identity is exactly what this string is for.
var version = "0.0.0-dev"

func main() {
	os.Exit(run(os.LookupEnv, os.Stderr, os.Stdout))
}

// run writes the build identity to stderr as its first action — before
// configuration is even read — so every invocation says which binary it
// is, including one that dies at the configuration step with nothing
// else to go on. It then loads and validates configuration through
// lookup, returning the process exit code: non-zero with the rejected
// or missing keys named on stderr, zero otherwise. stdout carries no
// output on this path — run's argv-dispatch shape (usage on an explicit
// -h/--help) is the one path that writes to it, and lands with the
// serving and migrate-only entry points. run never terminates the
// process itself: main exiting non-zero is not a panic.
func run(lookup config.Lookup, stderr, stdout io.Writer) int {
	_ = stdout // no writer on this path yet — see the doc comment above.

	if _, err := fmt.Fprintf(stderr, "lab-game bot %s\n", version); err != nil {
		return 1
	}

	if _, err := config.Load(lookup); err != nil {
		if _, writeErr := fmt.Fprintf(stderr, "lab-game bot: configuration: %v\n", err); writeErr != nil {
			return 1
		}
		return 1
	}

	return 0
}
