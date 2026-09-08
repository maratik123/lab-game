// Command commentrefs is the comment reference gate: it decides, for every
// file of the gated set, whether a comment carries a reference of a banned
// class, and reports each finding as `<file>:<line>: <class>: <text>`.
package main

import "os"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
