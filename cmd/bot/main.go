// Command bot is the Telegram bot process for the lab-game maze game.
//
// Scaffold only: the update loop, storage, and raid FSM land with their own
// specs (see docs/DESIGN.md §14 for the MVP scope). Configuration is
// loaded and validated before any other work (issue #18).
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/maratik123/lab-game/internal/config"
)

// version is the build identity reported by the bot; the release pipeline
// overrides it with -ldflags.
const version = "0.0.0-dev"

func main() {
	os.Exit(run(os.LookupEnv, os.Stderr, os.Stdout))
}

// run loads and validates configuration through lookup before doing
// anything else — configuration failing leaves stdout untouched, only
// stderr — and returns the process exit code: non-zero with the rejected
// or missing keys named on stderr, or zero with the build identity on
// stdout. run never terminates the process itself (AC11): main exiting
// non-zero is not a panic and needs no ai-docs/panic-index.md row.
func run(lookup config.Lookup, stderr, stdout io.Writer) int {
	if _, err := config.Load(lookup); err != nil {
		if _, writeErr := fmt.Fprintf(stderr, "lab-game bot: configuration: %v\n", err); writeErr != nil {
			return 1
		}
		return 1
	}

	if _, err := fmt.Fprintf(stdout, "lab-game bot %s\n", version); err != nil {
		return 1
	}
	return 0
}
