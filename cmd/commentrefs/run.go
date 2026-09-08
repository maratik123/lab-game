package main

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"

	"github.com/maratik123/lab-game/internal/commentref"
)

// Exit codes: 0 = no finding, 1 = at least one finding, 2 = the gate could
// not run at all — an instrument failure, never read as a clean tree.
const (
	exitClean    = 0
	exitFindings = 1
	exitFailure  = 2
)

// run implements the command's whole behaviour over a testable signature:
// no arguments scans the whole tracked, gated set from the worktree;
// "--staged" scans the staged, gated paths of the index, read from their
// index blobs; explicit paths scan those paths from the worktree.
func run(args []string, stdout, stderr io.Writer) int {
	ctx := context.Background()

	root, err := repoRoot(ctx)
	if err != nil {
		printf(stderr, "commentrefs: %v\n", err)
		return exitFailure
	}

	// A module-symbol finding needs this module's own package names; their
	// absence (no go.mod reachable, no Go toolchain) narrows what the gate
	// can decide rather than stopping it outright — every other class is
	// still fully decidable.
	modulePackages, err := modulePackageNames(ctx, root)
	if err != nil {
		printf(stderr, "commentrefs: module package names unavailable, module-symbol findings are skipped: %v\n", err)
		modulePackages = map[string]struct{}{}
	}

	var files []fileInput
	switch {
	case len(args) == 1 && args[0] == "--staged":
		files, err = gatherStaged(ctx, root)
	case len(args) > 0:
		files, err = gatherWorktreePaths(root, args)
	default:
		var paths []string
		paths, err = trackedPaths(ctx, root)
		if err == nil {
			files, err = gatherWorktreePaths(root, paths)
		}
	}
	if err != nil {
		printf(stderr, "commentrefs: %v\n", err)
		return exitFailure
	}

	return scan(files, modulePackages, stdout, stderr)
}

// fileInput is one path to route and, unless it is a symbolic link, its
// content.
type fileInput struct {
	path      string
	isSymlink bool
	content   []byte
}

// gatherWorktreePaths resolves each of paths against root, reading its
// content from the worktree unless it is a symbolic link.
func gatherWorktreePaths(root string, paths []string) ([]fileInput, error) {
	files := make([]fileInput, 0, len(paths))
	for _, p := range paths {
		full := filepath.Join(root, filepath.FromSlash(p))
		info, err := os.Lstat(full) //nolint:gosec // full is joined under root from git's own tracked-path listing, not from an external request
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", p, err)
		}
		isSymlink := info.Mode()&os.ModeSymlink != 0
		var content []byte
		if !isSymlink {
			content, err = os.ReadFile(full) //nolint:gosec // same path, read only after the Lstat above confirms it is not a symlink
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", p, err)
			}
		}
		files = append(files, fileInput{path: p, isSymlink: isSymlink, content: content})
	}
	return files, nil
}

// gatherStaged resolves the staged, gated-candidate entries of the index,
// reading each non-symlink entry's content from its index blob rather than
// the worktree, so a partial stage is judged as it will be committed.
func gatherStaged(ctx context.Context, root string) ([]fileInput, error) {
	entries, err := stagedEntries(ctx, root)
	if err != nil {
		return nil, err
	}
	files := make([]fileInput, 0, len(entries))
	for _, e := range entries {
		isSymlink := e.mode == "120000"
		var content []byte
		if !isSymlink {
			content, err = indexBlob(ctx, root, e.path)
			if err != nil {
				return nil, fmt.Errorf("read staged %s: %w", e.path, err)
			}
		}
		files = append(files, fileInput{path: e.path, isSymlink: isSymlink, content: content})
	}
	return files, nil
}

// scan routes and classifies every file, printing one report line per
// finding and one skip line per symbolic link, and returns the process
// exit code.
func scan(files []fileInput, modulePackages map[string]struct{}, stdout, stderr io.Writer) int {
	hadError := false
	hadFinding := false

	for _, f := range files {
		outcome, err := commentref.Route(f.path, f.isSymlink, f.content)
		if err != nil {
			printf(stderr, "commentrefs: %v\n", err)
			hadError = true
			continue
		}
		if outcome.Skipped {
			printf(stdout, "SKIP %s: symbolic link\n", f.path)
			continue
		}
		if outcome.Class == commentref.ClassNone {
			continue
		}

		comments, err := commentref.Extract(outcome.Class, f.content)
		if err != nil {
			printf(stderr, "commentrefs: %s: %v\n", f.path, err)
			hadError = true
			continue
		}

		ownPackage := ""
		if outcome.Class == commentref.ClassGo {
			ownPackage = goPackageClause(f.content)
		}

		for _, c := range comments {
			for _, finding := range commentref.Classify(c, ownPackage, modulePackages) {
				printf(stdout, "%s:%d: %s: %s\n", f.path, finding.Line, finding.Class, finding.Text)
				hadFinding = true
			}
		}
	}

	switch {
	case hadError:
		return exitFailure
	case hadFinding:
		return exitFindings
	default:
		return exitClean
	}
}

// printf writes a report or diagnostic line, discarding the write error:
// a broken stdout/stderr pipe leaves nothing more this command can do
// about it, and the exit code already reflects what was found.
func printf(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

// goPackageClause returns the package name declared at the top of a Go
// source file, or "" if it cannot be parsed — the classifier then treats
// every module-qualified symbol as outside the comment's own package,
// which is the conservative direction.
func goPackageClause(src []byte) string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.PackageClauseOnly)
	if err != nil {
		return ""
	}
	return f.Name.Name
}
