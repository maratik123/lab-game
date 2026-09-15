// Package contract declares the posting signature a balance-moving
// mechanic's basis document is expected to produce, and checks a real
// transaction's postings and item movements against it. It writes
// nothing; only a test imports it.
package contract

import (
	"fmt"

	"github.com/maratik123/lab-game/internal/store"
)

// The basis names a DocumentType may carry.
const (
	basisManualCorrection = "manual_correction"
	basisEvent            = "event"
	basisDeferredTask     = "deferred_task"
	basisRecurrentTask    = "recurrent_task"
	basisPlayerOperation  = "player_operation"
)

// DocumentType identifies the basis document a journal entry references,
// together with the entry's type within that basis's table when the
// basis carries one. The zero DocumentType is not a valid type: build one
// with ManualCorrection, Event, DeferredTask or RecurrentTask. A player
// operation has no exported constructor: it carries no type column of its
// own, so no registry may hold a declaration under that basis.
type DocumentType struct {
	basis string
	code  string
}

// ManualCorrection returns the DocumentType for a manual correction, a
// whole-table basis with no type of its own.
func ManualCorrection() DocumentType {
	return DocumentType{basis: basisManualCorrection}
}

// Event returns the DocumentType for an event of the given event.type.
func Event(t store.EventType) DocumentType {
	return DocumentType{basis: basisEvent, code: string(t)}
}

// DeferredTask returns the DocumentType for a deferred task of the given
// deferred_task.task_type code.
func DeferredTask(code string) DocumentType {
	return DocumentType{basis: basisDeferredTask, code: code}
}

// RecurrentTask returns the DocumentType for a recurrent task of the given
// recurrent_task.task_type code.
func RecurrentTask(code string) DocumentType {
	return DocumentType{basis: basisRecurrentTask, code: code}
}

// playerOperation returns the DocumentType a player operation's journal
// entry carries. There is no exported constructor, and NewRegistry
// refuses a declaration under this basis: the player operation has no
// type column of its own, so no mechanic may declare a whole-table
// signature for it.
func playerOperation() DocumentType {
	return DocumentType{basis: basisPlayerOperation}
}

// known reports whether d is a type NewRegistry may accept a declaration
// for. A basis that carries no type (manual_correction, player_operation)
// must have no code; a basis that carries one (event, deferred_task,
// recurrent_task) must have a non-empty code. The zero DocumentType and
// any basis this package does not recognise are refused.
func (d DocumentType) known() bool {
	switch d.basis {
	case basisManualCorrection, basisPlayerOperation:
		return d.code == ""
	case basisEvent, basisDeferredTask, basisRecurrentTask:
		return d.code != ""
	default:
		return false
	}
}

// String renders d as "event/shop_sale", "deferred_task/<code>",
// "manual_correction" or "player_operation". The zero DocumentType, and
// any basis this package does not recognise, render as "unknown_basis".
func (d DocumentType) String() string {
	if d.basis == "" {
		return "unknown_basis"
	}
	if d.code == "" {
		return d.basis
	}
	return d.basis + "/" + d.code
}

// Sign is the sign of a posting leg's declared amount. The zero Sign is
// not valid — use Positive or Negative.
type Sign int

// Sign members.
const (
	Positive Sign = iota + 1
	Negative
)

// String renders sg as "positive", "negative" or "unset".
func (sg Sign) String() string {
	switch sg {
	case Positive:
		return "positive"
	case Negative:
		return "negative"
	default:
		return "unset"
	}
}

// cardinalityKind distinguishes an exact count from a lower-bounded one.
type cardinalityKind int

const (
	cardinalityExactly cardinalityKind = iota
	cardinalityAtLeast
)

// Cardinality states how many actual rows a declared Leg's key must match:
// Exactly(n) or AtLeast(n), n >= 1. A leg that may be absent is simply not
// declared.
type Cardinality struct {
	kind cardinalityKind
	n    int
}

// Exactly returns the Cardinality met only by exactly n actual rows.
func Exactly(n int) Cardinality {
	return Cardinality{kind: cardinalityExactly, n: n}
}

// AtLeast returns the Cardinality met by n or more actual rows.
func AtLeast(n int) Cardinality {
	return Cardinality{kind: cardinalityAtLeast, n: n}
}

// valid reports whether c's count is at least 1.
func (c Cardinality) valid() bool {
	return c.n >= 1
}

// String renders c as "exactly(n)" or "at least(n)".
func (c Cardinality) String() string {
	switch c.kind {
	case cardinalityExactly:
		return fmt.Sprintf("exactly(%d)", c.n)
	case cardinalityAtLeast:
		return fmt.Sprintf("at least(%d)", c.n)
	default:
		return fmt.Sprintf("unknown cardinality(%d)", c.n)
	}
}

// legClass distinguishes an expected posting from an expected item
// movement within a Leg.
type legClass int

