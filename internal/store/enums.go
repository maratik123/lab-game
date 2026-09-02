package store

// OwnerKind mirrors the database enum owner_kind: the kind of address space
// an owner row occupies (docs/DESIGN.md §11).
type OwnerKind string

// OwnerKind members, in the database's declaration order.
const (
	OwnerWorld  OwnerKind = "world"
	OwnerPlayer OwnerKind = "player"
	OwnerChat   OwnerKind = "chat"
)

var ownerKinds = []OwnerKind{OwnerWorld, OwnerPlayer, OwnerChat}

func (k OwnerKind) known() bool {
	for _, m := range ownerKinds {
		if k == m {
			return true
		}
	}
	return false
}

// Kind mirrors the database enum ledger_kind: the currency an account
// carries, read from account_definition.kind.
type Kind string

// Kind members, in the database's declaration order.
const (
	KindMoney      Kind = "money"
	KindExperience Kind = "experience"
)

var kinds = []Kind{KindMoney, KindExperience}

// OperationSource mirrors the database enum operation_source: the client
// that originated a player_operation.
type OperationSource string

// OperationSource members, in the database's declaration order.
const (
	SourceTelegram OperationSource = "telegram"
)

var operationSources = []OperationSource{SourceTelegram}
