package onboard

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/mymmrac/telego"

	"github.com/maratik123/lab-game/internal/chat"
	"github.com/maratik123/lab-game/internal/ingest"
	"github.com/maratik123/lab-game/internal/store"
	"github.com/maratik123/lab-game/internal/storetest"
)

func startMessageUpdate(updateID int, chatType string, chatTelegramID, fromID int64, text string) ingest.Update {
	return ingest.Update{Raw: telego.Update{
		UpdateID: updateID,
		Message: &telego.Message{
			MessageID: updateID,
			Chat:      telego.Chat{ID: chatTelegramID, Type: chatType},
			From:      &telego.User{ID: fromID},
			Text:      text,
			Date:      1000,
		},
	}}
}

func membershipExists(t *testing.T, ctx context.Context, tx pgx.Tx, chatOwnerID, playerOwnerID store.OwnerID) bool {
	t.Helper()
	var count int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM chat_membership WHERE chat_id = $1 AND player_id = $2`, chatOwnerID, playerOwnerID,
	).Scan(&count); err != nil {
		t.Fatalf("count chat_membership: %v", err)
	}
	return count > 0
}

// TestStartHandler_withChatPayload covers (a): a private /start with a
// chat payload creates the player, records the membership and appends
// player_started with both dimensions.
func TestStartHandler_withChatPayload(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const chatTelegramID = int64(-200101)
	chatOwner, err := store.CreateOwner(ctx, tx, store.OwnerChat, ptr(chatTelegramID))
	if err != nil {
		t.Fatalf("CreateOwner(chat): %v", err)
	}

	h := NewStartHandler()
	const playerTelegramID = int64(300101)
	u := startMessageUpdate(1, telego.ChatTypePrivate, playerTelegramID, playerTelegramID, "/start "+EncodeChatStart(chatTelegramID))
	if err := h.Handle(ctx, tx, u); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	var playerOwnerID store.OwnerID
	if err := tx.QueryRow(ctx, `SELECT id FROM owner WHERE kind = 'player' AND telegram_id = $1`, playerTelegramID).Scan(&playerOwnerID); err != nil {
		t.Fatalf("read player owner: %v", err)
	}
	if !membershipExists(t, ctx, tx, chatOwner.ID, playerOwnerID) {
		t.Fatal("membership not recorded")
	}

	events := eventsOfType(t, ctx, tx, store.EventPlayerStarted)
	if len(events) != 1 {
		t.Fatalf("player_started events = %d, want 1", len(events))
	}
	if events[0].chatID != chatOwner.ID {
		t.Fatalf("event chat_id = %d, want %d", events[0].chatID, chatOwner.ID)
	}
}

// TestStartHandler_secondChatMembershipKeepsFirst covers (b): an
// existing player starting through a second chat's link is a member of
// both, the first membership is untouched, and the second chat's own
// player_started is appended.
func TestStartHandler_secondChatMembershipKeepsFirst(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const chat1TelegramID, chat2TelegramID = int64(-200201), int64(-200202)
	chat1, err := store.CreateOwner(ctx, tx, store.OwnerChat, ptr(chat1TelegramID))
	if err != nil {
		t.Fatalf("CreateOwner(chat1): %v", err)
	}
	chat2, err := store.CreateOwner(ctx, tx, store.OwnerChat, ptr(chat2TelegramID))
	if err != nil {
		t.Fatalf("CreateOwner(chat2): %v", err)
	}

	h := NewStartHandler()
	const playerTelegramID = int64(300201)
	if err := h.Handle(ctx, tx, startMessageUpdate(1, telego.ChatTypePrivate, playerTelegramID, playerTelegramID, "/start "+EncodeChatStart(chat1TelegramID))); err != nil {
		t.Fatalf("Handle(chat1): %v", err)
	}
	if err := h.Handle(ctx, tx, startMessageUpdate(2, telego.ChatTypePrivate, playerTelegramID, playerTelegramID, "/start "+EncodeChatStart(chat2TelegramID))); err != nil {
		t.Fatalf("Handle(chat2): %v", err)
	}

	var playerOwnerID store.OwnerID
	if err := tx.QueryRow(ctx, `SELECT id FROM owner WHERE kind = 'player' AND telegram_id = $1`, playerTelegramID).Scan(&playerOwnerID); err != nil {
		t.Fatalf("read player owner: %v", err)
	}
	if !membershipExists(t, ctx, tx, chat1.ID, playerOwnerID) {
		t.Fatal("chat1 membership missing")
	}
	if !membershipExists(t, ctx, tx, chat2.ID, playerOwnerID) {
		t.Fatal("chat2 membership missing")
	}

	events := eventsOfType(t, ctx, tx, store.EventPlayerStarted)
	if len(events) != 2 {
		t.Fatalf("player_started events = %d, want 2", len(events))
	}
	if events[0].chatID != chat1.ID {
		t.Fatalf("first event chat_id = %d, want %d", events[0].chatID, chat1.ID)
	}
	if events[1].chatID != chat2.ID {
		t.Fatalf("second event chat_id = %d, want %d", events[1].chatID, chat2.ID)
	}
}

// TestStartHandler_repeatSameChatLinkAppendsNothing covers (c): a second
// Start through the same chat's link leaves one player and one
// membership, and appends no second event.
func TestStartHandler_repeatSameChatLinkAppendsNothing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const chatTelegramID = int64(-200301)
	if _, err := store.CreateOwner(ctx, tx, store.OwnerChat, ptr(chatTelegramID)); err != nil {
		t.Fatalf("CreateOwner(chat): %v", err)
	}

	h := NewStartHandler()
	const playerTelegramID = int64(300301)
	u := startMessageUpdate(1, telego.ChatTypePrivate, playerTelegramID, playerTelegramID, "/start "+EncodeChatStart(chatTelegramID))
	if err := h.Handle(ctx, tx, u); err != nil {
		t.Fatalf("Handle (first): %v", err)
	}
	u2 := startMessageUpdate(2, telego.ChatTypePrivate, playerTelegramID, playerTelegramID, "/start "+EncodeChatStart(chatTelegramID))
	if err := h.Handle(ctx, tx, u2); err != nil {
		t.Fatalf("Handle (repeat): %v", err)
	}

	var playerCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM owner WHERE kind = 'player' AND telegram_id = $1`, playerTelegramID).Scan(&playerCount); err != nil {
		t.Fatalf("count player owner: %v", err)
	}
	if playerCount != 1 {
		t.Fatalf("player owner rows = %d, want 1", playerCount)
	}
	if got := len(eventsOfType(t, ctx, tx, store.EventPlayerStarted)); got != 1 {
		t.Fatalf("player_started events = %d, want 1", got)
	}
}

