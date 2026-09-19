package onboard

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/mymmrac/telego"

	"github.com/maratik123/lab-game/internal/store"
	"github.com/maratik123/lab-game/internal/storetest"
)

func funnelPlayerStarted(t *testing.T, ctx context.Context, tx pgx.Tx, chatOwnerID store.OwnerID) int64 {
	t.Helper()
	var count int64
	if err := tx.QueryRow(ctx,
		`SELECT player_started FROM metric_activation_funnel WHERE chat_id = $1`, chatOwnerID,
	).Scan(&count); err != nil {
		t.Fatalf("query metric_activation_funnel: %v", err)
	}
	return count
}

// TestFunnel_addThenStartAttributesPlayer covers (c): handling an add
// update and then a Start update through that chat's link makes the
// funnel report that chat with the player attributed to it, computed
// from the recorded events alone.
func TestFunnel_addThenStartAttributesPlayer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	presence := NewPresenceHandler()
	start := NewStartHandler()

	const chatTelegramID = int64(-400101)
	if err := presence.Handle(ctx, tx, myChatMemberUpdate(1, chatTelegramID, telego.ChatTypeGroup, "Settlement", 500, 1000, leftStatus(999), memberStatus(999))); err != nil {
		t.Fatalf("Handle(add): %v", err)
	}

	const playerTelegramID = int64(400102)
	if err := start.Handle(ctx, tx, startMessageUpdate(2, telego.ChatTypePrivate, playerTelegramID, playerTelegramID, "/start "+EncodeChatStart(chatTelegramID))); err != nil {
		t.Fatalf("Handle(start): %v", err)
	}

	var chatOwnerID store.OwnerID
	if err := tx.QueryRow(ctx, `SELECT id FROM owner WHERE kind = 'chat' AND telegram_id = $1`, chatTelegramID).Scan(&chatOwnerID); err != nil {
		t.Fatalf("read chat owner: %v", err)
	}

	if got := funnelPlayerStarted(t, ctx, tx, chatOwnerID); got != 1 {
		t.Fatalf("metric_activation_funnel.player_started for the chat = %d, want 1", got)
	}
}

// TestFunnel_bareStartThenLinkAttributesFirstChat covers (d): the case
// the emission rule turns on — a bare Start, then a Start through that
// chat's link, and the funnel attributes the player to that chat, which
// is false under an emit-on-player-creation-only rule.
func TestFunnel_bareStartThenLinkAttributesFirstChat(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	presence := NewPresenceHandler()
	start := NewStartHandler()

	const chatTelegramID = int64(-400201)
	if err := presence.Handle(ctx, tx, myChatMemberUpdate(1, chatTelegramID, telego.ChatTypeGroup, "Settlement", 500, 1000, leftStatus(999), memberStatus(999))); err != nil {
		t.Fatalf("Handle(add): %v", err)
	}

	const playerTelegramID = int64(400202)
	if err := start.Handle(ctx, tx, startMessageUpdate(2, telego.ChatTypePrivate, playerTelegramID, playerTelegramID, "/start")); err != nil {
		t.Fatalf("Handle(bare start): %v", err)
	}
	if err := start.Handle(ctx, tx, startMessageUpdate(3, telego.ChatTypePrivate, playerTelegramID, playerTelegramID, "/start "+EncodeChatStart(chatTelegramID))); err != nil {
		t.Fatalf("Handle(link start): %v", err)
	}

	var chatOwnerID store.OwnerID
	if err := tx.QueryRow(ctx, `SELECT id FROM owner WHERE kind = 'chat' AND telegram_id = $1`, chatTelegramID).Scan(&chatOwnerID); err != nil {
		t.Fatalf("read chat owner: %v", err)
	}

	if got := funnelPlayerStarted(t, ctx, tx, chatOwnerID); got != 1 {
		t.Fatalf("metric_activation_funnel.player_started for the chat = %d, want 1 (attributed via the membership event, not the bare-Start one)", got)
	}
}
