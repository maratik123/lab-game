// Command testpg provisions or discovers a shared PostgreSQL server for the
// database-backed test suite, runs a child gate command against it, and
// removes any server it started once the child exits. With no server named
// or found, it falls through to an anonymous, sized container; --up creates
// a long-lived named server and --down removes it.
package main

import "os"

func main() {
	os.Exit(run(os.Args[1:], os.LookupEnv, productionSeam(), os.Stdout, os.Stderr))
}