// TestStartHandler_groupMessageCreatesNothing covers (d): a group
// message from a player who never pressed Start creates nothing.
func TestStartHandler_groupMessageCreatesNothing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	h := NewStartHandler()
	u := startMessageUpdate(1, telego.ChatTypeGroup, -200401, 300401, "/start")
	if err := h.Handle(ctx, tx, u); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	var ownerCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM owner WHERE kind IN ('player', 'chat')`).Scan(&ownerCount); err != nil {
		t.Fatalf("count owner: %v", err)
	}
	if ownerCount != 0 {
		t.Fatalf("player/chat owner rows = %d, want 0", ownerCount)
	}
}

// TestStartHandler_bareStartNoPayload covers (e): a private /start
// with no payload creates the player and records no membership.
func TestStartHandler_bareStartNoPayload(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	h := NewStartHandler()
	const playerTelegramID = int64(300501)
	if err := h.Handle(ctx, tx, startMessageUpdate(1, telego.ChatTypePrivate, playerTelegramID, playerTelegramID, "/start")); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	var playerOwnerID store.OwnerID
	if err := tx.QueryRow(ctx, `SELECT id FROM owner WHERE kind = 'player' AND telegram_id = $1`, playerTelegramID).Scan(&playerOwnerID); err != nil {
		t.Fatalf("read player owner: %v", err)
	}

	events := eventsOfType(t, ctx, tx, store.EventPlayerStarted)
	if len(events) != 1 {
		t.Fatalf("player_started events = %d, want 1", len(events))
	}
	if events[0].chatID != 0 {
		t.Fatalf("event chat_id = %d, want 0 (no chat dimension)", events[0].chatID)
	}
	if _, ok := events[0].payload["chat_telegram_id"]; ok {
		t.Fatalf("payload carries chat_telegram_id = %v, want absent", events[0].payload["chat_telegram_id"])
	}

	var membershipCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM chat_membership WHERE player_id = $1`, playerOwnerID).Scan(&membershipCount); err != nil {
		t.Fatalf("count chat_membership: %v", err)
	}
	if membershipCount != 0 {
		t.Fatalf("chat_membership rows = %d, want 0", membershipCount)
	}
}

