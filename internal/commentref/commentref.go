// Package commentref extracts and classifies comments in the file classes a
// pre-commit and CI gate must judge. It answers one question per file: which
// byte ranges are comments, and which of those carry a reference this
// project's rule forbids. Extraction is one file per grammar; classification
// is a single pass shared by every grammar.
package commentref

// Comment is one comment as a grammar's extractor reports it: a source line
// and the comment's own text with its marker (//, #, --, /* */) removed.
// Line is one-based. A block comment spanning several source lines is
// reported as one Comment per line, so every finding names a single line.
type Comment struct {
	Line int
	Text string
}
