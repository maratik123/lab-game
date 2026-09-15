package contract

import (
	"sort"
	"testing"

	"github.com/shopspring/decimal"
	"pgregory.net/rapid"

	"github.com/maratik123/lab-game/internal/store"
)

// pk builds a postingKey fixture.
func pk(scope, account string, kind store.Kind, sign Sign) postingKey {
	return postingKey{scopeCode: scope, accountCode: account, kind: kind, sign: sign}
}

// mk builds a movementKey fixture.
func mk(from, to string) movementKey {
	return movementKey{fromScopeCode: from, toScopeCode: to}
}

// posting builds a postingRow fixture.
func posting(scope, account string, kind store.Kind, sign Sign, amount decimal.Decimal) postingRow {
	return postingRow{key: pk(scope, account, kind, sign), amount: amount}
}

// movement builds a movementRow fixture.
func movement(from, to string) movementRow {
	return movementRow{key: mk(from, to)}
}

// renderSorted renders each mismatch and sorts the result, so a
// comparison does not depend on conform's own output order.
func renderSorted(ms []mismatch) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.String()
	}
	sort.Strings(out)
	return out
}

// assertMismatches compares got against want, order-independently.
func assertMismatches(t *testing.T, got, want []mismatch) {
	t.Helper()
	gotStrs := renderSorted(got)
	wantStrs := renderSorted(want)
	if len(gotStrs) != len(wantStrs) {
		t.Fatalf("conform() = %v (%d), want %v (%d)", gotStrs, len(gotStrs), wantStrs, len(wantStrs))
	}
	for i := range wantStrs {
		if gotStrs[i] != wantStrs[i] {
			t.Errorf("conform() = %v, want %v", gotStrs, wantStrs)
			return
		}
	}
}

// shopSaleDebitLeg and shopSaleCreditLeg are made-up shop_sale-shaped
// legs for this package's own tests, borrowing existing catalog codes.
// They are not the game's real shop_sale signature and must not be
// copied into the shipped declared set.
func shopSaleDebitLeg() Leg {
	return PostingLeg("attributes", "money", store.KindMoney, Negative, Exactly(1))
}

func shopSaleCreditLeg() Leg {
	return PostingLeg("world", "money", store.KindMoney, Positive, Exactly(1))
}

func shopSaleSig() Signature {
	return Expect(shopSaleDebitLeg(), shopSaleCreditLeg())
}

func d(n int64) decimal.Decimal {
	return decimal.NewFromInt(n)
}

