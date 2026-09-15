package contract

import "errors"

// Sentinel errors this package returns.
var (
	// ErrInvalidDeclaration is returned by NewRegistry when a Declaration
	// or its Signature cannot be registered.
	ErrInvalidDeclaration = errors.New("contract: invalid declaration")
	// ErrNonconforming is returned by (*Registry).Check when a document's
	// actual postings or item movements do not match its declared
	// signature, including a set that does not sum to zero per kind.
	ErrNonconforming = errors.New("contract: document does not conform to its signature")
	// ErrNoDocument is returned by (*Registry).Check when no journal
	// entry of the wanted type exists past the mark.
	ErrNoDocument = errors.New("contract: no document of the wanted type past the mark")
	// ErrNoSignature is returned by (*Registry).Check when a journal
	// entry past the mark has a type with no declared signature.
	ErrNoSignature = errors.New("contract: document type has no declared signature")
)
