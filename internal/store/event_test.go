package store

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/shopspring/decimal"
)

func TestEvent_post_writesEventJournalEntryAndPostings(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	money, _ := createPlayer(t, ctx, tx)
	fund(t, ctx, tx, money, KindMoney, decimal.NewFromInt(100))

	if err := Post(ctx, tx, &Event{Type: EventShopSale},
		Posting{AccountID: money, Amount: decimal.NewFromInt(-10)},
		Posting{AccountID: WorldMoney, Amount: decimal.NewFromInt(10)},
	); err != nil {
		t.Fatalf("post under *Event: %v", err)
	}

	var eventCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM event WHERE type = $1`, EventShopSale).Scan(&eventCount); err != nil {
		t.Fatalf("count event: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("event rows = %d, want 1", eventCount)
	}

	var poID, mcID, dtID, rtID *int64
	var evID int64
	if err := tx.QueryRow(ctx,
		`SELECT player_operation_id, manual_correction_id, deferred_task_id, recurrent_task_id, event_id
		 FROM journal_entry WHERE event_id IS NOT NULL`,
	).Scan(&poID, &mcID, &dtID, &rtID, &evID); err != nil {
		t.Fatalf("select journal_entry: %v", err)
	}
	if poID != nil || mcID != nil || dtID != nil || rtID != nil {
		t.Fatalf("journal_entry other arc columns = %v/%v/%v/%v, want all nil", poID, mcID, dtID, rtID)
	}

	var postingCount int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM posting p JOIN journal_entry je ON je.id = p.journal_entry_id WHERE je.event_id IS NOT NULL`,
	).Scan(&postingCount); err != nil {
		t.Fatalf("count postings: %v", err)
	}
	if postingCount != 2 {
		t.Fatalf("postings under the event journal_entry = %d, want 2", postingCount)
	}
}

func TestEvent_typedNilThroughPost_returnsErrNoBasisWithoutStatement(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool, rec := newStoreWithRecorder(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	var nilEv *Event
	rec.Reset()
	err = Post(ctx, tx, nilEv, Posting{AccountID: WorldMoney, Amount: decimal.NewFromInt(1)})
	if !errors.Is(err, ErrNoBasis) {
		t.Fatalf("Post with typed-nil *Event = %v, want ErrNoBasis", err)
	}
	if stmts := rec.Statements(); len(stmts) != 0 {
		t.Fatalf("statements issued after typed-nil basis = %v, want none", stmts)
	}
}

func TestAppendEvent_writesEventRow_noJournalEntry(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	id, err := AppendEvent(ctx, tx, Event{Type: EventNodeEntered})
	if err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}
	if id == 0 {
		t.Fatalf("expected a non-zero event id")
	}

	var journalCount, postingCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM journal_entry WHERE event_id = $1`, id).Scan(&journalCount); err != nil {
		t.Fatalf("count journal_entry: %v", err)
	}
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM posting p JOIN journal_entry je ON je.id = p.journal_entry_id WHERE je.event_id = $1`, id,
	).Scan(&postingCount); err != nil {
		t.Fatalf("count posting: %v", err)
	}
	if journalCount != 0 || postingCount != 0 {
		t.Fatalf("journal/posting referencing the appended event = %d/%d, want 0/0", journalCount, postingCount)
	}
}

func TestEvent_unknownType_returnsErrUnknownEventType(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)

	t.Run("Post", func(t *testing.T) {
		t.Parallel()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		money, _ := createPlayer(t, ctx, tx)
		err = Post(ctx, tx, &Event{Type: EventType("nope")},
			Posting{AccountID: money, Amount: decimal.NewFromInt(-1)},
			Posting{AccountID: WorldMoney, Amount: decimal.NewFromInt(1)},
		)
		if !errors.Is(err, ErrUnknownEventType) {
			t.Fatalf("Post with unregistered type = %v, want ErrUnknownEventType", err)
		}
	})

	t.Run("AppendEvent", func(t *testing.T) {
		t.Parallel()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		_, err = AppendEvent(ctx, tx, Event{Type: EventType("nope")})
		if !errors.Is(err, ErrUnknownEventType) {
			t.Fatalf("AppendEvent with unregistered type = %v, want ErrUnknownEventType", err)
		}
	})
}

