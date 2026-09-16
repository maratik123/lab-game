package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/maratik123/lab-game/internal/maze"
)

// worldFileSuffix is the extension a world-set entry must carry to be
// read as a world; every other entry (a subdirectory, a README, any
// other file) is silently ignored.
const worldFileSuffix = ".yaml"

// loadWorldSet reads every world file directly inside dir, decodes and
// validates each against the world schema, and returns every decoded
// World in the sorted order of their file names — a property of this
// code, not of the directory read. A directory holding no world file is
// refused naming the world-path variable; a duplicated id across two
// files is refused naming both.
func loadWorldSet(dir string) ([]World, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, keyErrorf(envWorldPath, ErrUnreadable, "%s", err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), worldFileSuffix) {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	if len(names) == 0 {
		return nil, keyErrorf(envWorldPath, ErrMissing, "no world file found in %s", dir)
	}

	var errs []error
	var worlds []World
	idFiles := map[string]string{}
	for _, name := range names {
		path := filepath.Join(dir, name)
		w, err := loadWorldFile(path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if other, dup := idFiles[w.ID]; dup {
			errs = append(errs, keyErrorf(envWorldPath, ErrInvalidValue,
				"duplicate world id %q in %s and %s", w.ID, other, path))
			continue
		}
		idFiles[w.ID] = path
		worlds = append(worlds, *w)
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return worlds, nil
}

// loadWorldFile reads, parses and validates one world file at path
// against the scalar and content world schemas, cross-checks the naming
// style's slots, and folds the generation inputs through the generator's
// own constructor — the single source of every cross-field refusal.
func loadWorldFile(path string) (*World, error) {
	//nolint:gosec // G304: path is drawn from the operator-supplied LAB_GAME_WORLD_PATH directory's own listing.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: world file %s: %w", path, err)
	}
	if err := checkDuplicateKeys(data); err != nil {
		return nil, fmt.Errorf("config: world file %s: %w", path, err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("config: world file %s: parse: %w", path, err)
	}

	var root *yaml.Node
	if !doc.IsZero() && len(doc.Content) > 0 {
		root = doc.Content[0]
	}

	w := &World{}
	tree := buildSchemaTree(append(worldScalarSchema(w), worldContentSchema(w)...))
	var errs []error
	errs = append(errs, walkNode(root, tree, nil, "world")...)
	if len(errs) == 0 {
		errs = append(errs, checkNamingStyleSlots(w.NamingStyle)...)
	}
	if len(errs) == 0 {
		if _, err := maze.New(w.Seed, w.Generation); err != nil {
			errs = append(errs, keyErrorf("generation", ErrInvalidValue, "%s", err))
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return w, nil
}
