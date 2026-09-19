package chat

import (
	"context"
	"errors"
	"testing"

	"github.com/maratik123/lab-game/internal/store"
	"github.com/maratik123/lab-game/internal/storetest"
	"github.com/maratik123/lab-game/internal/testdb"
)

func TestAddMembership_idempotentOnRepeat(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tgChat, tgPlayer := int64(6001), int64(6002)
	chatOwner, err := store.CreateOwner(ctx, tx, store.OwnerChat, &tgChat)
	if err != nil {
		t.Fatalf("CreateOwner(chat): %v", err)
	}
	playerOwner, err := store.CreateOwner(ctx, tx, store.OwnerPlayer, &tgPlayer)
	if err != nil {
		t.Fatalf("CreateOwner(player): %v", err)
	}

	created, err := AddMembership(ctx, tx, chatOwner.ID, playerOwner.ID)
	if err != nil {
		t.Fatalf("first AddMembership: %v", err)
	}
	if !created {
		t.Fatalf("first AddMembership created = %v, want true", created)
	}

	created, err = AddMembership(ctx, tx, chatOwner.ID, playerOwner.ID)
	if err != nil {
		t.Fatalf("repeat AddMembership: %v", err)
	}
	if created {
		t.Fatalf("repeat AddMembership created = %v, want false", created)
	}

	var count int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM chat_membership WHERE chat_id = $1 AND player_id = $2`,
		chatOwner.ID, playerOwner.ID,
	).Scan(&count); err != nil {
		t.Fatalf("count chat_membership: %v", err)
	}
	if count != 1 {
		t.Fatalf("chat_membership rows = %d, want 1", count)
	}
}

func TestAddMembership_wrongChatKindRefusedWritingNothing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tg1, tg2 := int64(6003), int64(6004)
	notAChat, err := store.CreateOwner(ctx, tx, store.OwnerPlayer, &tg1)
	if err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}
	player, err := store.CreateOwner(ctx, tx, store.OwnerPlayer, &tg2)
	if err != nil {
		t.Fatalf("CreateOwner(player): %v", err)
	}

	_, err = AddMembership(ctx, tx, notAChat.ID, player.ID)
	if !errors.Is(err, ErrNotAChat) {
		t.Fatalf("AddMembership(not-a-chat, player) = %v, want ErrNotAChat", err)
	}

	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM chat_membership`).Scan(&count); err != nil {
		t.Fatalf("count chat_membership: %v", err)
	}
	if count != 0 {
		t.Fatalf("chat_membership rows = %d, want 0", count)
	}
}

