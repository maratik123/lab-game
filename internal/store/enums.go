package store

// OwnerKind mirrors the database enum owner_kind: the kind of address space
// an owner row occupies.
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
	KindSlots      Kind = "slots"
	KindWeight     Kind = "weight"
)

var kinds = []Kind{KindMoney, KindExperience, KindSlots, KindWeight}

// CapacityRole mirrors the database enum capacity_role: which half of a
// capacity kind's free/used pair an account_definition row represents.
// The empty string is a Go-side stand-in for SQL NULL — the money and
// experience rows carry no capacity_role — and has no database member of
// its own, so it is deliberately absent from capacityRoles.
type CapacityRole string

// CapacityRole members, in the database's declaration order.
const (
	CapacityFree CapacityRole = "free"
	CapacityUsed CapacityRole = "used"
)

var capacityRoles = []CapacityRole{CapacityFree, CapacityUsed}

// OperationSource mirrors the database enum operation_source: the client
// that originated a player_operation.
type OperationSource string

// OperationSource members, in the database's declaration order.
const (
	SourceTelegram OperationSource = "telegram"
)

var operationSources = []OperationSource{SourceTelegram}

// EventVolumeClass mirrors the database enum event_volume_class: the
// traffic-volume partition of the event-type registry
// (event_type_definition.volume_class), read by the retention pass to
// split storage without re-classifying every type.
type EventVolumeClass string

// EventVolumeClass members, in the database's declaration order.
const (
	VolumeLow  EventVolumeClass = "low_volume"
	VolumeHigh EventVolumeClass = "high_volume"
)

var eventVolumeClasses = []EventVolumeClass{VolumeLow, VolumeHigh}
