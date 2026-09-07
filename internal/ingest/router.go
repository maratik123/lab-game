package ingest

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/mymmrac/telego"
)

// Update is one Bot API update handed to a Handler: the raw telego.Update
// in a NAMED field — never embedded, because embedding would promote
// telego.Update's own Context() and WithContext() methods onto this
// type, and the context riding inside a polled update is always a
// context.Background() (design D9, spec *Technical constraints* item 5)
// — plus this package's own derived fields, computed once by the loop so
// a handler never assembles an operation_id from raw Telegram fields
// itself.
type Update struct {
	// Raw is the update exactly as GetUpdates returned it.
	Raw telego.Update
	// Kind is Raw's derived Kind (design D4). The zero Kind means Raw
	// matched no table row.
	Kind Kind
	// OperationID is Raw's canonical idempotency key, in the
	// IDSpaceUpdate space (design D9).
	OperationID string
	// CallbackQueryOperationID is Raw's callback-query idempotency key,
	// in the IDSpaceCallbackQuery space. Empty when Raw carries no
	// callback query — every real operation_id this package builds is
	// non-empty by construction (design D9), so the empty string is a
	// safe "absent" sentinel here.
	CallbackQueryOperationID string
}

// NewUpdate derives every field of an Update from raw: its Kind, its
// canonical operation_id in the update_id space, and — when raw carries a
// callback query — its callback-query operation_id (design D9).
func NewUpdate(raw telego.Update) (Update, error) {
	opID, err := operationID(IDSpaceUpdate, strconv.Itoa(raw.UpdateID))
	if err != nil {
		return Update{}, fmt.Errorf("ingest: build update_id operation_id: %w", err)
	}
	u := Update{
		Raw:         raw,
		Kind:        Derive(&raw),
		OperationID: opID,
	}
	if raw.CallbackQuery != nil {
		cqOpID, err := operationID(IDSpaceCallbackQuery, raw.CallbackQuery.ID)
		if err != nil {
			return Update{}, fmt.Errorf("ingest: build callback_query.id operation_id: %w", err)
		}
		u.CallbackQueryOperationID = cqOpID
	}
	return u, nil
}

// Handler executes one Kind's effects. Implementations are declared by
// the consumer and registered once, at start-up, through Route.
//
// A Handler MUST propagate the ctx it is handed to every call it makes on
// tx (mirrors scheduler.Handler's obligation — design D7's risk row):
// Handle must use the ctx parameter, never Raw's own riding context,
// which is always a context.Background() on this poll path and would
// silently escape both the loop's ctx and the per-attempt transaction
// (design D9).
//
// A Handler MUST NOT issue an outbound Bot API call whose permission
// rests on a row its own uncommitted transaction created (design D18): a
// Telegram send is not rollback-able, so a message justified by a row
// the transaction then rolls back has already reached a real person. The
// designed route for a post-commit send is the outbound notification
// queue (#43); until it lands, perform such a send after the loop has
// committed, outside Handle.
type Handler interface {
	// Handle runs u's effects inside tx, the loop's own transaction for
	// this attempt. Handle must honour ctx's deadline (if any) on every
	// call it makes on tx, and must never read u.Raw's own context.
	Handle(ctx context.Context, tx pgx.Tx, u Update) error
}

// Route pairs one Kind with the Handler that owns it.
type Route struct {
	Kind    Kind
	Handler Handler
}

// Router dispatches an Update to the Handler registered for its Kind.
// Immutable once built by NewRouter — mirrors scheduler.Registry.
type Router struct {
	handlers map[Kind]Handler
	kinds    []Kind
}

// NewRouter builds a Router from routes, refusing a duplicate Kind and a
// Kind with no kindTable row (design D4 — a route can never be
// registered into a hole).
func NewRouter(routes ...Route) (*Router, error) {
	handlers := make(map[Kind]Handler, len(routes))
	kinds := make([]Kind, 0, len(routes))
	for _, route := range routes {
		if !knownKind(route.Kind) {
			return nil, fmt.Errorf("%w: %q", ErrUnknownKind, route.Kind)
		}
		if _, dup := handlers[route.Kind]; dup {
			return nil, fmt.Errorf("%w: %q", ErrDuplicateRoute, route.Kind)
		}
		handlers[route.Kind] = route.Handler
		kinds = append(kinds, route.Kind)
	}
	sort.Slice(kinds, func(i, j int) bool { return kinds[i] < kinds[j] })
	return &Router{handlers: handlers, kinds: kinds}, nil
}

// Kinds returns r's registered kinds, in a deterministic (sorted) order.
func (r *Router) Kinds() []Kind {
	out := make([]Kind, len(r.kinds))
	copy(out, r.kinds)
	return out
}

// Lookup reports the Handler registered for k, and whether one exists.
func (r *Router) Lookup(k Kind) (Handler, bool) {
	h, ok := r.handlers[k]
	return h, ok
}
