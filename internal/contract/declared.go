package contract

// declarations is the whole module's shipped set of posting signatures,
// one element per declared DocumentType. A mechanic that moves balances
// adds its own type's row here, in its own pull request, with every leg
// on its own line naming the catalog codes, kind, sign and cardinality
// that changed.
var declarations = []Declaration{
	{Type: ManualCorrection(), Signature: AnyBalanced()},
}

// Declared returns the Registry built from this module's shipped
// declarations.
func Declared() (*Registry, error) {
	return NewRegistry(declarations...)
}
