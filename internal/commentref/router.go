package commentref

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

// FileClass names which extractor a gated path is read with.
type FileClass int

// The file classes the gate recognises. ClassNone means "not part of the
// gated set" rather than an error.
const (
	ClassNone FileClass = iota
	ClassGo
	ClassShell
	ClassYAML
	ClassSQL
	ClassMakefile
	ClassGitignore
	ClassEnvExample
)

// fileClassNames names each FileClass for String, in declaration order.
var fileClassNames = [...]string{
	ClassNone:       "none",
	ClassGo:         "go",
	ClassShell:      "shell",
	ClassYAML:       "yaml",
	ClassSQL:        "sql",
	ClassMakefile:   "makefile",
	ClassGitignore:  "gitignore",
	ClassEnvExample: "env-example",
}

// String returns c's name, or a numeric placeholder for a value outside
// the declared set.
func (c FileClass) String() string {
	if c < 0 || int(c) >= len(fileClassNames) {
		return fmt.Sprintf("FileClass(%d)", int(c))
	}
	return fileClassNames[c]
}

// ErrUnroutable reports a path under the content-keyed directory that
// matches none of its recognised shapes — the shell extension, a `#!`
// shebang, or a symbolic link to either. The gated set names the
// directory in full, so a member the router cannot classify is an
// instrument failure, not a silent pass.
var ErrUnroutable = errors.New("commentref: path has no recognised shape")

// githooksDir is the one directory whose gated members are not all keyed
// by extension or by a whole file name.
const githooksDir = ".githooks/"

// Outcome is the result of routing one path: either a FileClass ready for
// Extract, or a report that the path is not gated at all, or that it was
// skipped because it is a symbolic link.
type Outcome struct {
	Class   FileClass
	Skipped bool
}

// Route decides which extractor, if any, a path is read with. content is
// consulted only for a path under the content-keyed directory that carries
// no shell extension; every other class is decided by path shape alone.
// A symbolic link is skipped rather than routed, whatever class its own
// name would otherwise match, because it carries no comment of its own —
// its target is scanned under its own path.
func Route(gitPath string, isSymlink bool, content []byte) (Outcome, error) {
	if class, gated := classifyByPath(gitPath); gated {
		if isSymlink {
			return Outcome{Skipped: true}, nil
		}
		return Outcome{Class: class}, nil
	}

	if !strings.HasPrefix(gitPath, githooksDir) {
		return Outcome{}, nil
	}
	if isSymlink {
		return Outcome{Skipped: true}, nil
	}
	if len(content) >= 2 && content[0] == '#' && content[1] == '!' {
		return Outcome{Class: ClassShell}, nil
	}
	return Outcome{}, fmt.Errorf("%w: %s", ErrUnroutable, gitPath)
}

// classifyByPath decides a FileClass from a path's extension or base name
// alone. It reports gated=false for anything that is not part of the
// extension-or-name-keyed portion of the gated set — including a path
// under the content-keyed directory with no shell extension, which Route
// resolves separately.
func classifyByPath(gitPath string) (class FileClass, gated bool) {
	base := path.Base(gitPath)
	switch base {
	case "Makefile":
		return ClassMakefile, true
	case ".gitignore":
		return ClassGitignore, true
	case ".env.example":
		return ClassEnvExample, true
	}

	switch {
	case strings.HasSuffix(gitPath, ".go"):
		return ClassGo, true
	case strings.HasSuffix(gitPath, ".sh"):
		return ClassShell, true
	case strings.HasSuffix(gitPath, ".sql"):
		return ClassSQL, true
	case strings.HasSuffix(gitPath, ".yml"), strings.HasSuffix(gitPath, ".yaml"):
		return ClassYAML, true
	}
	return ClassNone, false
}

// Extract dispatches to the extractor for class.
func Extract(class FileClass, content []byte) ([]Comment, error) {
	switch class {
	case ClassGo:
		return ExtractGo(content)
	case ClassShell:
		return ExtractShell(content)
	case ClassYAML:
		return ExtractYAML(content)
	case ClassSQL:
		return ExtractSQL(content)
	case ClassMakefile:
		return ExtractMakefile(content)
	case ClassGitignore:
		return ExtractGitignore(content)
	case ClassEnvExample:
		return ExtractEnvExample(content)
	default:
		return nil, fmt.Errorf("commentref: no extractor for class %d", class)
	}
}