func TestAddMembership_wrongPlayerKindRefusedWritingNothing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tg1, tg2 := int64(6005), int64(6006)
	chatOwner, err := store.CreateOwner(ctx, tx, store.OwnerChat, &tg1)
	if err != nil {
		t.Fatalf("CreateOwner(chat): %v", err)
	}
	notAPlayer, err := store.CreateOwner(ctx, tx, store.OwnerChat, &tg2)
	if err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}

	_, err = AddMembership(ctx, tx, chatOwner.ID, notAPlayer.ID)
	if !errors.Is(err, ErrNotAPlayer) {
		t.Fatalf("AddMembership(chat, not-a-player) = %v, want ErrNotAPlayer", err)
	}

	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM chat_membership`).Scan(&count); err != nil {
		t.Fatalf("count chat_membership: %v", err)
	}
	if count != 0 {
		t.Fatalf("chat_membership rows = %d, want 0", count)
	}
}

// TestPoolLookup_answersBothGateQuestions drives PoolLookup over a real
// pool: the player question, and the presence question keyed on a
// telegram id — false for a chat the bot left and for a telegram id no
// chat owner exists for, neither case an error.
func TestPoolLookup_answersBothGateQuestions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	lookup := NewPoolLookup(pool)

	tgPlayer, tgChatLeft, tgChatPresent, tgNoOwner := int64(6101), int64(6102), int64(6103), int64(6104)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	player, err := store.CreateOwner(ctx, tx, store.OwnerPlayer, &tgPlayer)
	if err != nil {
		t.Fatalf("CreateOwner(player): %v", err)
	}
	chatLeft, err := store.CreateOwner(ctx, tx, store.OwnerChat, &tgChatLeft)
	if err != nil {
		t.Fatalf("CreateOwner(chatLeft): %v", err)
	}
	chatPresent, err := store.CreateOwner(ctx, tx, store.OwnerChat, &tgChatPresent)
	if err != nil {
		t.Fatalf("CreateOwner(chatPresent): %v", err)
	}
	if _, err := SetPresence(ctx, tx, chatLeft.ID, true); err != nil {
		t.Fatalf("SetPresence(chatLeft, true): %v", err)
	}
	if _, err := SetPresence(ctx, tx, chatLeft.ID, false); err != nil {
		t.Fatalf("SetPresence(chatLeft, false): %v", err)
	}
	if _, err := SetPresence(ctx, tx, chatPresent.ID, true); err != nil {
		t.Fatalf("SetPresence(chatPresent, true): %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	_ = player

	gotPlayer, err := lookup.PlayerExists(ctx, tgPlayer)
	if err != nil {
		t.Fatalf("PlayerExists: %v", err)
	}
	if !gotPlayer {
		t.Fatal("PlayerExists(player) = false, want true")
	}

	gotPresent, err := lookup.BotPresentInChat(ctx, tgChatPresent)
	if err != nil {
		t.Fatalf("BotPresentInChat(present): %v", err)
	}
	if !gotPresent {
		t.Fatal("BotPresentInChat(present chat) = false, want true")
	}

	gotLeft, err := lookup.BotPresentInChat(ctx, tgChatLeft)
	if err != nil {
		t.Fatalf("BotPresentInChat(left): %v", err)
	}
	if gotLeft {
		t.Fatal("BotPresentInChat(left chat) = true, want false")
	}

	gotNoOwner, err := lookup.BotPresentInChat(ctx, tgNoOwner)
	if err != nil {
		t.Fatalf("BotPresentInChat(no owner): %v", err)
	}
	if gotNoOwner {
		t.Fatal("BotPresentInChat(no owner at all) = true, want false")
	}
}

// TestAddMembership_nonexistentChatIDRefused pins the branch a
// wrong-kind chat id cannot reach: a chat id with no owner row at all
// (rather than an owner row of the wrong kind) is disambiguated through
// pgx.ErrNoRows on the follow-up read, not through a kind mismatch.
func TestAddMembership_nonexistentChatIDRefused(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tg := int64(6007)
	player, err := store.CreateOwner(ctx, tx, store.OwnerPlayer, &tg)
	if err != nil {
		t.Fatalf("CreateOwner(player): %v", err)
	}

	_, err = AddMembership(ctx, tx, store.OwnerID(999999999), player.ID)
	if !errors.Is(err, ErrNotAChat) {
		t.Fatalf("AddMembership(nonexistent chat id, player) = %v, want ErrNotAChat", err)
	}
}

// TestAddMembership_nonexistentPlayerIDRefused is nonexistentChatIDRefused's
// mirror for the player side.
func TestAddMembership_nonexistentPlayerIDRefused(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tg := int64(6008)
	chatOwner, err := store.CreateOwner(ctx, tx, store.OwnerChat, &tg)
	if err != nil {
		t.Fatalf("CreateOwner(chat): %v", err)
	}

	_, err = AddMembership(ctx, tx, chatOwner.ID, store.OwnerID(999999999))
	if !errors.Is(err, ErrNotAPlayer) {
		t.Fatalf("AddMembership(chat, nonexistent player id) = %v, want ErrNotAPlayer", err)
	}
}

// TestPoolLookup_botPresentInChat_closedPoolSurfacesError asserts a
// closed pool's error is returned rather than papered over as a false —
// PlayerExists' own sibling test pins the same property for the player
// question. A pool built directly here, rather than through the shared
// test-pool helper, avoids a double Close from that helper's own
// cleanup.
func TestPoolLookup_botPresentInChat_closedPoolSurfacesError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := testdb.Schema(t)
	pool, err := store.NewPool(ctx, cfg)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	pool.Close()

	lookup := NewPoolLookup(pool)
	if _, err := lookup.BotPresentInChat(ctx, 1); err == nil {
		t.Fatal("BotPresentInChat over a closed pool: want an error, got nil")
	}
}
