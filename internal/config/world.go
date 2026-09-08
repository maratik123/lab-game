package config

import "os"

// resolveWorldPath validates LAB_GAME_WORLD_PATH: present, non-empty, and
// naming a target that opens and closes cleanly (existence and
// readability) — nothing inside it is read. The package declares no type
// describing the world set's interior; the caller gets back the path
// string only, so the loader constrains neither a file nor a directory
// target.
func resolveWorldPath(lookup Lookup) (string, error) {
	val, ok := lookup(envWorldPath)
	if !ok {
		return "", keyErrorf(envWorldPath, ErrMissing, "required")
	}
	if val == "" {
		return "", keyErrorf(envWorldPath, ErrInvalidValue, "must not be empty")
	}

	//nolint:gosec // G304: val is the operator-supplied LAB_GAME_WORLD_PATH value — opening it to probe readability is the feature.
	f, err := os.Open(val)
	if err != nil {
		return "", keyErrorf(envWorldPath, ErrUnreadable, "%s", err)
	}
	if err := f.Close(); err != nil {
		return "", keyErrorf(envWorldPath, ErrUnreadable, "close: %s", err)
	}
	return val, nil
}
