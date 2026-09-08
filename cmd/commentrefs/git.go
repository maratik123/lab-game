package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// runGit invokes git with a fixed argument vector and no shell, from dir.
func runGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // fixed argument vector, no shell — args name git subcommands and repository-relative paths only

	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// repoRoot returns the worktree root, or an error if the working directory
// is not inside a git worktree.
func repoRoot(ctx context.Context) (string, error) {
	out, err := runGit(ctx, ".", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("no git worktree: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// trackedPaths lists every tracked path in the worktree.
func trackedPaths(ctx context.Context, root string) ([]string, error) {
	out, err := runGit(ctx, root, "ls-files", "-z")
	if err != nil {
		return nil, err
	}
	return splitNulTerminated(out), nil
}

// stagedEntry is one path staged in the index, with the mode git will
// commit it as.
type stagedEntry struct {
	path string
	mode string // e.g. "100644", "120000" (symlink)
}

// stagedEntries lists the staged, gated-candidate paths of the index whose
// status is Added, Copied, Modified or Renamed — the set a commit will
// actually write.
func stagedEntries(ctx context.Context, root string) ([]stagedEntry, error) {
	out, err := runGit(ctx, root, "diff", "--cached", "--raw", "-z")
	if err != nil {
		return nil, err
	}
	fields := splitNulTerminated(out)

	var entries []stagedEntry
	for i := 0; i < len(fields); i++ {
		rec := fields[i]
		if !strings.HasPrefix(rec, ":") {
			continue
		}
		cols := strings.Fields(rec)
		if len(cols) < 5 {
			continue
		}
		newMode := strings.TrimPrefix(cols[1], ":")
		status := cols[4]
		if len(status) == 0 || !strings.ContainsRune("ACMR", rune(status[0])) {
			continue
		}
		i++
		if i >= len(fields) {
			break
		}
		path := fields[i]
		if status[0] == 'R' {
			// A rename record carries the destination path next.
			i++
			if i >= len(fields) {
				break
			}
			path = fields[i]
		}
		entries = append(entries, stagedEntry{path: path, mode: newMode})
	}
	return entries, nil
}

// indexBlob reads path's staged content from the index (stage 0).
func indexBlob(ctx context.Context, root, path string) ([]byte, error) {
	return runGit(ctx, root, "show", ":"+path)
}

// splitNulTerminated splits a NUL-terminated byte stream from a `git -z`
// invocation into its fields, dropping the trailing empty field.
func splitNulTerminated(b []byte) []string {
	s := strings.TrimSuffix(string(b), "\x00")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\x00")
}

// modulePackageNames returns the set of package names this module's own
// internal packages declare, read off the tree at dir rather than from any
// fixed list.
func modulePackageNames(ctx context.Context, dir string) (map[string]struct{}, error) {
	cmd := exec.CommandContext(ctx, "go", "list", "-f", "{{.Name}}", "./internal/...")
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("go list ./internal/...: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	names := make(map[string]struct{})
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if line == "" {
			continue
		}
		names[line] = struct{}{}
	}
	return names, nil
}