// TestErrUnknownEventType_distinctFromOtherSentinels asserts AC8's
// distinguishability as a loop over the sentinel set, so a future sentinel
// added by aliasing (rather than errors.New) fails this test.
func TestErrUnknownEventType_distinctFromOtherSentinels(t *testing.T) {
	t.Parallel()

	others := []error{
		ErrNoBasis, ErrEmptyBatch, ErrInvalidAmount, ErrUnknownAccount,
		ErrUnbalanced, ErrAlreadyPosted, ErrOverdraft, ErrBalanceRowMissing,
		ErrInvalidOwner,
	}
	for _, other := range others {
		if errors.Is(ErrUnknownEventType, other) {
			t.Fatalf("ErrUnknownEventType must be distinct from %v", other)
		}
	}
}

// TestEvent_noIdempotencyKey_replayWritesTwoRows asserts the § Approach
// consequence directly, rather than leaving it as prose: unlike
// PlayerOperation, Event carries no (source, operation_id)-style key, so a
// second Post built from an equal Event value is not deduplicated.
func TestEvent_noIdempotencyKey_replayWritesTwoRows(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer rollback(t, ctx, tx)

	money, _ := createPlayer(t, ctx, tx)
	fund(t, ctx, tx, money, KindMoney, decimal.NewFromInt(100))

	ev := Event{Type: EventNodeEntered}
	for i := 0; i < 2; i++ {
		if err := Post(ctx, tx, &ev,
			Posting{AccountID: money, Amount: decimal.NewFromInt(-1)},
			Posting{AccountID: WorldMoney, Amount: decimal.NewFromInt(1)},
		); err != nil {
			t.Fatalf("post %d: %v", i, err)
		}
	}

	var eventCount, journalCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM event WHERE type = $1`, EventNodeEntered).Scan(&eventCount); err != nil {
		t.Fatalf("count event: %v", err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM journal_entry WHERE event_id IS NOT NULL`).Scan(&journalCount); err != nil {
		t.Fatalf("count journal_entry: %v", err)
	}
	if eventCount != 2 || journalCount != 2 {
		t.Fatalf("event/journal_entry rows after replay = %d/%d, want 2/2", eventCount, journalCount)
	}
}