func TestConform_postingDimensions(t *testing.T) {
	t.Parallel()

	t.Run("control: the unchanged document returns no mismatch", func(t *testing.T) {
		t.Parallel()
		doc := document{postings: []postingRow{
			posting("attributes", "money", store.KindMoney, Negative, d(-100)),
			posting("world", "money", store.KindMoney, Positive, d(100)),
		}}
		assertMismatches(t, conform(shopSaleSig(), doc), nil)
	})

	t.Run("account definition", func(t *testing.T) {
		t.Parallel()
		doc := document{postings: []postingRow{
			posting("attributes", "money", store.KindMoney, Negative, d(-100)),
			posting("attributes", "money", store.KindMoney, Positive, d(100)),
		}}
		want := []mismatch{
			{class: mismatchCardinality, key: pk("world", "money", store.KindMoney, Positive), wantCard: Exactly(1), gotCount: 0},
			{class: mismatchUnexpected, key: pk("attributes", "money", store.KindMoney, Positive), gotCount: 1},
		}
		assertMismatches(t, conform(shopSaleSig(), doc), want)
	})

	t.Run("kind alone", func(t *testing.T) {
		t.Parallel()
		doc := document{postings: []postingRow{
			posting("attributes", "money", store.KindExperience, Negative, d(-100)),
			posting("world", "money", store.KindExperience, Positive, d(100)),
		}}
		want := []mismatch{
			{class: mismatchCardinality, key: pk("attributes", "money", store.KindMoney, Negative), wantCard: Exactly(1), gotCount: 0},
			{class: mismatchCardinality, key: pk("world", "money", store.KindMoney, Positive), wantCard: Exactly(1), gotCount: 0},
			{class: mismatchUnexpected, key: pk("attributes", "money", store.KindExperience, Negative), gotCount: 1},
			{class: mismatchUnexpected, key: pk("world", "money", store.KindExperience, Positive), gotCount: 1},
		}
		assertMismatches(t, conform(shopSaleSig(), doc), want)
	})

	t.Run("sign", func(t *testing.T) {
		t.Parallel()
		doc := document{postings: []postingRow{
			posting("attributes", "money", store.KindMoney, Positive, d(100)),
			posting("world", "money", store.KindMoney, Negative, d(-100)),
		}}
		want := []mismatch{
			{class: mismatchCardinality, key: pk("attributes", "money", store.KindMoney, Negative), wantCard: Exactly(1), gotCount: 0},
			{class: mismatchCardinality, key: pk("world", "money", store.KindMoney, Positive), wantCard: Exactly(1), gotCount: 0},
			{class: mismatchUnexpected, key: pk("attributes", "money", store.KindMoney, Positive), gotCount: 1},
			{class: mismatchUnexpected, key: pk("world", "money", store.KindMoney, Negative), gotCount: 1},
		}
		assertMismatches(t, conform(shopSaleSig(), doc), want)
	})

	t.Run("cardinality: two debit rows against Exactly(1)", func(t *testing.T) {
		t.Parallel()
		doc := document{postings: []postingRow{
			posting("attributes", "money", store.KindMoney, Negative, d(-50)),
			posting("attributes", "money", store.KindMoney, Negative, d(-50)),
			posting("world", "money", store.KindMoney, Positive, d(100)),
		}}
		want := []mismatch{
			{class: mismatchCardinality, key: pk("attributes", "money", store.KindMoney, Negative), wantCard: Exactly(1), gotCount: 2},
		}
		assertMismatches(t, conform(shopSaleSig(), doc), want)
	})

	t.Run("cardinality: no debit row against Exactly(1)", func(t *testing.T) {
		t.Parallel()
		doc := document{postings: []postingRow{
			posting("world", "money", store.KindMoney, Positive, d(100)),
		}}
		want := []mismatch{
			{class: mismatchCardinality, key: pk("attributes", "money", store.KindMoney, Negative), wantCard: Exactly(1), gotCount: 0},
			{class: mismatchUnbalanced, kind: store.KindMoney, gotSum: d(100)},
		}
		assertMismatches(t, conform(shopSaleSig(), doc), want)
	})

	t.Run("cardinality: one debit row against AtLeast(2)", func(t *testing.T) {
		t.Parallel()
		sig := Expect(
			PostingLeg("attributes", "money", store.KindMoney, Negative, AtLeast(2)),
			shopSaleCreditLeg(),
		)
		doc := document{postings: []postingRow{
			posting("attributes", "money", store.KindMoney, Negative, d(-100)),
			posting("world", "money", store.KindMoney, Positive, d(100)),
		}}
		want := []mismatch{
			{class: mismatchCardinality, key: pk("attributes", "money", store.KindMoney, Negative), wantCard: AtLeast(2), gotCount: 1},
		}
		assertMismatches(t, conform(sig, doc), want)
	})
}

