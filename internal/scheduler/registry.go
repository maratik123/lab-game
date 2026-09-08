package scheduler

import "fmt"

// Recurrence pairs a cadence with the configuration key its period value
// came from. It carries no instance-key field: a recurrence's instance
// key is derived as its declared type's name — Registry
// admits one Declaration per type, so a second instance key for the same
// recurrent type would key a declaration that cannot exist.
type Recurrence struct {
	// Cadence computes the next occurrence from the previous run_at and a
	// DB-supplied instant. Must not be nil.
	Cadence Cadence
	// ConfigKey names the configuration key this Cadence's period was
	// read from — a declaration names which key it reads, it does not
	// embed a number. Must not be empty.
	ConfigKey string
}

// Declaration is one registry entry.
type Declaration struct {
	// Type is the persisted task-type name. Must not be empty.
	Type Type
	// Handler executes this type's effects. Must not be nil.
	Handler Handler
	// Recurrence is non-nil for a recurring task type, nil for a
	// one-shot.
	Recurrence *Recurrence
}

// Registry is the immutable set of declared task types. Built once by
// NewRegistry, never mutated afterwards, and therefore safe for
// concurrent use without a lock.
type Registry struct {
	decls map[Type]Declaration
}

// NewRegistry builds a Registry from decls, refusing:
//   - an empty Type;
//   - a nil Handler;
//   - a duplicate Type;
//   - a Recurrence with a nil Cadence;
//   - a Recurrence with an empty ConfigKey.
//
// Every refusal wraps ErrInvalidDeclaration and names the offending type.
func NewRegistry(decls ...Declaration) (*Registry, error) {
	byType := make(map[Type]Declaration, len(decls))
	for _, d := range decls {
		if d.Type == "" {
			return nil, fmt.Errorf("%w: empty type", ErrInvalidDeclaration)
		}
		if d.Handler == nil {
			return nil, fmt.Errorf("%w: type %q: nil handler", ErrInvalidDeclaration, d.Type)
		}
		if _, exists := byType[d.Type]; exists {
			return nil, fmt.Errorf("%w: type %q: duplicate", ErrInvalidDeclaration, d.Type)
		}
		if d.Recurrence != nil {
			if d.Recurrence.Cadence == nil {
				return nil, fmt.Errorf("%w: type %q: recurrence with a nil cadence", ErrInvalidDeclaration, d.Type)
			}
			if d.Recurrence.ConfigKey == "" {
				return nil, fmt.Errorf("%w: type %q: recurrence with an empty configuration key", ErrInvalidDeclaration, d.Type)
			}
		}
		byType[d.Type] = d
	}
	return &Registry{decls: byType}, nil
}

// declaration returns the Declaration for t, and whether one exists.
func (r *Registry) declaration(t Type) (Declaration, bool) {
	d, ok := r.decls[t]
	return d, ok
}
