package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWorldPath_VariableUnset(t *testing.T) {
	t.Parallel()
	_, err := resolveWorldPath(mapLookup(map[string]string{}))
	assertKeyError(t, err, ErrMissing, envWorldPath)
}

func TestResolveWorldPath_VariableEmpty(t *testing.T) {
	t.Parallel()
	_, err := resolveWorldPath(mapLookup(map[string]string{envWorldPath: ""}))
	assertKeyError(t, err, ErrInvalidValue, envWorldPath)
}

func TestResolveWorldPath_NonExistentPath(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "does-not-exist")
	_, err := resolveWorldPath(mapLookup(map[string]string{envWorldPath: path}))
	assertKeyError(t, err, ErrUnreadable, envWorldPath)
}

func TestResolveWorldPath_ParentIsRegularFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	regular := filepath.Join(dir, "regular")
	if err := os.WriteFile(regular, nil, 0o600); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	// regular is a file, so a path treating it as a directory can never
	// resolve for any uid — deterministic without a chmod fixture.
	path := filepath.Join(regular, "subpath")
	_, err := resolveWorldPath(mapLookup(map[string]string{envWorldPath: path}))
	assertKeyError(t, err, ErrUnreadable, envWorldPath)
}

func TestResolveWorldPath_ExistingDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	got, err := resolveWorldPath(mapLookup(map[string]string{envWorldPath: dir}))
	if err != nil {
		t.Fatalf("resolveWorldPath: unexpected error: %v", err)
	}
	if got != dir {
		t.Errorf("resolveWorldPath = %q, want %q", got, dir)
	}
}

func TestResolveWorldPath_ExistingRegularFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	regular := filepath.Join(dir, "world-set")
	if err := os.WriteFile(regular, nil, 0o600); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	got, err := resolveWorldPath(mapLookup(map[string]string{envWorldPath: regular}))
	if err != nil {
		t.Fatalf("resolveWorldPath: unexpected error: %v", err)
	}
	if got != regular {
		t.Errorf("resolveWorldPath = %q, want %q", got, regular)
	}
}
