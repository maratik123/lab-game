package config

import (
	"errors"
	"fmt"
)

// Sentinels for the failure classes a configuration source can raise. Every
// concrete failure is a *KeyError wrapping one of these; compare with
// errors.Is.
var (
	// ErrMissing is returned when a required key or environment variable is
	// absent.
	ErrMissing = errors.New("config: missing")

	// ErrUnknownKey is returned when a balance file contains a key the
	// schema does not declare.
	ErrUnknownKey = errors.New("config: unknown key")

	// ErrInvalidValue is returned when a key is present but its value has
	// the wrong shape, the wrong YAML tag, or fails its predicate.
	ErrInvalidValue = errors.New("config: invalid value")

	// ErrUnreadable is returned when a file-system path named by
	// configuration cannot be opened.
	ErrUnreadable = errors.New("config: unreadable path")
)

// KeyError names the dotted balance-file key path or environment-variable
// name that a configuration failure belongs to, and wraps the sentinel that
// classifies the failure together with the underlying cause.
type KeyError struct {
	// Key is the dotted balance-file path (e.g. "raid.stamina.cap") or the
	// environment-variable name (e.g. "LAB_GAME_BOT_TOKEN") the failure
	// belongs to.
	Key string
	// Err is the sentinel (ErrMissing, ErrUnknownKey, ErrInvalidValue, or
	// ErrUnreadable) wrapped together with the underlying cause via
	// fmt.Errorf's %w, or the sentinel alone.
	Err error
}

// Error renders "config: <key>: <cause>".
func (e *KeyError) Error() string {
	return fmt.Sprintf("config: %s: %s", e.Key, e.Err)
}

// Unwrap returns the wrapped sentinel/cause so errors.Is and errors.As see
// through to it.
func (e *KeyError) Unwrap() error {
	return e.Err
}

// keyErrorf builds a *KeyError for key, wrapping sentinel together with a
// formatted cause via %w so errors.Is(err, sentinel) still succeeds.
func keyErrorf(key string, sentinel error, format string, args ...any) *KeyError {
	return &KeyError{Key: key, Err: fmt.Errorf("%w: %s", sentinel, fmt.Sprintf(format, args...))}
}
