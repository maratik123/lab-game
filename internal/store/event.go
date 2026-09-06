package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	// sqlstateForeignKeyViolation is PostgreSQL's SQLSTATE for a foreign-key
	// constraint violation (class 23, integrity constraint violation).
	sqlstateForeignKeyViolation = "23503"
	// constraintEventTypeFKey names the foreign key from event.type to
	// event_type_definition.code in migration 00003_event_log.sql;
	// Event.insert maps a violation of exactly this constraint to
	// ErrUnknownEventType.
	constraintEventTypeFKey = "event_type_fkey"
)

// Event is both the basis document for an event-backed posting group
// (docs/DESIGN.md §11, §13.1) and the row AppendEvent writes on the
// no-posting path. Type must name a row of the event_type_definition
// registry. PlayerID, ChatID, MazeID and Depth are the §13.4 dimensions that
// are universal across types — the ones every shipped view may filter or
// group on — and are pointers uniformly, one per nullable column, because a
// zero-as-absent encoding would corrupt Depth: 0 is a real depth, the
// entrance. Payload carries everything else and round-trips as-is; a nil
// Payload is stored as '{}' by the migration's own DEFAULT, via a SQL-side
// COALESCE.
//
// Event carries no occurrence-time field — ts defaults to the database's
// now() — and no idempotency key: unlike PlayerOperation, two Post or
// AppendEvent calls built from equal Event values write two distinct event
// rows and (via Post) two distinct journal entries.
//
// *Event is one of the store package's sealed PostingBasis implementations
// (see PostingBasis's doc comment); every method below is safe to call on a
// nil receiver, matching every other implementation's contract.
type Event struct {
	Type     EventType
	PlayerID *OwnerID
	ChatID   *OwnerID
	MazeID   *int64
	Depth    *int32
	Payload  json.RawMessage
}

func (e *Event) entrySQL() (string, error) {
	if e == nil {
		return "", ErrNoBasis
	}
	return `INSERT INTO journal_entry (event_id) VALUES ($1) RETURNING id`, nil
}

func (e *Event) insert(ctx context.Context, tx pgx.Tx) (int64, error) {
	if e == nil {
		return 0, ErrNoBasis
	}
	var id int64
	err := tx.QueryRow(ctx,
		`INSERT INTO event (type, player_id, chat_id, maze_id, depth, payload)
		 VALUES ($1, $2, $3, $4, $5, COALESCE($6, '{}'::jsonb))
		 RETURNING id`,
		e.Type, e.PlayerID, e.ChatID, e.MazeID, e.Depth, e.Payload,
	).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == sqlstateForeignKeyViolation && pgErr.ConstraintName == constraintEventTypeFKey {
			return 0, fmt.Errorf("%w: %s", ErrUnknownEventType, e.Type)
		}
		return 0, fmt.Errorf("insert event: %w", err)
	}
	return id, nil
}

// AppendEvent writes one event row inside tx and returns its id. Unlike
// Post, it creates no journal_entry and moves no balances — the majority
// shape of event traffic (§13.4: node_entered, notification_sent,
// button_clicked, …). The caller owns tx: AppendEvent neither commits nor
// rolls back. e is taken by value, not by pointer, so the no-posting path
// has no nil case to define: there is no basis for a nil check to be about.
// An unregistered e.Type is refused by the database and surfaced as
// ErrUnknownEventType; the transaction is then aborted and the caller must
// roll back.
func AppendEvent(ctx context.Context, tx pgx.Tx, e Event) (EventID, error) {
	id, err := e.insert(ctx, tx)
	if err != nil {
		return 0, err
	}
	return EventID(id), nil
}