func TestEvent_transactionOwnership(t *testing.T) {
	t.Parallel()

	t.Run("tx_stays_open_after_post", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		pool := newStore(t)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		money, _ := createPlayer(t, ctx, tx)
		fund(t, ctx, tx, money, KindMoney, decimal.NewFromInt(10))
		if err := Post(ctx, tx, &Event{Type: EventNodeEntered},
			Posting{AccountID: money, Amount: decimal.NewFromInt(-1)},
			Posting{AccountID: WorldMoney, Amount: decimal.NewFromInt(1)},
		); err != nil {
			t.Fatalf("post: %v", err)
		}
		var one int
		if err := tx.QueryRow(ctx, `SELECT 1`).Scan(&one); err != nil {
			t.Fatalf("transaction unusable after Post: %v", err)
		}
	})

	t.Run("tx_stays_open_after_append", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		pool := newStore(t)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		if _, err := AppendEvent(ctx, tx, Event{Type: EventNodeEntered}); err != nil {
			t.Fatalf("AppendEvent: %v", err)
		}
		var one int
		if err := tx.QueryRow(ctx, `SELECT 1`).Scan(&one); err != nil {
			t.Fatalf("transaction unusable after AppendEvent: %v", err)
		}
	})

	t.Run("rollback_leaves_no_event_row", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		pool := newStore(t)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		id, err := AppendEvent(ctx, tx, Event{Type: EventBotKicked})
		if err != nil {
			t.Fatalf("AppendEvent: %v", err)
		}
		rollback(t, ctx, tx)

		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM event WHERE id = $1`, id).Scan(&count); err != nil {
			t.Fatalf("count event after rollback: %v", err)
		}
		if count != 0 {
			t.Fatalf("event rows after rollback = %d, want 0", count)
		}
	})
}

func TestEvent_payloadRoundtrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)

	t.Run("nil_becomes_empty_object", func(t *testing.T) {
		t.Parallel()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		id, err := AppendEvent(ctx, tx, Event{Type: EventNodeEntered})
		if err != nil {
			t.Fatalf("AppendEvent: %v", err)
		}
		var raw []byte
		if err := tx.QueryRow(ctx, `SELECT payload FROM event WHERE id = $1`, id).Scan(&raw); err != nil {
			t.Fatalf("select payload: %v", err)
		}
		if string(raw) != "{}" {
			t.Fatalf("payload = %s, want {}", raw)
		}
	})

	t.Run("nonNil_roundtrips_equal_as_parsed_json", func(t *testing.T) {
		t.Parallel()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		payload := json.RawMessage(`{"b":2,"a":1}`)
		id, err := AppendEvent(ctx, tx, Event{Type: EventNodeEntered, Payload: payload})
		if err != nil {
			t.Fatalf("AppendEvent: %v", err)
		}
		var raw []byte
		if err := tx.QueryRow(ctx, `SELECT payload FROM event WHERE id = $1`, id).Scan(&raw); err != nil {
			t.Fatalf("select payload: %v", err)
		}
		var got, want map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("unmarshal got: %v", err)
		}
		if err := json.Unmarshal(payload, &want); err != nil {
			t.Fatalf("unmarshal want: %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("payload roundtrip = %v, want %v", got, want)
		}
	})
}

func TestEvent_nullableFields_roundtrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := newStore(t)

	t.Run("all_absent", func(t *testing.T) {
		t.Parallel()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		id, err := AppendEvent(ctx, tx, Event{Type: EventNodeEntered})
		if err != nil {
			t.Fatalf("AppendEvent: %v", err)
		}
		var playerID, chatID, mazeID *int64
		var depth *int32
		if err := tx.QueryRow(ctx,
			`SELECT player_id, chat_id, maze_id, depth FROM event WHERE id = $1`, id,
		).Scan(&playerID, &chatID, &mazeID, &depth); err != nil {
			t.Fatalf("select nullable fields: %v", err)
		}
		if playerID != nil || chatID != nil || mazeID != nil || depth != nil {
			t.Fatalf("nullable fields = %v/%v/%v/%v, want all nil", playerID, chatID, mazeID, depth)
		}
	})

	t.Run("all_present_including_zero_depth", func(t *testing.T) {
		t.Parallel()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer rollback(t, ctx, tx)

		playerOwner, err := CreateOwner(ctx, tx, OwnerPlayer, ptrInt64(nextTelegramID.Add(1)))
		if err != nil {
			t.Fatalf("create player owner: %v", err)
		}
		chatOwner, err := CreateOwner(ctx, tx, OwnerChat, ptrInt64(nextTelegramID.Add(1)))
		if err != nil {
			t.Fatalf("create chat owner: %v", err)
		}
		mazeID := int64(5)
		depth := int32(0) // depth 0 is the entrance, a real depth, not "absent"

		id, err := AppendEvent(ctx, tx, Event{
			Type:     EventNodeEntered,
			PlayerID: &playerOwner.ID,
			ChatID:   &chatOwner.ID,
			MazeID:   &mazeID,
			Depth:    &depth,
		})
		if err != nil {
			t.Fatalf("AppendEvent: %v", err)
		}

		var gotPlayer, gotChat, gotMaze *int64
		var gotDepth *int32
		if err := tx.QueryRow(ctx,
			`SELECT player_id, chat_id, maze_id, depth FROM event WHERE id = $1`, id,
		).Scan(&gotPlayer, &gotChat, &gotMaze, &gotDepth); err != nil {
			t.Fatalf("select nullable fields: %v", err)
		}
		if gotPlayer == nil || OwnerID(*gotPlayer) != playerOwner.ID {
			t.Fatalf("player_id = %v, want %d", gotPlayer, playerOwner.ID)
		}
		if gotChat == nil || OwnerID(*gotChat) != chatOwner.ID {
			t.Fatalf("chat_id = %v, want %d", gotChat, chatOwner.ID)
		}
		if gotMaze == nil || *gotMaze != mazeID {
			t.Fatalf("maze_id = %v, want %d", gotMaze, mazeID)
		}
		if gotDepth == nil || *gotDepth != 0 {
			t.Fatalf("depth = %v, want 0 (not NULL)", gotDepth)
		}
	})
}

func ptrInt64(v int64) *int64 { return &v }
