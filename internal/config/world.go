package config

import "os"

// resolveWorldPath validates LAB_GAME_WORLD_PATH: present, non-empty, and
// naming a directory that stat resolves — nothing inside it is read here.
// loadWorldSet is what reads the directory's own entries.
func resolveWorldPath(lookup Lookup) (string, error) {
	val, ok := lookup(envWorldPath)
	if !ok {
		return "", keyErrorf(envWorldPath, ErrMissing, "required")
	}
	if val == "" {
		return "", keyErrorf(envWorldPath, ErrInvalidValue, "must not be empty")
	}

	info, err := os.Stat(val)
	if err != nil {
		return "", keyErrorf(envWorldPath, ErrUnreadable, "%s", err)
	}
	if !info.IsDir() {
		return "", keyErrorf(envWorldPath, ErrInvalidValue, "must be a directory")
	}
	return val, nil
}
