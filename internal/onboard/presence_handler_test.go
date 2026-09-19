package onboard

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/mymmrac/telego"

	"github.com/maratik123/lab-game/internal/chat"
	"github.com/maratik123/lab-game/internal/ingest"
	"github.com/maratik123/lab-game/internal/store"
	"github.com/maratik123/lab-game/internal/storetest"
)

func memberStatus(id int64) *telego.ChatMemberMember {
	return &telego.ChatMemberMember{Status: telego.MemberStatusMember, User: telego.User{ID: id}}
}

func leftStatus(id int64) *telego.ChatMemberLeft {
	return &telego.ChatMemberLeft{Status: telego.MemberStatusLeft, User: telego.User{ID: id}}
}

func adminStatus(id int64) *telego.ChatMemberAdministrator {
	return &telego.ChatMemberAdministrator{Status: telego.MemberStatusAdministrator, User: telego.User{ID: id}}
}

func myChatMemberUpdate(updateID int, chatTelegramID int64, chatType, title string, actorID, date int64, oldMember, newMember telego.ChatMember) ingest.Update {
	return ingest.Update{Raw: telego.Update{
		UpdateID: updateID,
		MyChatMember: &telego.ChatMemberUpdated{
			Chat:          telego.Chat{ID: chatTelegramID, Type: chatType, Title: title},
			From:          telego.User{ID: actorID},
			Date:          date,
			OldChatMember: oldMember,
			NewChatMember: newMember,
		},
	}}
}

// recordedEvent is one event row this suite reads back for assertions.
type recordedEvent struct {
	chatID  store.OwnerID
	payload map[string]any
}

