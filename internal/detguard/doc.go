// Package detguard holds the shared determinism predicates that every
// determinism-path package in this module checks its own non-test
// source against: no wall-clock read, no non-deterministic random
// source, no unpinned math/rand/v2 identifier beyond the ChaCha8 stream
// constructor, no bare float32/float64 type name and no float-valued
// decimal accessor at every route this package's AST walk reaches, and
// no map ranging as far as a name-based scan can see one. It composes
// the shared directory-walking and file-parsing helper and performs no
// walking of its own — every predicate, what a guard actually forbids,
// lives here, the package that owns the cross-package proposition "this
// package is on the determinism path."
package detguard