// TestStartHandler_laterLinkAfterBareStart covers (e'): the case the
// emission rule exists for — a player who already started bare then
// starts through chat X's link: no player is created, the membership
// is, and a player_started carrying chat X is appended.
func TestStartHandler_laterLinkAfterBareStart(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const chatTelegramID = int64(-200601)
	chatOwner, err := store.CreateOwner(ctx, tx, store.OwnerChat, ptr(chatTelegramID))
	if err != nil {
		t.Fatalf("CreateOwner(chat): %v", err)
	}

	h := NewStartHandler()
	const playerTelegramID = int64(300601)
	if err := h.Handle(ctx, tx, startMessageUpdate(1, telego.ChatTypePrivate, playerTelegramID, playerTelegramID, "/start")); err != nil {
		t.Fatalf("Handle(bare): %v", err)
	}
	var playerOwnerID store.OwnerID
	if err := tx.QueryRow(ctx, `SELECT id FROM owner WHERE kind = 'player' AND telegram_id = $1`, playerTelegramID).Scan(&playerOwnerID); err != nil {
		t.Fatalf("read player owner: %v", err)
	}

	if err := h.Handle(ctx, tx, startMessageUpdate(2, telego.ChatTypePrivate, playerTelegramID, playerTelegramID, "/start "+EncodeChatStart(chatTelegramID))); err != nil {
		t.Fatalf("Handle(link): %v", err)
	}

	var playerCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM owner WHERE kind = 'player' AND telegram_id = $1`, playerTelegramID).Scan(&playerCount); err != nil {
		t.Fatalf("count player owner: %v", err)
	}
	if playerCount != 1 {
		t.Fatalf("player owner rows = %d, want 1 (no second player created)", playerCount)
	}
	if !membershipExists(t, ctx, tx, chatOwner.ID, playerOwnerID) {
		t.Fatal("membership not recorded on the later link")
	}

	events := eventsOfType(t, ctx, tx, store.EventPlayerStarted)
	if len(events) != 2 {
		t.Fatalf("player_started events = %d, want 2", len(events))
	}
	if events[1].chatID != chatOwner.ID {
		t.Fatalf("second event chat_id = %d, want %d", events[1].chatID, chatOwner.ID)
	}
}

// TestStartHandler_linkToUnknownChat covers (f): a payload naming a
// chat the bot was never added to creates the player, records no
// membership, appends the no-chat-dimension event, and leaves the chat
// telegram id visible in its payload.
func TestStartHandler_linkToUnknownChat(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	h := NewStartHandler()
	const playerTelegramID = int64(300701)
	const unknownChatTelegramID = int64(-200701)
	if err := h.Handle(ctx, tx, startMessageUpdate(1, telego.ChatTypePrivate, playerTelegramID, playerTelegramID, "/start "+EncodeChatStart(unknownChatTelegramID))); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	var playerOwnerID store.OwnerID
	if err := tx.QueryRow(ctx, `SELECT id FROM owner WHERE kind = 'player' AND telegram_id = $1`, playerTelegramID).Scan(&playerOwnerID); err != nil {
		t.Fatalf("read player owner: %v", err)
	}
	var membershipCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM chat_membership WHERE player_id = $1`, playerOwnerID).Scan(&membershipCount); err != nil {
		t.Fatalf("count chat_membership: %v", err)
	}
	if membershipCount != 0 {
		t.Fatalf("chat_membership rows = %d, want 0", membershipCount)
	}

	events := eventsOfType(t, ctx, tx, store.EventPlayerStarted)
	if len(events) != 1 {
		t.Fatalf("player_started events = %d, want 1", len(events))
	}
	if events[0].chatID != 0 {
		t.Fatalf("event chat_id = %d, want 0 (no owner row to join to)", events[0].chatID)
	}
	got, ok := events[0].payload["chat_telegram_id"]
	if !ok {
		t.Fatal("payload carries no chat_telegram_id, want it visible even though the chat never resolved")
	}
	if got != float64(unknownChatTelegramID) {
		t.Fatalf("payload chat_telegram_id = %v, want %v", got, unknownChatTelegramID)
	}
}

