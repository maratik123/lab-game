package contract_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/maratik123/lab-game/internal/contract"
	"github.com/maratik123/lab-game/internal/store"
)

// validLeg is a posting leg every refusal case that needs one legal leg
// beside the case under test can reuse.
func validLeg() contract.Leg {
	return contract.PostingLeg("world", "money", store.KindMoney, contract.Positive, contract.Exactly(1))
}

func TestNewRegistry_refuses(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		declarations []contract.Declaration
		wantSubstr   string
	}{
		{
			name: "duplicate type",
			declarations: []contract.Declaration{
				{Type: contract.Event(store.EventShopSale), Signature: contract.Expect(validLeg())},
				{Type: contract.Event(store.EventShopSale), Signature: contract.Expect(
					contract.PostingLeg("attributes", "money", store.KindMoney, contract.Negative, contract.Exactly(1)),
				)},
			},
			wantSubstr: contract.Event(store.EventShopSale).String(),
		},
		{
			name: "the zero Signature",
			declarations: []contract.Declaration{
				{Type: contract.Event(store.EventShopSale), Signature: contract.Signature{}},
			},
			wantSubstr: contract.Event(store.EventShopSale).String(),
		},
		{
			name: "Expect with no legs",
			declarations: []contract.Declaration{
				{Type: contract.Event(store.EventShopSale), Signature: contract.Expect()},
			},
			wantSubstr: contract.Event(store.EventShopSale).String(),
		},
		{
			name: "AnyBalanced on Event",
			declarations: []contract.Declaration{
				{Type: contract.Event(store.EventShopSale), Signature: contract.AnyBalanced()},
			},
			wantSubstr: contract.Event(store.EventShopSale).String(),
		},
		{
			name: "AnyBalanced on DeferredTask",
			declarations: []contract.Declaration{
				{Type: contract.DeferredTask("x"), Signature: contract.AnyBalanced()},
			},
			wantSubstr: contract.DeferredTask("x").String(),
		},
		{
			name: "AnyBalanced on RecurrentTask",
			declarations: []contract.Declaration{
				{Type: contract.RecurrentTask("x"), Signature: contract.AnyBalanced()},
			},
			wantSubstr: contract.RecurrentTask("x").String(),
		},
		{
			name: "event type with an empty code",
			declarations: []contract.Declaration{
				{Type: contract.Event(""), Signature: contract.Expect(validLeg())},
			},
			wantSubstr: "event",
		},
		{
			name: "task type with an empty code",
			declarations: []contract.Declaration{
				{Type: contract.DeferredTask(""), Signature: contract.Expect(validLeg())},
			},
			wantSubstr: "deferred_task",
		},
		{
			name: "the zero DocumentType",
			declarations: []contract.Declaration{
				{Type: contract.DocumentType{}, Signature: contract.Expect(validLeg())},
			},
		},
		{
			name: "Exactly(0)",
			declarations: []contract.Declaration{
				{Type: contract.Event(store.EventShopSale), Signature: contract.Expect(
					contract.PostingLeg("world", "money", store.KindMoney, contract.Positive, contract.Exactly(0)),
				)},
			},
			wantSubstr: contract.Event(store.EventShopSale).String(),
		},
		{
			name: "AtLeast(0)",
			declarations: []contract.Declaration{
				{Type: contract.Event(store.EventShopSale), Signature: contract.Expect(
					contract.PostingLeg("world", "money", store.KindMoney, contract.Positive, contract.AtLeast(0)),
				)},
			},
			wantSubstr: contract.Event(store.EventShopSale).String(),
		},
		{
			name: "an unset sign",
			declarations: []contract.Declaration{
				{Type: contract.Event(store.EventShopSale), Signature: contract.Expect(
					contract.PostingLeg("world", "money", store.KindMoney, 0, contract.Exactly(1)),
				)},
			},
			wantSubstr: contract.Event(store.EventShopSale).String(),
		},
		{
			name: "duplicate posting-leg keys",
			declarations: []contract.Declaration{
				{Type: contract.Event(store.EventShopSale), Signature: contract.Expect(
					contract.PostingLeg("world", "money", store.KindMoney, contract.Positive, contract.Exactly(1)),
					contract.PostingLeg("world", "money", store.KindMoney, contract.Positive, contract.Exactly(2)),
				)},
			},
			wantSubstr: contract.Event(store.EventShopSale).String(),
		},
		{
			name: "duplicate movement-leg keys",
			declarations: []contract.Declaration{
				{Type: contract.DeferredTask("probe_grant"), Signature: contract.Expect(
					contract.MovementLeg("world", "backpack", contract.Exactly(1)),
					contract.MovementLeg("world", "backpack", contract.Exactly(2)),
				)},
			},
			wantSubstr: contract.DeferredTask("probe_grant").String(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := contract.NewRegistry(tc.declarations...)
			if !errors.Is(err, contract.ErrInvalidDeclaration) {
				t.Fatalf("NewRegistry() error = %v, want errors.Is ErrInvalidDeclaration", err)
			}
			if tc.wantSubstr != "" && !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("NewRegistry() error = %q, want it to name %q", err.Error(), tc.wantSubstr)
			}
		})
	}
}

func TestNewRegistry_acceptsEveryForm(t *testing.T) {
	t.Parallel()

	_, err := contract.NewRegistry(
		contract.Declaration{Type: contract.ManualCorrection(), Signature: contract.AnyBalanced()},
		contract.Declaration{Type: contract.Event(store.EventShopSale), Signature: contract.Expect(
			contract.PostingLeg("attributes", "money", store.KindMoney, contract.Negative, contract.Exactly(1)),
			contract.PostingLeg("world", "money", store.KindMoney, contract.Positive, contract.Exactly(1)),
			contract.MovementLeg("world", "backpack", contract.Exactly(1)),
		)},
		contract.Declaration{Type: contract.DeferredTask("probe_grant"), Signature: contract.Expect(
			contract.PostingLeg("attributes", "money", store.KindMoney, contract.Negative, contract.Exactly(1)),
			contract.MovementLeg("world", "backpack", contract.Exactly(1)),
		)},
		contract.Declaration{Type: contract.RecurrentTask("probe_tick"), Signature: contract.Expect(
			contract.PostingLeg("attributes", "money", store.KindMoney, contract.Negative, contract.Exactly(1)),
			contract.MovementLeg("world", "backpack", contract.Exactly(1)),
		)},
	)
	if err != nil {
		t.Fatalf("NewRegistry() unexpected error: %v", err)
	}
}
