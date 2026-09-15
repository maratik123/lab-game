package contract

import (
	"fmt"
	"sort"

	"github.com/shopspring/decimal"

	"github.com/maratik123/lab-game/internal/store"
)

// satisfiedBy reports whether count actual rows meet c.
func (c Cardinality) satisfiedBy(count int) bool {
	switch c.kind {
	case cardinalityExactly:
		return count == c.n
	case cardinalityAtLeast:
		return count >= c.n
	default:
		return false
	}
}

// postingRow is one actual posting conform judges: the key read along
// posting -> account -> account definition -> scope definition, and the
// posted amount, signed, kept only for the zero-sum-per-kind check.
type postingRow struct {
	key    postingKey
	amount decimal.Decimal
}

// movementRow is one actual item movement conform judges: the key read
// from its two holder scopes.
type movementRow struct {
	key movementKey
}

// document is the actual shape (*Registry).Check reads for one journal
// entry past the mark: every posting and item movement it produced.
type document struct {
	postings  []postingRow
	movements []movementRow
}

// mismatchClass names the three ways a document can fail to conform.
type mismatchClass int

const (
	// mismatchCardinality: a declared leg's key, whose actual count does
	// not satisfy the declared Cardinality.
	mismatchCardinality mismatchClass = iota
	// mismatchUnexpected: an actual row's key that no leg declares.
	mismatchUnexpected
	// mismatchUnbalanced: a kind whose postings do not sum to zero.
	mismatchUnbalanced
)

// mismatch is one way conform found doc not to conform to sig: a class
// and the key or kind it concerns.
type mismatch struct {
	class    mismatchClass
	key      any // postingKey or movementKey; unset for mismatchUnbalanced
	kind     store.Kind
	wantCard Cardinality
	gotCount int
	gotSum   decimal.Decimal
}

// String renders m for a diagnostic message and for test comparison.
func (m mismatch) String() string {
	switch m.class {
	case mismatchCardinality:
		return fmt.Sprintf("cardinality: %v wants %s, found %d", m.key, m.wantCard, m.gotCount)
	case mismatchUnexpected:
		return fmt.Sprintf("unexpected: %v found %d", m.key, m.gotCount)
	case mismatchUnbalanced:
		return fmt.Sprintf("unbalanced: kind %s sums to %s", m.kind, m.gotSum)
	default:
		return fmt.Sprintf("unknown mismatch class %d", m.class)
	}
}

// sortKey orders mismatches by class, then by their rendered identity, so
// conform's result is deterministic and testable.
func (m mismatch) sortKey() string {
	return fmt.Sprintf("%d|%s", m.class, m)
}

// conform compares doc's actual postings and item movements against sig,
// and returns every mismatch, sorted by class and key. A nil result means
// doc conforms.
//
// For AnyBalanced, only the zero-sum-per-kind check applies: any
// postings that balance and any movements conform. For Expect, every
// declared leg's key must be met by its Cardinality (mismatchCardinality
// on a miss) and every actual row's key must be declared by some leg
// (mismatchUnexpected on a miss), in addition to the zero-sum check.
func conform(sig Signature, doc document) []mismatch {
	var mismatches []mismatch

	actualPostings := make(map[postingKey]int, len(doc.postings))
	for _, p := range doc.postings {
		actualPostings[p.key]++
	}
	actualMovements := make(map[movementKey]int, len(doc.movements))
	for _, mv := range doc.movements {
		actualMovements[mv.key]++
	}

	if sig.form == signatureExpect {
		declaredPostings := make(map[postingKey]bool, len(sig.legs))
		declaredMovements := make(map[movementKey]bool, len(sig.legs))
		for _, leg := range sig.legs {
			switch leg.class {
			case legClassPosting:
				key, ok := leg.key().(postingKey)
				if !ok {
					continue
				}
				declaredPostings[key] = true
				count := actualPostings[key]
				if !leg.cardinality.satisfiedBy(count) {
					mismatches = append(mismatches, mismatch{
						class: mismatchCardinality, key: key,
						wantCard: leg.cardinality, gotCount: count,
					})
				}
			case legClassMovement:
				key, ok := leg.key().(movementKey)
				if !ok {
					continue
				}
				declaredMovements[key] = true
				count := actualMovements[key]
				if !leg.cardinality.satisfiedBy(count) {
					mismatches = append(mismatches, mismatch{
						class: mismatchCardinality, key: key,
						wantCard: leg.cardinality, gotCount: count,
					})
				}
			}
		}
		for key, count := range actualPostings {
			if !declaredPostings[key] {
				mismatches = append(mismatches, mismatch{class: mismatchUnexpected, key: key, gotCount: count})
			}
		}
		for key, count := range actualMovements {
			if !declaredMovements[key] {
				mismatches = append(mismatches, mismatch{class: mismatchUnexpected, key: key, gotCount: count})
			}
		}
	}

	// Every document, of either form, is checked for a zero sum per
	// kind — the same predicate the write path enforces.
	sums := make(map[store.Kind]decimal.Decimal)
	for _, p := range doc.postings {
		sums[p.key.kind] = sums[p.key.kind].Add(p.amount)
	}
	for kind, sum := range sums {
		if !sum.IsZero() {
			mismatches = append(mismatches, mismatch{class: mismatchUnbalanced, kind: kind, gotSum: sum})
		}
	}

	sort.Slice(mismatches, func(i, j int) bool {
		return mismatches[i].sortKey() < mismatches[j].sortKey()
	})
	return mismatches
}
