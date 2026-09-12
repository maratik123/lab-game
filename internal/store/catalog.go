package store

// ScopeDefinition mirrors a row of the seeded scope_definition catalog: a
// named scope and which owner kind has it.
type ScopeDefinition struct {
	ID        int16
	Code      string
	OwnerKind OwnerKind
}

// AccountDefinition mirrors a row of the seeded account_definition catalog:
// a named account inside a scope, its ledger kind, whether it is
// controlled — i.e. carries an account_balance row — and, for a capacity
// account, which half of its kind's free/used pair it is. CapacityRole is
// the empty string for SQL NULL (the money and experience rows), read via
// COALESCE(capacity_role::text, the empty SQL string), never a pointer,
// because the mirror comparison below is element-for-element on a
// comparable struct.
type AccountDefinition struct {
	ID                int16
	ScopeDefinitionID int16
	Code              string
	Kind              Kind
	Controlled        bool
	CapacityRole      CapacityRole
}

// scopeDefinitions mirrors the ledger-core and item-machine migrations'
// seeded scope_definition rows exactly.
var scopeDefinitions = []ScopeDefinition{
	{ID: 1, Code: "world", OwnerKind: OwnerWorld},
	{ID: 2, Code: "attributes", OwnerKind: OwnerPlayer},
	{ID: 3, Code: "backpack", OwnerKind: OwnerPlayer},
}

// accountDefinitions mirrors the ledger-core and item-machine migrations'
// seeded account_definition rows exactly.
var accountDefinitions = []AccountDefinition{
	{ID: 1, ScopeDefinitionID: 1, Code: "money", Kind: KindMoney, Controlled: false},
	{ID: 2, ScopeDefinitionID: 1, Code: "experience", Kind: KindExperience, Controlled: false},
	{ID: 3, ScopeDefinitionID: 2, Code: "money", Kind: KindMoney, Controlled: true},
	{ID: 4, ScopeDefinitionID: 2, Code: "experience", Kind: KindExperience, Controlled: true},
	{ID: 5, ScopeDefinitionID: 1, Code: "slots_free", Kind: KindSlots, Controlled: false, CapacityRole: CapacityFree},
	{ID: 6, ScopeDefinitionID: 1, Code: "slots_used", Kind: KindSlots, Controlled: false, CapacityRole: CapacityUsed},
	{ID: 7, ScopeDefinitionID: 1, Code: "weight_free", Kind: KindWeight, Controlled: false, CapacityRole: CapacityFree},
	{ID: 8, ScopeDefinitionID: 1, Code: "weight_used", Kind: KindWeight, Controlled: false, CapacityRole: CapacityUsed},
	{ID: 9, ScopeDefinitionID: 3, Code: "slots_free", Kind: KindSlots, Controlled: true, CapacityRole: CapacityFree},
	{ID: 10, ScopeDefinitionID: 3, Code: "slots_used", Kind: KindSlots, Controlled: true, CapacityRole: CapacityUsed},
	{ID: 11, ScopeDefinitionID: 3, Code: "weight_free", Kind: KindWeight, Controlled: true, CapacityRole: CapacityFree},
	{ID: 12, ScopeDefinitionID: 3, Code: "weight_used", Kind: KindWeight, Controlled: true, CapacityRole: CapacityUsed},
}

// EventType names one row of the event_type_definition registry: event.type
// references event_type_definition.code by this value. A new type is
// registered by a forward migration inserting a row — never by adding a
// constant here alone — and TestCatalog_mirrors_database's element-for-element
// comparison fails on either side doing so without the other.
type EventType string

// EventType members, in the event-log migration's own declaration order.
const (
	EventBotAddedToChat        EventType = "bot_added_to_chat"
	EventPlayerStarted         EventType = "player_started"
	EventRaidStarted           EventType = "raid_started"
	EventNodeEntered           EventType = "node_entered"
	EventCombatResolved        EventType = "combat_resolved"
	EventRaidFinished          EventType = "raid_finished"
	EventDeath                 EventType = "death"
	EventBackpackDropped       EventType = "backpack_dropped"
	EventBackpackLooted        EventType = "backpack_looted"
	EventBackpackExpired       EventType = "backpack_expired"
	EventStaminaBurnedOverflow EventType = "stamina_burned_overflow"
	EventShopSale              EventType = "shop_sale"
	EventShopPurchase          EventType = "shop_purchase"
	EventNotificationSent      EventType = "notification_sent"
	EventButtonClicked         EventType = "button_clicked"
	EventBotKicked             EventType = "bot_kicked"
)

// EventTypeDefinition mirrors a row of the seeded event_type_definition
// catalog: a registered event type and its volume class. ID exists solely
// to carry the migration's own declaration order so the mirror comparison
// can be ordered rather than set-wise; it is referenced by nothing.
type EventTypeDefinition struct {
	ID          int16
	Code        EventType
	VolumeClass EventVolumeClass
}

// eventTypeDefinitions mirrors the event-log migration's seeded
// event_type_definition rows exactly, in declaration order: notification_sent
// and button_clicked carry VolumeHigh, every other type VolumeLow.
var eventTypeDefinitions = []EventTypeDefinition{
	{ID: 1, Code: EventBotAddedToChat, VolumeClass: VolumeLow},
	{ID: 2, Code: EventPlayerStarted, VolumeClass: VolumeLow},
	{ID: 3, Code: EventRaidStarted, VolumeClass: VolumeLow},
	{ID: 4, Code: EventNodeEntered, VolumeClass: VolumeLow},
	{ID: 5, Code: EventCombatResolved, VolumeClass: VolumeLow},
	{ID: 6, Code: EventRaidFinished, VolumeClass: VolumeLow},
	{ID: 7, Code: EventDeath, VolumeClass: VolumeLow},
	{ID: 8, Code: EventBackpackDropped, VolumeClass: VolumeLow},
	{ID: 9, Code: EventBackpackLooted, VolumeClass: VolumeLow},
	{ID: 10, Code: EventBackpackExpired, VolumeClass: VolumeLow},
	{ID: 11, Code: EventStaminaBurnedOverflow, VolumeClass: VolumeLow},
	{ID: 12, Code: EventShopSale, VolumeClass: VolumeLow},
	{ID: 13, Code: EventShopPurchase, VolumeClass: VolumeLow},
	{ID: 14, Code: EventNotificationSent, VolumeClass: VolumeHigh},
	{ID: 15, Code: EventButtonClicked, VolumeClass: VolumeHigh},
	{ID: 16, Code: EventBotKicked, VolumeClass: VolumeLow},
}
