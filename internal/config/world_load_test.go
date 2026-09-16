package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validWorldYAML is a complete, known-good world document — the scalar
// half and the content half together — every negative case in this file
// mutates: one documented change per case.
const validWorldYAML = `id: validation_world
seed: 7
k: 2
generation:
  radius: 9
  island_share: 0.2
  extra_passage_share: 0.1
  growing_tree_bias: 0.5
  portal_share_lower: 0.2
  portal_share_upper: 0.4
  weights:
    backtracker: 1
    kruskal: 1
    prim: 1
    growing_tree: 1
    wilson_walk: 1
resource_profile:
  - kind: spun_sugar
    weight: 1
naming_style:
  templates:
    - "{adjective} {noun}"
  parts:
    adjective:
      - fluffy
    noun:
      - meadow
lexicon:
  ambient:
    - the wind smells of sugar
bestiary:
  - id: cotton_wolf
    name: Cotton Wolf
    role: common
`

// writeWorldDir writes contents to a fresh temp directory as one world
// file and returns the directory's path.
func writeWorldDir(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "world.yaml"), []byte(contents), 0o600); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return dir
}

// secondValidWorldYAML is validWorldYAML with a distinct id, so the
// two-world and duplicate-id cases each have a real second file to work
// with.
const secondValidWorldYAML = `id: second_world
seed: 11
k: 3
generation:
  radius: 6
  island_share: 0
  extra_passage_share: 0
  growing_tree_bias: 0
  portal_share_lower: 0.2
  portal_share_upper: 0.3
  weights:
    backtracker: 1
    kruskal: 0
    prim: 0
    growing_tree: 0
    wilson_walk: 0
resource_profile:
  - kind: pastel_fleece
    weight: 1
naming_style:
  templates:
    - "{color} place"
  parts:
    color:
      - green
lexicon:
  ambient:
    - a second world's own phrase
bestiary:
  - id: second_beast
    name: Second Beast
    role: dangerous
`

func TestLoadWorldSet_OneWorldLoads(t *testing.T) {
	t.Parallel()
	dir := writeWorldDir(t, validWorldYAML)
	worlds, err := loadWorldSet(dir)
	if err != nil {
		t.Fatalf("loadWorldSet: unexpected error: %v", err)
	}
	if len(worlds) != 1 {
		t.Fatalf("worlds = %+v, want exactly one", worlds)
	}
	w := worlds[0]
	if w.ID != "validation_world" || w.Seed != 7 || w.GateSpacing != 2 {
		t.Errorf("worlds[0] = %+v", w)
	}
	if w.Generation.Radius != 9 {
		t.Errorf("Generation.Radius = %d, want 9", w.Generation.Radius)
	}
	if len(w.ResourceProfile) != 1 || w.ResourceProfile[0].Kind != "spun_sugar" {
		t.Errorf("ResourceProfile = %+v", w.ResourceProfile)
	}
	if len(w.Bestiary) != 1 || w.Bestiary[0].ID != "cotton_wolf" {
		t.Errorf("Bestiary = %+v", w.Bestiary)
	}
}

func TestLoadWorldSet_TwoWorldsBothLoad(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(validWorldYAML), 0o600); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.yaml"), []byte(secondValidWorldYAML), 0o600); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	worlds, err := loadWorldSet(dir)
	if err != nil {
		t.Fatalf("loadWorldSet: unexpected error: %v", err)
	}
	if len(worlds) != 2 {
		t.Fatalf("worlds = %+v, want exactly two", worlds)
	}
	ids := map[string]bool{worlds[0].ID: true, worlds[1].ID: true}
	if !ids["validation_world"] || !ids["second_world"] {
		t.Errorf("worlds ids = %v, want both validation_world and second_world", ids)
	}
}

func TestLoadWorldSet_DuplicateIDsAcrossFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(validWorldYAML), 0o600); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	// The second fixture carries the same id as the first, so the
	// duplicate is caught across two distinct files rather than within
	// one.
	dup := strings.Replace(secondValidWorldYAML, "id: second_world\n", "id: validation_world\n", 1)
	if err := os.WriteFile(filepath.Join(dir, "b.yaml"), []byte(dup), 0o600); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	_, err := loadWorldSet(dir)
	if err == nil {
		t.Fatal("loadWorldSet: expected a duplicate-id error")
	}
	if !strings.Contains(err.Error(), "a.yaml") || !strings.Contains(err.Error(), "b.yaml") {
		t.Errorf("error %q does not name both files", err)
	}
	assertKeyError(t, err, ErrInvalidValue, envWorldPath)
}

