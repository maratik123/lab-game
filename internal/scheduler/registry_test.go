package scheduler

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// fakeHandler is a Handler that never runs in these package-foundation
// tests — only NewRegistry's construction-time refusals are under test
// here.
type fakeHandler struct{}

func (fakeHandler) Execute(context.Context, pgx.Tx, Task) (Outcome, error) {
	return OutcomeDone, nil
}

func TestNewRegistry_refusesEmptyType(t *testing.T) {
	t.Parallel()
	_, err := NewRegistry(Declaration{Type: "", Handler: fakeHandler{}})
	if !errors.Is(err, ErrInvalidDeclaration) {
		t.Fatalf("NewRegistry(empty type) = %v, want ErrInvalidDeclaration", err)
	}
}

func TestNewRegistry_refusesNilHandler(t *testing.T) {
	t.Parallel()
	_, err := NewRegistry(Declaration{Type: "t", Handler: nil})
	if !errors.Is(err, ErrInvalidDeclaration) {
		t.Fatalf("NewRegistry(nil handler) = %v, want ErrInvalidDeclaration", err)
	}
}

func TestNewRegistry_refusesDuplicateType(t *testing.T) {
	t.Parallel()
	_, err := NewRegistry(
		Declaration{Type: "t", Handler: fakeHandler{}},
		Declaration{Type: "t", Handler: fakeHandler{}},
	)
	if !errors.Is(err, ErrInvalidDeclaration) {
		t.Fatalf("NewRegistry(duplicate type) = %v, want ErrInvalidDeclaration", err)
	}
}

func TestNewRegistry_refusesRecurrenceWithNilCadence(t *testing.T) {
	t.Parallel()
	_, err := NewRegistry(Declaration{Type: "t", Handler: fakeHandler{}, Recurrence: &Recurrence{ConfigKey: "k"}})
	if !errors.Is(err, ErrInvalidDeclaration) {
		t.Fatalf("NewRegistry(nil cadence) = %v, want ErrInvalidDeclaration", err)
	}
}

func TestNewRegistry_refusesRecurrenceWithEmptyConfigKey(t *testing.T) {
	t.Parallel()
	_, err := NewRegistry(Declaration{Type: "t", Handler: fakeHandler{}, Recurrence: &Recurrence{Cadence: Every(time.Second)}})
	if !errors.Is(err, ErrInvalidDeclaration) {
		t.Fatalf("NewRegistry(empty config key) = %v, want ErrInvalidDeclaration", err)
	}
}

func TestNewRegistry_acceptsValidDeclarations(t *testing.T) {
	t.Parallel()
	reg, err := NewRegistry(
		Declaration{Type: "oneshot", Handler: fakeHandler{}},
		Declaration{Type: "recurrent", Handler: fakeHandler{}, Recurrence: &Recurrence{Cadence: Every(time.Second), ConfigKey: "k"}},
	)
	if err != nil {
		t.Fatalf("NewRegistry(valid): %v", err)
	}
	if _, ok := reg.declaration("oneshot"); !ok {
		t.Fatalf("expected oneshot to be declared")
	}
	if _, ok := reg.declaration("recurrent"); !ok {
		t.Fatalf("expected recurrent to be declared")
	}
	if _, ok := reg.declaration("nope"); ok {
		t.Fatalf("expected nope to be undeclared")
	}
}

func TestNewRegistry_errorNamesOffendingType(t *testing.T) {
	t.Parallel()
	_, err := NewRegistry(Declaration{Type: "widget", Handler: nil})
	if err == nil {
		t.Fatalf("expected an error")
	}
	if got := err.Error(); !strings.Contains(got, "widget") {
		t.Fatalf("error %q does not name the offending type %q", got, "widget")
	}
}
