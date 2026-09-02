package store

// ScopeDefinition mirrors a row of the seeded scope_definition catalog: a
// named scope and which owner kind has it.
type ScopeDefinition struct {
	ID        int16
	Code      string
	OwnerKind OwnerKind
}

// AccountDefinition mirrors a row of the seeded account_definition catalog:
// a named account inside a scope, its ledger kind, and whether it is
// controlled — i.e. carries an account_balance row (KD-28).
type AccountDefinition struct {
	ID                int16
	ScopeDefinitionID int16
	Code              string
	Kind              Kind
	Controlled        bool
}

// scopeDefinitions mirrors migration 00001_ledger_core.sql's seeded
// scope_definition rows exactly.
var scopeDefinitions = []ScopeDefinition{
	{ID: 1, Code: "world", OwnerKind: OwnerWorld},
	{ID: 2, Code: "attributes", OwnerKind: OwnerPlayer},
}

// accountDefinitions mirrors migration 00001_ledger_core.sql's seeded
// account_definition rows exactly.
var accountDefinitions = []AccountDefinition{
	{ID: 1, ScopeDefinitionID: 1, Code: "money", Kind: KindMoney, Controlled: false},
	{ID: 2, ScopeDefinitionID: 1, Code: "experience", Kind: KindExperience, Controlled: false},
	{ID: 3, ScopeDefinitionID: 2, Code: "money", Kind: KindMoney, Controlled: true},
	{ID: 4, ScopeDefinitionID: 2, Code: "experience", Kind: KindExperience, Controlled: true},
}
