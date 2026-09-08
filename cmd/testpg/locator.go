package main

import (
	"os"
	"strings"
)

// locatorPath is where the long-lived server's connection string is
// recorded: the repository's own ignored scratch directory, relative to
// the working directory every target and hook in this project already
// invokes its recipes from.
const locatorPath = "tmp/testpg-dsn"

// readLocator reads the locator file and returns its trimmed content. ok is
// false when the file does not exist or is empty — both read as "no
// long-lived server known", never as an error.
func readLocator() (dsn string, ok bool) {
	contents, err := os.ReadFile(locatorPath)
	if err != nil {
		return "", false
	}
	dsn = strings.TrimSpace(string(contents))
	return dsn, dsn != ""
}

// writeLocator records dsn in the locator file, creating its parent
// directory if needed.
func writeLocator(dsn string) error {
	if err := os.MkdirAll("tmp", 0o750); err != nil {
		return err
	}
	return os.WriteFile(locatorPath, []byte(dsn+"\n"), 0o644) //nolint:gosec // the locator file names a test-only DSN under the ignored scratch directory, not a secret needing tighter permissions
}

// removeLocator deletes the locator file. Removing an already-absent file
// is not an error: that is exactly the state removeLocator is meant to
// reach.
func removeLocator() error {
	err := os.Remove(locatorPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
