// Command importguard is the transitive-dependency gate: it holds a
// rule table naming a package and the module prefixes its non-test
// dependency graph may never carry, and reports every violation found
// against the real tree.
package main

import "os"

func main() {
	os.Exit(run(".", productionRules(), os.Stdout, os.Stderr))
}
