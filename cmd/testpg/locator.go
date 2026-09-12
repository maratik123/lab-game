package main

import (
	"os"
	"strconv"
	"strings"
)

// locatorPath is where the long-lived server's connection string and the
// client count it was provisioned for are recorded: the repository's own
// ignored scratch directory, relative to the working directory every target
// and hook in this project already invokes its recipes from.
const locatorPath = "tmp/testpg-dsn"

// legacyLocatorClients is the client count assigned to a locator file that
// carries no recorded count at all — a file written before this wrapper
// started recording one, or one whose second line does not parse as a
// positive integer. It fails closed: a server whose sizing is unrecorded is
// treated as sized for the smallest possible run, so any run asking for more
// than one client declines it and provisions its own rather than trusting a
// mount that might be too small.
const legacyLocatorClients = 1

// readLocator reads the locator file and returns its trimmed DSN (the first
// line) and the client count the server was provisioned for (the second
// line). ok is false when the file does not exist, is empty, or has no
// parseable DSN on its first line — all read as "no long-lived server
// known", never as an error. A missing or unparseable second line reads as
// legacyLocatorClients, never as an error and never as "big enough".
func readLocator() (dsn string, clients int, ok bool) {
	contents, err := os.ReadFile(locatorPath)
	if err != nil {
		return "", 0, false
	}
	text := strings.TrimSpace(string(contents))
	if text == "" {
		return "", 0, false
	}

	lines := strings.SplitN(text, "\n", 2)
	dsn = strings.TrimSpace(lines[0])
	if dsn == "" {
		return "", 0, false
	}

	clients = legacyLocatorClients
	if len(lines) == 2 {
		if n, err := strconv.Atoi(strings.TrimSpace(lines[1])); err == nil && n > 0 {
			clients = n
		}
	}
	return dsn, clients, true
}

// writeLocator records dsn and the client count the server was provisioned
// for in the locator file, creating its parent directory if needed. clients
// below 1 is recorded as legacyLocatorClients, matching how readLocator
// treats an absent or unparseable count.
func writeLocator(dsn string, clients int) error {
	if clients < 1 {
		clients = legacyLocatorClients
	}
	if err := os.MkdirAll("tmp", 0o750); err != nil {
		return err
	}
	content := dsn + "\n" + strconv.Itoa(clients) + "\n"
	return os.WriteFile(locatorPath, []byte(content), 0o644) //nolint:gosec // the locator file names a test-only DSN under the ignored scratch directory, not a secret needing tighter permissions
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
