package contract

import (
	"errors"
	"strings"
	"testing"

	"github.com/maratik123/lab-game/internal/store"
)

func TestNewRegistry_refusesPlayerOperation(t *testing.T) {
	t.Parallel()

	_, err := NewRegistry(Declaration{Type: playerOperation(), Signature: AnyBalanced()})
	if !errors.Is(err, ErrInvalidDeclaration) {
		t.Fatalf("NewRegistry() error = %v, want errors.Is ErrInvalidDeclaration", err)
	}
	if !strings.Contains(err.Error(), basisPlayerOperation) {
		t.Errorf("NewRegistry() error = %q, want it to name %q", err.Error(), basisPlayerOperation)
	}
}

func TestDocumentType_String(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		typ  DocumentType
		want string
	}{
		{name: "manual correction", typ: ManualCorrection(), want: "manual_correction"},
		{name: "event", typ: Event(store.EventShopSale), want: "event/shop_sale"},
		{name: "deferred task", typ: DeferredTask("probe_grant"), want: "deferred_task/probe_grant"},
		{name: "recurrent task", typ: RecurrentTask("probe_tick"), want: "recurrent_task/probe_tick"},
		{name: "player operation", typ: playerOperation(), want: "player_operation"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := tc.typ.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}
