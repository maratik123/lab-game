package contract

import "fmt"

// Declaration pairs a DocumentType with the Signature a conforming
// document of that type must produce.
type Declaration struct {
	Type      DocumentType
	Signature Signature
}

// Registry holds one Signature per DocumentType. Build it with
// NewRegistry; there is no other constructor, so no caller can loosen a
// type's declared signature after the registry is built.
type Registry struct {
	byType map[DocumentType]Signature
}

// NewRegistry builds a Registry from declarations. It refuses:
//   - a declaration under the player_operation basis, which has no type
//     column of its own to declare a signature for;
//   - a duplicate DocumentType;
//   - the zero Signature;
//   - Expect with no legs;
//   - AnyBalanced on any type other than ManualCorrection;
//   - a leg with an invalid cardinality, an unset sign on a posting leg,
//     an empty scope, account or to-scope code, or an empty kind;
//   - a duplicate leg key within one Signature.
//
// Every refusal wraps ErrInvalidDeclaration and names the offending type.
func NewRegistry(declarations ...Declaration) (*Registry, error) {
	byType := make(map[DocumentType]Signature, len(declarations))
	for _, d := range declarations {
		if !d.Type.known() {
			return nil, fmt.Errorf("%w: %s: not a declarable document type", ErrInvalidDeclaration, d.Type)
		}
		if d.Type.basis == basisPlayerOperation {
			return nil, fmt.Errorf("%w: %s: a player operation has no type column and cannot declare a signature", ErrInvalidDeclaration, d.Type)
		}
		if _, exists := byType[d.Type]; exists {
			return nil, fmt.Errorf("%w: %s: duplicate declaration", ErrInvalidDeclaration, d.Type)
		}
		if err := validateSignature(d.Type, d.Signature); err != nil {
			return nil, err
		}
		byType[d.Type] = d.Signature
	}
	return &Registry{byType: byType}, nil
}

// validateSignature checks sig against every rule NewRegistry's doc
// comment states, for the document type t it was declared under.
func validateSignature(t DocumentType, sig Signature) error {
	switch sig.form {
	case signatureZero:
		return fmt.Errorf("%w: %s: signature is the zero value", ErrInvalidDeclaration, t)
	case signatureAnyBalanced:
		if t.basis != basisManualCorrection {
			return fmt.Errorf("%w: %s: AnyBalanced is only valid for ManualCorrection", ErrInvalidDeclaration, t)
		}
		return nil
	case signatureExpect:
		if len(sig.legs) == 0 {
			return fmt.Errorf("%w: %s: Expect declares no legs", ErrInvalidDeclaration, t)
		}
		seen := make(map[any]struct{}, len(sig.legs))
		for _, leg := range sig.legs {
			if err := leg.validate(); err != nil {
				return fmt.Errorf("%w: %s: %w", ErrInvalidDeclaration, t, err)
			}
			key := leg.key()
			if _, dup := seen[key]; dup {
				return fmt.Errorf("%w: %s: duplicate leg key %v", ErrInvalidDeclaration, t, key)
			}
			seen[key] = struct{}{}
		}
		return nil
	default:
		return fmt.Errorf("%w: %s: unknown signature form", ErrInvalidDeclaration, t)
	}
}
