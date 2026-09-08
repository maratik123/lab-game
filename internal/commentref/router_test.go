package commentref

import (
	"errors"
	"testing"
)

func TestRoute_ByExtensionOrName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path string
		want FileClass
	}{
		{"cmd/bot/main.go", ClassGo},
		{"ai-docs/scripts/check-ac-shape.sh", ClassShell},
		{"internal/store/migrations/0001_init.sql", ClassSQL},
		{".github/workflows/ci.yml", ClassYAML},
		{"config/balance.yaml", ClassYAML},
		{"Makefile", ClassMakefile},
		{".gitignore", ClassGitignore},
		{".env.example", ClassEnvExample},
		{".githooks/coverage-ratchet.sh", ClassShell},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			got, err := Route(tc.path, false, nil)
			if err != nil {
				t.Fatalf("Route(%s) error = %v", tc.path, err)
			}
			if got.Skipped {
				t.Fatalf("Route(%s): got Skipped, want routed", tc.path)
			}
			if got.Class != tc.want {
				t.Fatalf("Route(%s).Class = %v, want %v", tc.path, got.Class, tc.want)
			}
		})
	}
}

func TestRoute_PathOutsideGatedSet(t *testing.T) {
	t.Parallel()
	got, err := Route("README.md", false, nil)
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if got.Class != ClassNone || got.Skipped {
		t.Fatalf("Route(README.md) = %+v, want the ungated zero value", got)
	}
}

func TestRoute_GithooksShebangNoExtension(t *testing.T) {
	t.Parallel()
	got, err := Route(".githooks/pre-commit", false, []byte("#!/bin/sh\necho hi\n"))
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if got.Class != ClassShell {
		t.Fatalf("Route(.githooks/pre-commit) = %+v, want ClassShell", got)
	}
}

func TestRoute_GithooksUnrecognisedShapeIsAnInstrumentFailure(t *testing.T) {
	t.Parallel()
	_, err := Route(".githooks/README", false, []byte("not a shebang\n"))
	if !errors.Is(err, ErrUnroutable) {
		t.Fatalf("Route(.githooks/README) error = %v, want ErrUnroutable", err)
	}
}

func TestRoute_SymlinkIsSkippedNotClassified(t *testing.T) {
	t.Parallel()
	tests := []string{"cmd/bot/main.go", ".githooks/pre-commit"}
	for _, p := range tests {
		t.Run(p, func(t *testing.T) {
			t.Parallel()
			got, err := Route(p, true, []byte("#!/bin/sh\n"))
			if err != nil {
				t.Fatalf("Route(%s) error = %v", p, err)
			}
			if !got.Skipped {
				t.Fatalf("Route(%s) = %+v, want Skipped", p, got)
			}
		})
	}
}

// TestRoute_SymlinkTargetStillReported pairs the skip above with the
// requirement that a symlink's own target file, scanned under its own
// path, is still classified normally — so a skip cannot silently become an
// ungated file.
func TestRoute_SymlinkTargetStillReported(t *testing.T) {
	t.Parallel()
	got, err := Route(".githooks/pre-commit.sh", false, []byte("#!/bin/sh\necho hi\n"))
	if err != nil {
		t.Fatalf("Route() error = %v", err)
	}
	if got.Class != ClassShell || got.Skipped {
		t.Fatalf("Route(.githooks/pre-commit.sh) = %+v, want ClassShell, not skipped", got)
	}
}

func TestExtract_UnknownClass(t *testing.T) {
	t.Parallel()
	if _, err := Extract(ClassNone, nil); err == nil {
		t.Fatal("Extract(ClassNone, …) error = nil, want an error")
	}
}
