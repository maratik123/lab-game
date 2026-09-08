package commentref

import (
	"regexp"
	"strings"
)

// Class names one of the lexically decidable banned reference classes.
type Class string

// The banned reference classes the gate decides.
const (
	ClassLocator        Class = "locator"
	ClassMarkdownPath   Class = "markdown-path"
	ClassACID           Class = "ac-id"
	ClassDecisionAnchor Class = "decision-anchor"
	ClassSection        Class = "section"
	ClassIssue          Class = "issue"
	ClassRepoPath       Class = "repo-path"
	ClassURL            Class = "url"
	ClassModuleSymbol   Class = "module-symbol"
)

// Finding is one banned reference the classifier decided, carrying the
// comment's own line, the class it matched, and the exact matched text.
type Finding struct {
	Line  int
	Class Class
	Text  string
}

var (
	reTODOIssue    = regexp.MustCompile(`\bTODO\(#[0-9]+\)`)
	reURL          = regexp.MustCompile(`\bhttps?://\S+`)
	reLocator      = regexp.MustCompile(`\b[A-Za-z0-9_./-]*[./][A-Za-z0-9_./-]*:[0-9]+\b`)
	reMarkdownPath = regexp.MustCompile(`\S*\.md\b`)
	reACID         = regexp.MustCompile(`\bAC[0-9]+\b`)
	reDecisionRef  = regexp.MustCompile(`\b(?:KD-[0-9]+|D[0-9]+)\b`)
	reSection      = regexp.MustCompile(`§`)
	reIssue        = regexp.MustCompile(`#[0-9]+`)
	rePathToken    = regexp.MustCompile(`[A-Za-z0-9_./-]+`)
	reModuleSymbol = regexp.MustCompile(`\b([a-z][A-Za-z0-9]*)\.([A-Z][A-Za-z0-9_]*)\b`)
)

// gatedSourceExts are the extensions of the gated set a repo-path finding
// matches on, independent of any top-level directory.
var gatedSourceExts = []string{".go", ".sh", ".sql", ".yml", ".yaml"}

// gatedRootNames are the repository-root file names a repo-path finding
// matches on when named exactly.
var gatedRootNames = map[string]bool{
	"Makefile":     true,
	".gitignore":   true,
	".env.example": true,
}

// repoTopDirs are this repository's top-level directories; a token whose
// first path segment names one of them is a repo-path finding.
var repoTopDirs = map[string]bool{
	"cmd":       true,
	"internal":  true,
	"docs":      true,
	"ai-docs":   true,
	"config":    true,
	".claude":   true,
	".github":   true,
	".githooks": true,
}

// Classify decides the banned reference classes a single extracted comment
// carries. ownPackage is the comment's own Go package name, or empty for a
// gated file that has no Go package of its own; a trailing `_test` is
// stripped before comparison, so an external test package's own fixtures
// still pass for the package under test. modulePackages is the set of
// package names this module declares, read off the tree the caller is
// running against.
func Classify(c Comment, ownPackage string, modulePackages map[string]struct{}) []Finding {
	ownPackage = strings.TrimSuffix(ownPackage, "_test")
	text, exempt := stripDirective(c.Text)
	if exempt {
		return nil
	}
	text = reTODOIssue.ReplaceAllStringFunc(text, blankOut)

	var findings []Finding
	add := func(class Class, matched string) {
		findings = append(findings, Finding{Line: c.Line, Class: class, Text: matched})
	}

	for _, m := range reURL.FindAllString(text, -1) {
		add(ClassURL, m)
	}
	text = reURL.ReplaceAllStringFunc(text, blankOut)

	for _, m := range reLocator.FindAllString(text, -1) {
		add(ClassLocator, m)
	}
	text = reLocator.ReplaceAllStringFunc(text, blankOut)

	for _, m := range reMarkdownPath.FindAllString(text, -1) {
		add(ClassMarkdownPath, m)
	}
	text = reMarkdownPath.ReplaceAllStringFunc(text, blankOut)

	for _, m := range reACID.FindAllString(text, -1) {
		add(ClassACID, m)
	}
	for _, m := range reDecisionRef.FindAllString(text, -1) {
		add(ClassDecisionAnchor, m)
	}
	for _, m := range reSection.FindAllString(text, -1) {
		add(ClassSection, m)
	}
	for _, m := range reIssue.FindAllString(text, -1) {
		add(ClassIssue, m)
	}
	for _, tok := range rePathToken.FindAllString(text, -1) {
		if isRepoPathToken(tok) {
			add(ClassRepoPath, tok)
		}
	}
	for _, m := range reModuleSymbol.FindAllStringSubmatch(text, -1) {
		pkg := m[1]
		if pkg == ownPackage {
			continue
		}
		if _, ok := modulePackages[pkg]; ok {
			add(ClassModuleSymbol, m[0])
		}
	}

	return findings
}

// stripDirective applies the KD-5 exemptions: a machine-read directive is
// exempt in full, except `nolint:`, whose human reason text — everything
// after the linter list — is classified normally.
func stripDirective(text string) (toClassify string, exempt bool) {
	trimmed := strings.TrimLeft(text, " \t")
	switch {
	case strings.HasPrefix(trimmed, "nolint:"):
		rest := trimmed[len("nolint:"):]
		idx := strings.IndexAny(rest, " \t")
		if idx == -1 {
			return "", true
		}
		return rest[idx:], false
	case strings.HasPrefix(trimmed, "go:"),
		strings.HasPrefix(trimmed, "+build"),
		strings.HasPrefix(trimmed, "+goose"),
		strings.HasPrefix(trimmed, "shellcheck"),
		strings.HasPrefix(trimmed, "!"):
		return "", true
	default:
		return text, false
	}
}

// isRepoPathToken decides whether tok is a repo-path finding: its first
// path segment names a top-level directory of this repository, it ends in
// a source extension of the gated set, or it is one of the repository-root
// file names.
func isRepoPathToken(tok string) bool {
	if gatedRootNames[tok] {
		return true
	}
	for _, ext := range gatedSourceExts {
		if strings.HasSuffix(tok, ext) {
			return true
		}
	}
	if idx := strings.IndexByte(tok, '/'); idx != -1 && repoTopDirs[tok[:idx]] {
		return true
	}
	return false
}

// blankOut replaces a matched span with spaces of the same length, so an
// earlier class's match cannot also be picked up by a later, broader
// pattern, while token positions elsewhere in the string are undisturbed.
func blankOut(m string) string {
	return strings.Repeat(" ", len(m))
}
