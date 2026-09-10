// Command bot is the Telegram bot process for the lab-game maze game: a
// composition root that assembles the storage pool, the metrics and
// readiness listener, the Telegram client, the ingest loop, the task
// scheduler and the restart-hygiene liveness heartbeat, then serves
// until a signal asks it to drain.
package main

import "os"

// version is the build identity reported by the bot; the release
// pipeline overrides it with -ldflags "-X main.version=...". A
// package-level var, not a const: -X only rewrites a var's initial
// value, and a linked-time identity is exactly what this string is for.
var version = "0.0.0-dev"

func main() {
	os.Exit(run(os.Args[1:], os.LookupEnv, os.Stderr, os.Stdout))
}