func eventsOfType(t *testing.T, ctx context.Context, tx pgx.Tx, typ store.EventType) []recordedEvent {
	t.Helper()
	rows, err := tx.Query(ctx, `SELECT chat_id, payload FROM event WHERE type = $1 ORDER BY id`, typ)
	if err != nil {
		t.Fatalf("query events of type %s: %v", typ, err)
	}
	defer rows.Close()
	var got []recordedEvent
	for rows.Next() {
		var chatID *int64
		var raw json.RawMessage
		if err := rows.Scan(&chatID, &raw); err != nil {
			t.Fatalf("scan event: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		var id store.OwnerID
		if chatID != nil {
			id = store.OwnerID(*chatID)
		}
		got = append(got, recordedEvent{chatID: id, payload: payload})
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return got
}

func ownerScopeDefIDs(t *testing.T, ctx context.Context, tx pgx.Tx, ownerID store.OwnerID) []int16 {
	t.Helper()
	rows, err := tx.Query(ctx, `SELECT scope_definition_id FROM scope WHERE owner_id = $1`, ownerID)
	if err != nil {
		t.Fatalf("query scope: %v", err)
	}
	defer rows.Close()
	var got []int16
	for rows.Next() {
		var id int16
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan scope: %v", err)
		}
		got = append(got, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	return got
}

// TestPresenceHandler_add covers (a): an add for a group chat creates
// the chat owner, its home scope and a present row, and appends
// bot_added_to_chat with the chat dimension set.
func TestPresenceHandler_add(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	h := NewPresenceHandler()
	const chatTelegramID = int64(-1001111111111)
	u := myChatMemberUpdate(1, chatTelegramID, telego.ChatTypeGroup, "Settlement", 500, 1000, leftStatus(999), memberStatus(999))

	if err := h.Handle(ctx, tx, u); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	var ownerID store.OwnerID
	if err := tx.QueryRow(ctx, `SELECT id FROM owner WHERE kind = 'chat' AND telegram_id = $1`, chatTelegramID).Scan(&ownerID); err != nil {
		t.Fatalf("read chat owner: %v", err)
	}
	if scopes := ownerScopeDefIDs(t, ctx, tx, ownerID); len(scopes) != 1 || scopes[0] != 4 {
		t.Fatalf("chat scope_definition ids = %v, want exactly [4] (the home scope)", scopes)
	}
	present, ok, err := chat.Present(ctx, tx, ownerID)
	if err != nil {
		t.Fatalf("Present: %v", err)
	}
	if !ok || !present {
		t.Fatalf("Present = (%v, %v), want (true, true)", present, ok)
	}

	events := eventsOfType(t, ctx, tx, store.EventBotAddedToChat)
	if len(events) != 1 {
		t.Fatalf("bot_added_to_chat events = %d, want 1", len(events))
	}
	if events[0].chatID != ownerID {
		t.Fatalf("event chat_id = %d, want %d", events[0].chatID, ownerID)
	}
}

// TestPresenceHandler_secondChatIsIndependent covers (b): a second chat
// added yields a second chat with a home of its own, and neither is the
// other.
func TestPresenceHandler_secondChatIsIndependent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	h := NewPresenceHandler()
	const chat1, chat2 = int64(-100201), int64(-100202)
	if err := h.Handle(ctx, tx, myChatMemberUpdate(1, chat1, telego.ChatTypeGroup, "A", 500, 1000, leftStatus(999), memberStatus(999))); err != nil {
		t.Fatalf("Handle(chat1): %v", err)
	}
	if err := h.Handle(ctx, tx, myChatMemberUpdate(2, chat2, telego.ChatTypeGroup, "B", 500, 1000, leftStatus(999), memberStatus(999))); err != nil {
		t.Fatalf("Handle(chat2): %v", err)
	}

	var owner1, owner2 store.OwnerID
	if err := tx.QueryRow(ctx, `SELECT id FROM owner WHERE kind = 'chat' AND telegram_id = $1`, chat1).Scan(&owner1); err != nil {
		t.Fatalf("read owner1: %v", err)
	}
	if err := tx.QueryRow(ctx, `SELECT id FROM owner WHERE kind = 'chat' AND telegram_id = $1`, chat2).Scan(&owner2); err != nil {
		t.Fatalf("read owner2: %v", err)
	}
	if owner1 == owner2 {
		t.Fatalf("owner1 = owner2 = %d, want distinct chat owners", owner1)
	}
	for _, id := range []store.OwnerID{owner1, owner2} {
		if scopes := ownerScopeDefIDs(t, ctx, tx, id); len(scopes) != 1 || scopes[0] != 4 {
			t.Fatalf("owner %d scope_definition ids = %v, want exactly [4]", id, scopes)
		}
	}
}

// TestPresenceHandler_noChunkOrGateCreated covers (c): no chunk and no
// gate row exists for either chat afterwards.
func TestPresenceHandler_noChunkOrGateCreated(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	h := NewPresenceHandler()
	if err := h.Handle(ctx, tx, myChatMemberUpdate(1, -100301, telego.ChatTypeGroup, "A", 500, 1000, leftStatus(999), memberStatus(999))); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	var chunkCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM chunk`).Scan(&chunkCount); err != nil {
		t.Fatalf("count chunk: %v", err)
	}
	if chunkCount != 0 {
		t.Fatalf("chunk rows = %d, want 0", chunkCount)
	}
}

// TestPresenceHandler_removalAndReAdd covers (d), (e) and (f): a removal
// flips presence, appends bot_kicked with a correct, joinable payload,
// and leaves the chat, its home and its membership in place; a re-add
// flips presence back against the same chat owner id.
func TestPresenceHandler_removalAndReAdd(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	h := NewPresenceHandler()
	const chatTelegramID = int64(-100401)
	if err := h.Handle(ctx, tx, myChatMemberUpdate(1, chatTelegramID, telego.ChatTypeGroup, "Settlement", 500, 1000, leftStatus(999), memberStatus(999))); err != nil {
		t.Fatalf("Handle(add): %v", err)
	}

	var ownerID store.OwnerID
	if err := tx.QueryRow(ctx, `SELECT id FROM owner WHERE kind = 'chat' AND telegram_id = $1`, chatTelegramID).Scan(&ownerID); err != nil {
		t.Fatalf("read owner: %v", err)
	}

	// Seed a membership so its survival can be checked.
	tgPlayer := int64(600401)
	player, err := store.CreateOwner(ctx, tx, store.OwnerPlayer, &tgPlayer)
	if err != nil {
		t.Fatalf("CreateOwner(player): %v", err)
	}
	if _, err := chat.AddMembership(ctx, tx, ownerID, player.ID); err != nil {
		t.Fatalf("AddMembership: %v", err)
	}

	// Removal.
	if err := h.Handle(ctx, tx, myChatMemberUpdate(2, chatTelegramID, telego.ChatTypeGroup, "Settlement", 501, 2000, memberStatus(999), leftStatus(999))); err != nil {
		t.Fatalf("Handle(remove): %v", err)
	}

	present, ok, err := chat.Present(ctx, tx, ownerID)
	if err != nil {
		t.Fatalf("Present: %v", err)
	}
	if !ok || present {
		t.Fatalf("Present after removal = (%v, %v), want (true, false)", present, ok)
	}

	kicked := eventsOfType(t, ctx, tx, store.EventBotKicked)
	if len(kicked) != 1 {
		t.Fatalf("bot_kicked events = %d, want 1", len(kicked))
	}
	if kicked[0].chatID != ownerID {
		t.Fatalf("bot_kicked chat_id = %d, want %d (joined back to owner, not read from the payload)", kicked[0].chatID, ownerID)
	}
	if got := kicked[0].payload["chat_telegram_id"]; got != float64(chatTelegramID) {
		t.Errorf("bot_kicked payload chat_telegram_id = %v, want %v", got, chatTelegramID)
	}
	if got := kicked[0].payload["actor_telegram_id"]; got != float64(501) {
		t.Errorf("bot_kicked payload actor_telegram_id = %v, want 501", got)
	}
	if got := kicked[0].payload["old_status"]; got != telego.MemberStatusMember {
		t.Errorf("bot_kicked payload old_status = %v, want %q", got, telego.MemberStatusMember)
	}
	if got := kicked[0].payload["new_status"]; got != telego.MemberStatusLeft {
		t.Errorf("bot_kicked payload new_status = %v, want %q", got, telego.MemberStatusLeft)
	}
	if got := kicked[0].payload["update_id"]; got != float64(2) {
		t.Errorf("bot_kicked payload update_id = %v, want 2", got)
	}

	var membershipCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM chat_membership WHERE chat_id = $1 AND player_id = $2`, ownerID, player.ID).Scan(&membershipCount); err != nil {
		t.Fatalf("count chat_membership: %v", err)
	}
	if membershipCount != 1 {
		t.Fatalf("chat_membership rows after removal = %d, want 1 (untouched)", membershipCount)
	}
	if scopes := ownerScopeDefIDs(t, ctx, tx, ownerID); len(scopes) != 1 || scopes[0] != 4 {
		t.Fatalf("chat scope after removal = %v, want unchanged [4]", scopes)
	}

	// Re-add: flips presence back against the SAME chat owner id.
	if err := h.Handle(ctx, tx, myChatMemberUpdate(3, chatTelegramID, telego.ChatTypeGroup, "Settlement", 502, 3000, leftStatus(999), memberStatus(999))); err != nil {
		t.Fatalf("Handle(re-add): %v", err)
	}
	var reAddedOwnerID store.OwnerID
	if err := tx.QueryRow(ctx, `SELECT id FROM owner WHERE kind = 'chat' AND telegram_id = $1`, chatTelegramID).Scan(&reAddedOwnerID); err != nil {
		t.Fatalf("read owner after re-add: %v", err)
	}
	if reAddedOwnerID != ownerID {
		t.Fatalf("owner id changed across re-add: %d -> %d", ownerID, reAddedOwnerID)
	}
	present, ok, err = chat.Present(ctx, tx, ownerID)
	if err != nil {
		t.Fatalf("Present after re-add: %v", err)
	}
	if !ok || !present {
		t.Fatalf("Present after re-add = (%v, %v), want (true, true)", present, ok)
	}
	if got := len(eventsOfType(t, ctx, tx, store.EventBotAddedToChat)); got != 2 {
		t.Fatalf("bot_added_to_chat events = %d, want 2 (the original add and the re-add)", got)
	}
}

// TestPresenceHandler_promotionIsNotAChange covers (g): a promotion from
// member to administrator, on an already-present chat, leaves presence
// true and appends nothing.
func TestPresenceHandler_promotionIsNotAChange(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	h := NewPresenceHandler()
	const chatTelegramID = int64(-100501)
	if err := h.Handle(ctx, tx, myChatMemberUpdate(1, chatTelegramID, telego.ChatTypeGroup, "A", 500, 1000, leftStatus(999), memberStatus(999))); err != nil {
		t.Fatalf("Handle(add): %v", err)
	}
	if err := h.Handle(ctx, tx, myChatMemberUpdate(2, chatTelegramID, telego.ChatTypeGroup, "A", 500, 2000, memberStatus(999), adminStatus(999))); err != nil {
		t.Fatalf("Handle(promotion): %v", err)
	}

	var ownerID store.OwnerID
	if err := tx.QueryRow(ctx, `SELECT id FROM owner WHERE kind = 'chat' AND telegram_id = $1`, chatTelegramID).Scan(&ownerID); err != nil {
		t.Fatalf("read owner: %v", err)
	}
	present, ok, err := chat.Present(ctx, tx, ownerID)
	if err != nil {
		t.Fatalf("Present: %v", err)
	}
	if !ok || !present {
		t.Fatalf("Present after promotion = (%v, %v), want (true, true)", present, ok)
	}
	if got := len(eventsOfType(t, ctx, tx, store.EventBotAddedToChat)); got != 1 {
		t.Fatalf("bot_added_to_chat events = %d, want 1 (the promotion appends nothing)", got)
	}
}

// TestPresenceHandler_promotionAsFirstUpdateEmitsNoAdd covers (g'): a
// promotion as the FIRST update the chat ever produces still writes the
// chat owner, its home and a present row — so the gate works — and
// still appends no bot_added_to_chat, because the prior value read from
// old_chat_member is already "present".
func TestPresenceHandler_promotionAsFirstUpdateEmitsNoAdd(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	h := NewPresenceHandler()
	const chatTelegramID = int64(-100601)
	if err := h.Handle(ctx, tx, myChatMemberUpdate(1, chatTelegramID, telego.ChatTypeGroup, "A", 500, 1000, memberStatus(999), adminStatus(999))); err != nil {
		t.Fatalf("Handle(promotion-first): %v", err)
	}

	var ownerID store.OwnerID
	if err := tx.QueryRow(ctx, `SELECT id FROM owner WHERE kind = 'chat' AND telegram_id = $1`, chatTelegramID).Scan(&ownerID); err != nil {
		t.Fatalf("read owner: %v", err)
	}
	if scopes := ownerScopeDefIDs(t, ctx, tx, ownerID); len(scopes) != 1 || scopes[0] != 4 {
		t.Fatalf("chat scope_definition ids = %v, want exactly [4]", scopes)
	}
	present, ok, err := chat.Present(ctx, tx, ownerID)
	if err != nil {
		t.Fatalf("Present: %v", err)
	}
	if !ok || !present {
		t.Fatalf("Present = (%v, %v), want (true, true)", present, ok)
	}
	if got := len(eventsOfType(t, ctx, tx, store.EventBotAddedToChat)); got != 0 {
		t.Fatalf("bot_added_to_chat events = %d, want 0 (the funnel gap this scenario pins)", got)
	}
}

// TestPresenceHandler_redeliveryAppendsNothing covers (h): the same
// update handled twice appends nothing the second time.
func TestPresenceHandler_redeliveryAppendsNothing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	h := NewPresenceHandler()
	u := myChatMemberUpdate(1, -100701, telego.ChatTypeGroup, "A", 500, 1000, leftStatus(999), memberStatus(999))
	if err := h.Handle(ctx, tx, u); err != nil {
		t.Fatalf("Handle (first): %v", err)
	}
	if err := h.Handle(ctx, tx, u); err != nil {
		t.Fatalf("Handle (redelivery): %v", err)
	}
	if got := len(eventsOfType(t, ctx, tx, store.EventBotAddedToChat)); got != 1 {
		t.Fatalf("bot_added_to_chat events after redelivery = %d, want 1", got)
	}
}

// TestPresenceHandler_privateAndChannelAreNoOps covers (i).
func TestPresenceHandler_privateAndChannelAreNoOps(t *testing.T) {
	t.Parallel()

	for _, chatType := range []string{telego.ChatTypePrivate, telego.ChatTypeChannel} {
		t.Run(chatType, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			pool := storetest.Pool(t)
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatalf("begin: %v", err)
			}
			defer func() { _ = tx.Rollback(ctx) }()

			h := NewPresenceHandler()
			u := myChatMemberUpdate(1, -100801, chatType, "A", 500, 1000, leftStatus(999), memberStatus(999))
			if err := h.Handle(ctx, tx, u); err != nil {
				t.Fatalf("Handle: %v", err)
			}

			var ownerCount int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM owner WHERE kind = 'chat'`).Scan(&ownerCount); err != nil {
				t.Fatalf("count chat owners: %v", err)
			}
			if ownerCount != 0 {
				t.Fatalf("chat owners created for a %s update = %d, want 0", chatType, ownerCount)
			}
		})
	}
}

// TestPresenceHandler_malformedUpdateErrorsWithoutPanic covers (j).
func TestPresenceHandler_malformedUpdateErrorsWithoutPanic(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	h := NewPresenceHandler()
	u := ingest.Update{Raw: telego.Update{UpdateID: 1, MyChatMember: &telego.ChatMemberUpdated{
		Chat: telego.Chat{ID: -1, Type: telego.ChatTypeGroup},
	}}}
	if err := h.Handle(ctx, tx, u); err == nil {
		t.Fatal("Handle(malformed update): want an error, got nil")
	}
}

// TestPresenceHandler_infrastructureErrorPropagates asserts a generic
// database error (here, a closed transaction) is returned rather than
// papered over — the same closed-tx pattern the chat package's own
// suite uses.
func TestPresenceHandler_infrastructureErrorPropagates(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	h := NewPresenceHandler()
	u := myChatMemberUpdate(1, -100901, telego.ChatTypeGroup, "A", 500, 1000, leftStatus(999), memberStatus(999))
	if err := h.Handle(ctx, tx, u); err == nil {
		t.Fatal("Handle on an already-closed transaction: want an error, got nil")
	}
}