func TestConform_movementDimensions(t *testing.T) {
	t.Parallel()

	sig := Expect(shopSaleDebitLeg(), shopSaleCreditLeg(), MovementLeg("world", "backpack", Exactly(1)))
	balanced := []postingRow{
		posting("attributes", "money", store.KindMoney, Negative, d(-100)),
		posting("world", "money", store.KindMoney, Positive, d(100)),
	}

	t.Run("control: the exact document reports nothing", func(t *testing.T) {
		t.Parallel()
		doc := document{
			postings:  balanced,
			movements: []movementRow{movement("world", "backpack")},
		}
		assertMismatches(t, conform(sig, doc), nil)
	})

	t.Run("reversed direction", func(t *testing.T) {
		t.Parallel()
		doc := document{
			postings:  balanced,
			movements: []movementRow{movement("backpack", "world")},
		}
		want := []mismatch{
			{class: mismatchCardinality, key: mk("world", "backpack"), wantCard: Exactly(1), gotCount: 0},
			{class: mismatchUnexpected, key: mk("backpack", "world"), gotCount: 1},
		}
		assertMismatches(t, conform(sig, doc), want)
	})

	t.Run("a second movement against Exactly(1)", func(t *testing.T) {
		t.Parallel()
		doc := document{
			postings:  balanced,
			movements: []movementRow{movement("world", "backpack"), movement("world", "backpack")},
		}
		want := []mismatch{
			{class: mismatchCardinality, key: mk("world", "backpack"), wantCard: Exactly(1), gotCount: 2},
		}
		assertMismatches(t, conform(sig, doc), want)
	})

	t.Run("a movement between an undeclared scope pair", func(t *testing.T) {
		t.Parallel()
		doc := document{
			postings:  balanced,
			movements: []movementRow{movement("world", "attributes")},
		}
		want := []mismatch{
			{class: mismatchCardinality, key: mk("world", "backpack"), wantCard: Exactly(1), gotCount: 0},
			{class: mismatchUnexpected, key: mk("world", "attributes"), gotCount: 1},
		}
		assertMismatches(t, conform(sig, doc), want)
	})
}

func TestConform_undeclaredRows(t *testing.T) {
	t.Parallel()

	doc := document{
		postings: []postingRow{
			posting("attributes", "money", store.KindMoney, Negative, d(-100)),
			posting("world", "money", store.KindMoney, Positive, d(100)),
			posting("attributes", "experience", store.KindExperience, Negative, d(-5)),
			posting("world", "experience", store.KindExperience, Positive, d(5)),
		},
		movements: []movementRow{movement("world", "backpack")},
	}
	want := []mismatch{
		{class: mismatchUnexpected, key: pk("attributes", "experience", store.KindExperience, Negative), gotCount: 1},
		{class: mismatchUnexpected, key: pk("world", "experience", store.KindExperience, Positive), gotCount: 1},
		{class: mismatchUnexpected, key: mk("world", "backpack"), gotCount: 1},
	}
	assertMismatches(t, conform(shopSaleSig(), doc), want)
}

func TestConform_anyBalanced(t *testing.T) {
	t.Parallel()

	t.Run("any balanced set with any movements reports nothing", func(t *testing.T) {
		t.Parallel()
		doc := document{
			postings: []postingRow{
				posting("world", "money", store.KindMoney, Negative, d(-100)),
				posting("attributes", "money", store.KindMoney, Positive, d(100)),
				posting("world", "experience", store.KindExperience, Negative, d(-5)),
				posting("attributes", "experience", store.KindExperience, Positive, d(5)),
			},
			movements: []movementRow{movement("world", "backpack"), movement("world", "backpack")},
		}
		assertMismatches(t, conform(AnyBalanced(), doc), nil)
	})

	t.Run("a set unbalanced in one kind reports unbalanced for that kind and nothing else", func(t *testing.T) {
		t.Parallel()
		doc := document{
			postings: []postingRow{
				posting("world", "money", store.KindMoney, Negative, d(-100)),
				posting("attributes", "money", store.KindMoney, Positive, d(90)),
				posting("world", "experience", store.KindExperience, Negative, d(-5)),
				posting("attributes", "experience", store.KindExperience, Positive, d(5)),
			},
		}
		want := []mismatch{
			{class: mismatchUnbalanced, kind: store.KindMoney, gotSum: d(-10)},
		}
		assertMismatches(t, conform(AnyBalanced(), doc), want)
	})
}