func TestLoadWorldSet_EmptyDirectory(t *testing.T) {
	t.Parallel()
	_, err := loadWorldSet(t.TempDir())
	assertKeyError(t, err, ErrMissing, envWorldPath)
}

func TestLoadWorldSet_PathIsRegularFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	regular := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(regular, nil, 0o600); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	_, err := loadWorldSet(regular)
	assertKeyError(t, err, ErrUnreadable, envWorldPath)
}

func TestLoadWorldSet_PathDoesNotExist(t *testing.T) {
	t.Parallel()
	_, err := loadWorldSet(filepath.Join(t.TempDir(), "does-not-exist"))
	assertKeyError(t, err, ErrUnreadable, envWorldPath)
}

func TestLoadWorldSet_NonYAMLEntriesAndSubdirectoryAreIgnored(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "world.yaml"), []byte(validWorldYAML), 0o600); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("not a world"), 0o600); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o700); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	worlds, err := loadWorldSet(dir)
	if err != nil {
		t.Fatalf("loadWorldSet: unexpected error: %v", err)
	}
	if len(worlds) != 1 {
		t.Fatalf("worlds = %+v, want exactly one (README.md and subdir/ ignored)", worlds)
	}
}

func TestLoadWorldSet_CrossFieldFold_PortalShareLowerAboveUpper(t *testing.T) {
	t.Parallel()
	yaml := strings.Replace(validWorldYAML, "portal_share_lower: 0.2\n  portal_share_upper: 0.4\n",
		"portal_share_lower: 0.6\n  portal_share_upper: 0.4\n", 1)
	if yaml == validWorldYAML {
		t.Fatal("fixture line not found")
	}
	dir := writeWorldDir(t, yaml)
	_, err := loadWorldSet(dir)
	assertKeyError(t, err, ErrInvalidValue, "generation")
	if !strings.Contains(err.Error(), "portal share lower bound") {
		t.Errorf("error %q does not carry the generator's own wording", err)
	}
}

func TestLoadWorldSet_TwoMalformedWorldsProduceBothFailuresInFileOrder(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	badA := strings.Replace(validWorldYAML, "id: validation_world\n", "", 1)
	badB := strings.Replace(secondValidWorldYAML, "seed: 11\n", "seed: not-an-int\n", 1)
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(badA), 0o600); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.yaml"), []byte(badB), 0o600); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	_, err := loadWorldSet(dir)
	assertKeyError(t, err, ErrMissing, "id")
	assertKeyError(t, err, ErrInvalidValue, "seed")
}

func TestLoadWorldSet_LoadPopulatesConfigWorlds(t *testing.T) {
	t.Parallel()
	env := validConfigEnv(t)
	cfg, err := Load(mapLookup(env))
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
	if len(cfg.Worlds) != 1 || cfg.Worlds[0].ID != "validation_world" {
		t.Errorf("Worlds = %+v", cfg.Worlds)
	}
}

func TestLoadWorldSet_LoadJoinsWorldFailureWithOtherSources(t *testing.T) {
	t.Parallel()
	env := validConfigEnv(t)
	env[envWorldPath] = t.TempDir() // empty: a world-set failure
	delete(env, envBotToken)        // an unrelated environment failure

	_, err := Load(mapLookup(env))
	if err == nil {
		t.Fatal("Load: expected an error")
	}
	if !containsKeyError(err, envWorldPath) {
		t.Errorf("Load: error does not name %s: %v", envWorldPath, err)
	}
	if !containsKeyError(err, envBotToken) {
		t.Errorf("Load: error does not name %s: %v", envBotToken, err)
	}
}

func TestLoadWorldFile_ReadFailureNamesThePath(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "does-not-exist.yaml")
	_, err := loadWorldFile(path)
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("loadWorldFile: error %v does not name %s", err, path)
	}
}

func TestLoadWorldFile_SyntaxErrorNamesThePath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "world.yaml")
	if err := os.WriteFile(path, []byte("id: [unterminated\n"), 0o600); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	_, err := loadWorldFile(path)
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("loadWorldFile: error %v does not name %s", err, path)
	}
}

func TestLoadWorldFile_DuplicateKey(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "world.yaml")
	yaml := strings.Replace(validWorldYAML, "seed: 7\n", "seed: 7\nseed: 8\n", 1)
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	_, err := loadWorldFile(path)
	if err == nil {
		t.Fatal("loadWorldFile: expected an error for a duplicated key")
	}
	if !strings.Contains(err.Error(), "already defined") {
		t.Errorf("loadWorldFile: error %q does not report the duplicate as the parser's own error", err)
	}
}
