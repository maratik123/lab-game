// Command bot is the Telegram bot process for the lab-game maze game.
//
// Scaffold only: the update loop, storage, and raid FSM land with their own
// specs (see docs/DESIGN.md §14 for the MVP scope).
package main

import (
	"fmt"
	"os"
)

// version is the build identity reported by the bot; the release pipeline
// overrides it with -ldflags.
const version = "0.0.0-dev"

func main() {
	if _, err := fmt.Fprintf(os.Stdout, "lab-game bot %s\n", version); err != nil {
		os.Exit(1)
	}
}