// TestConform_property draws an Expect signature in which every kind has
// a positive and a negative leg, and a document that satisfies it, then
// checks three invariants any permutation and any addition must hold.
func TestConform_property(t *testing.T) {
	t.Parallel()

	kindGen := rapid.SampledFrom([]store.Kind{store.KindMoney, store.KindExperience, store.KindSlots})
	codeGen := rapid.StringMatching(`[a-z][a-z0-9]{0,5}`)

	rapid.Check(t, func(rt *rapid.T) {
		numKinds := rapid.IntRange(1, 3).Draw(rt, "numKinds")
		var legs []Leg
		var rows []postingRow
		usedKinds := map[store.Kind]bool{}
		for i := 0; i < numKinds; i++ {
			kind := kindGen.Draw(rt, "kind")
			if usedKinds[kind] {
				continue
			}
			usedKinds[kind] = true
			posScope := codeGen.Draw(rt, "posScope")
			posAccount := codeGen.Draw(rt, "posAccount")
			negScope := codeGen.Draw(rt, "negScope")
			negAccount := codeGen.Draw(rt, "negAccount")
			amount := int64(rapid.IntRange(1, 1000).Draw(rt, "amount"))

			legs = append(legs,
				PostingLeg(posScope, posAccount, kind, Positive, Exactly(1)),
				PostingLeg(negScope, negAccount, kind, Negative, Exactly(1)),
			)
			rows = append(rows,
				posting(posScope, posAccount, kind, Positive, d(amount)),
				posting(negScope, negAccount, kind, Negative, d(-amount)),
			)
		}
		if len(legs) == 0 {
			return
		}
		sig := Expect(legs...)

		// Any permutation of the rows conforms.
		permuted := rapid.Permutation(rows).Draw(rt, "permutation")
		if got := conform(sig, document{postings: permuted}); len(got) != 0 {
			rt.Fatalf("conform() on a permutation = %v, want no mismatch", got)
		}

		// Removing a row an Exactly leg counts yields a cardinality
		// mismatch on that leg's key.
		removeIdx := rapid.IntRange(0, len(rows)-1).Draw(rt, "removeIdx")
		removed := append([]postingRow(nil), rows[:removeIdx]...)
		removed = append(removed, rows[removeIdx+1:]...)
		gotRemoved := conform(sig, document{postings: removed})
		foundCardinality := false
		for _, m := range gotRemoved {
			if m.class == mismatchCardinality && m.key == rows[removeIdx].key {
				foundCardinality = true
			}
		}
		if !foundCardinality {
			rt.Fatalf("conform() after removing %v = %v, want a cardinality mismatch on its key", rows[removeIdx].key, gotRemoved)
		}

		// Adding an undeclared, cancelling pair in one kind yields an
		// unexpected mismatch on each added key and no unbalanced entry.
		kind := kindGen.Draw(rt, "extraKind")
		extraScope := codeGen.Draw(rt, "extraScope") + "z"
		extraAmount := int64(rapid.IntRange(1, 1000).Draw(rt, "extraAmount"))
		extraPos := posting(extraScope, "extra_pos", kind, Positive, d(extraAmount))
		extraNeg := posting(extraScope, "extra_neg", kind, Negative, d(-extraAmount))
		added := append(append([]postingRow(nil), rows...), extraPos, extraNeg)
		gotAdded := conform(sig, document{postings: added})
		var sawPos, sawNeg, sawUnbalanced bool
		for _, m := range gotAdded {
			switch {
			case m.class == mismatchUnexpected && m.key == extraPos.key:
				sawPos = true
			case m.class == mismatchUnexpected && m.key == extraNeg.key:
				sawNeg = true
			case m.class == mismatchUnbalanced:
				sawUnbalanced = true
			}
		}
		if !sawPos || !sawNeg {
			rt.Fatalf("conform() after adding an undeclared cancelling pair = %v, want unexpected mismatches on %v and %v", gotAdded, extraPos.key, extraNeg.key)
		}
		if sawUnbalanced {
			rt.Fatalf("conform() after adding a cancelling pair = %v, want no unbalanced entry", gotAdded)
		}
	})
}
