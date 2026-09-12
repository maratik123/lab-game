// Package gateguard holds two structural guards that check the module
// points its own goroutine-ownership rules at itself: a table over
// every bare go statement in the module's compiled non-test source, and
// an assertion over the lint configuration's exclusion reach, its
// exclusion scoping, its enabled linter set and its pinned settings.
package gateguard