const (
	legClassPosting legClass = iota
	legClassMovement
)

// postingKey is the comparable identity a posting leg declares, and the
// identity an actual posting row is read into: the scope definition code
// and account definition code its account belongs to, the account's kind,
// and the sign of its amount.
type postingKey struct {
	scopeCode, accountCode string
	kind                   store.Kind
	sign                   Sign
}

// String renders k as "<scope>/<account> <kind> <sign>".
func (k postingKey) String() string {
	return fmt.Sprintf("%s/%s %s %s", k.scopeCode, k.accountCode, k.kind, k.sign)
}

// movementKey is the comparable identity a movement leg declares, and the
// identity an actual item movement row is read into: the scope
// definition codes of its two holder scopes.
type movementKey struct {
	fromScopeCode, toScopeCode string
}

// String renders k as "<from> -> <to>".
func (k movementKey) String() string {
	return fmt.Sprintf("%s -> %s", k.fromScopeCode, k.toScopeCode)
}

// Leg is one declared line of a Signature — an expected posting or an
// expected item movement — built by PostingLeg or MovementLeg.
type Leg struct {
	class       legClass
	scopeCode   string // posting: the account's scope definition code; movement: the from-scope code
	toScopeCode string // movement only: the to-scope code
	accountCode string // posting only: the account definition code
	kind        store.Kind
	sign        Sign
	cardinality Cardinality
}

// PostingLeg declares an expected posting: an account identified by its
// scope definition code and account definition code, the account's kind,
// the sign of the posted amount, and how many such postings a conforming
// document carries.
func PostingLeg(scopeCode, accountCode string, kind store.Kind, sign Sign, cardinality Cardinality) Leg {
	return Leg{
		class:       legClassPosting,
		scopeCode:   scopeCode,
		accountCode: accountCode,
		kind:        kind,
		sign:        sign,
		cardinality: cardinality,
	}
}

// MovementLeg declares an expected item movement: the scope definition
// codes of its from-holder and to-holder scopes, and how many such
// movements a conforming document carries.
func MovementLeg(fromScopeCode, toScopeCode string, cardinality Cardinality) Leg {
	return Leg{
		class:       legClassMovement,
		scopeCode:   fromScopeCode,
		toScopeCode: toScopeCode,
		cardinality: cardinality,
	}
}

// key returns l's comparable identity: a postingKey or a movementKey,
// usable as a map key to test two legs for the same declared row.
func (l Leg) key() any {
	switch l.class {
	case legClassPosting:
		return postingKey{scopeCode: l.scopeCode, accountCode: l.accountCode, kind: l.kind, sign: l.sign}
	case legClassMovement:
		return movementKey{fromScopeCode: l.scopeCode, toScopeCode: l.toScopeCode}
	default:
		return nil
	}
}

// validate reports whether l is a legal declaration: a valid cardinality,
// and, per class, non-empty codes, a non-empty kind and a valid sign for
// a posting leg, or two non-empty scope codes for a movement leg.
func (l Leg) validate() error {
	if !l.cardinality.valid() {
		return fmt.Errorf("cardinality %s: n must be at least 1", l.cardinality)
	}
	switch l.class {
	case legClassPosting:
		if l.scopeCode == "" || l.accountCode == "" {
			return fmt.Errorf("posting leg %s: scope code and account code must be set", l.key())
		}
		if l.kind == "" {
			return fmt.Errorf("posting leg %s: kind must be set", l.key())
		}
		if l.sign != Positive && l.sign != Negative {
			return fmt.Errorf("posting leg %s: sign must be Positive or Negative", l.key())
		}
		return nil
	case legClassMovement:
		if l.scopeCode == "" || l.toScopeCode == "" {
			return fmt.Errorf("movement leg %s: from-scope code and to-scope code must be set", l.key())
		}
		return nil
	default:
		return fmt.Errorf("leg: unknown class %d", l.class)
	}
}

// signatureForm distinguishes the two Signature shapes NewRegistry
// accepts from the zero Signature, which it refuses.
type signatureForm int

const (
	signatureZero signatureForm = iota
	signatureAnyBalanced
	signatureExpect
)

// Signature is a basis document's expected shape: AnyBalanced or Expect.
// The zero Signature is neither, and NewRegistry refuses it.
type Signature struct {
	form signatureForm
	legs []Leg
}

// AnyBalanced returns the Signature admitting any postings that sum to
// zero per kind, and any movements. NewRegistry accepts it only for
// ManualCorrection.
func AnyBalanced() Signature {
	return Signature{form: signatureAnyBalanced}
}

// Expect returns the Signature admitting exactly the given legs.
// NewRegistry refuses it with no legs.
func Expect(legs ...Leg) Signature {
	cp := make([]Leg, len(legs))
	copy(cp, legs)
	return Signature{form: signatureExpect, legs: cp}
}