// TestStartHandler_linkToRemovedChat covers (f'): a payload naming a
// chat the bot was removed from still records the membership and
// appends the event with that chat's dimension, exactly as a live chat
// would.
func TestStartHandler_linkToRemovedChat(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	const chatTelegramID = int64(-200801)
	chatOwner, err := store.CreateOwner(ctx, tx, store.OwnerChat, ptr(chatTelegramID))
	if err != nil {
		t.Fatalf("CreateOwner(chat): %v", err)
	}
	if _, err := chat.SetPresence(ctx, tx, chatOwner.ID, true); err != nil {
		t.Fatalf("SetPresence(true): %v", err)
	}
	if _, err := chat.SetPresence(ctx, tx, chatOwner.ID, false); err != nil {
		t.Fatalf("SetPresence(false): %v", err)
	}

	h := NewStartHandler()
	const playerTelegramID = int64(300801)
	if err := h.Handle(ctx, tx, startMessageUpdate(1, telego.ChatTypePrivate, playerTelegramID, playerTelegramID, "/start "+EncodeChatStart(chatTelegramID))); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	var playerOwnerID store.OwnerID
	if err := tx.QueryRow(ctx, `SELECT id FROM owner WHERE kind = 'player' AND telegram_id = $1`, playerTelegramID).Scan(&playerOwnerID); err != nil {
		t.Fatalf("read player owner: %v", err)
	}
	if !membershipExists(t, ctx, tx, chatOwner.ID, playerOwnerID) {
		t.Fatal("membership not recorded for a removed-but-existing chat")
	}
	events := eventsOfType(t, ctx, tx, store.EventPlayerStarted)
	if len(events) != 1 {
		t.Fatalf("player_started events = %d, want 1", len(events))
	}
	if events[0].chatID != chatOwner.ID {
		t.Fatalf("event chat_id = %d, want %d", events[0].chatID, chatOwner.ID)
	}
}

// TestStartHandler_malformedPayloadTreatedAsBareStart covers (g).
func TestStartHandler_malformedPayloadTreatedAsBareStart(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	h := NewStartHandler()
	const playerTelegramID = int64(300901)
	u := startMessageUpdate(1, telego.ChatTypePrivate, playerTelegramID, playerTelegramID, "/start not-a-valid-payload")
	if err := h.Handle(ctx, tx, u); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	events := eventsOfType(t, ctx, tx, store.EventPlayerStarted)
	if len(events) != 1 {
		t.Fatalf("player_started events = %d, want 1", len(events))
	}
	if events[0].chatID != 0 {
		t.Fatalf("event chat_id = %d, want 0", events[0].chatID)
	}
	if _, ok := events[0].payload["chat_telegram_id"]; ok {
		t.Fatal("payload carries chat_telegram_id for a malformed link, want it absent")
	}
}

// TestStartHandler_redeliveryAppendsNothing covers (h).
func TestStartHandler_redeliveryAppendsNothing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	h := NewStartHandler()
	u := startMessageUpdate(1, telego.ChatTypePrivate, 301001, 301001, "/start")
	if err := h.Handle(ctx, tx, u); err != nil {
		t.Fatalf("Handle (first): %v", err)
	}
	if err := h.Handle(ctx, tx, u); err != nil {
		t.Fatalf("Handle (redelivery): %v", err)
	}
	if got := len(eventsOfType(t, ctx, tx, store.EventPlayerStarted)); got != 1 {
		t.Fatalf("player_started events = %d, want 1", got)
	}
}

func ptr(v int64) *int64 { return &v }
