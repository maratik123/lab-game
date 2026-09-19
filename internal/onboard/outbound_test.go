package onboard

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/mymmrac/telego"

	"github.com/maratik123/lab-game/internal/backoff"
	"github.com/maratik123/lab-game/internal/chat"
	"github.com/maratik123/lab-game/internal/config"
	"github.com/maratik123/lab-game/internal/ingest"
	"github.com/maratik123/lab-game/internal/store"
	"github.com/maratik123/lab-game/internal/storetest"
	"github.com/maratik123/lab-game/internal/tg"
	"github.com/maratik123/lab-game/internal/tgtest"
)

// TestOutbound_deepLinkThroughTheRealGateAndClient drives a real
// sendMessage, carrying the deep link, through the real Telegram client
// and the real gate: a present, allowlisted chat is allowed and the
// request body the fake server received carries the link's payload; the
// same send to the same chat, after a removal, is refused by the gate.
func TestOutbound_deepLinkThroughTheRealGateAndClient(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := storetest.Pool(t)

	chatTelegramID := int64(-300101)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	chatOwner, err := store.CreateOwner(ctx, tx, store.OwnerChat, &chatTelegramID)
	if err != nil {
		t.Fatalf("CreateOwner(chat): %v", err)
	}
	if _, err := chat.SetPresence(ctx, tx, chatOwner.ID, true); err != nil {
		t.Fatalf("SetPresence(true): %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var lastBody []byte
	srv := tgtest.New(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		lastBody = body
		tgtest.Success(json.RawMessage(`{"message_id":1,"date":0,"chat":{"id":1,"type":"private"}}`))(w, r)
	})

	gate := ingest.NewGate([]int64{chatTelegramID}, chat.NewPoolLookup(pool))
	client, err := tg.New(tg.Options{
		BaseURL: tgtest.BaseURL,
		Token:   tgtest.Token,
		Transport: config.Transport{
			RetryMaxAttempts: 1,
			RetryBaseDelay:   time.Millisecond,
			RetryMaxDelay:    time.Millisecond,
			RetryFactor:      backoff.DefaultFactor,
			AttemptTimeout:   5 * time.Second,
		},
		HTTPClient: srv.Client(),
		Gate:       gate,
	})
	if err != nil {
		t.Fatalf("tg.New: %v", err)
	}

	const botUsername = "lab_game_bot"
	link, err := ChatStartLink(botUsername, chatTelegramID)
	if err != nil {
		t.Fatalf("ChatStartLink: %v", err)
	}

	sendWithLink := func() error {
		_, err := client.API().SendMessage(ctx, &telego.SendMessageParams{
			ChatID: telego.ChatID{ID: chatTelegramID},
			Text:   "welcome",
			ReplyMarkup: &telego.InlineKeyboardMarkup{
				InlineKeyboard: [][]telego.InlineKeyboardButton{{{Text: "Open", URL: link}}},
			},
		})
		return err
	}

	if err := sendWithLink(); err != nil {
		t.Fatalf("SendMessage to a present, allowlisted chat: %v", err)
	}
	var got struct {
		ReplyMarkup struct {
			InlineKeyboard [][]struct {
				URL string `json:"url"`
			} `json:"inline_keyboard"`
		} `json:"reply_markup"`
	}
	if err := json.Unmarshal(lastBody, &got); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	if len(got.ReplyMarkup.InlineKeyboard) != 1 || len(got.ReplyMarkup.InlineKeyboard[0]) != 1 {
		t.Fatalf("request body inline keyboard = %+v, want one row of one button", got.ReplyMarkup.InlineKeyboard)
	}
	if gotURL := got.ReplyMarkup.InlineKeyboard[0][0].URL; gotURL != link {
		t.Fatalf("request body button url = %q, want %q", gotURL, link)
	}

	// A removal refuses the same destination.
	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := chat.SetPresence(ctx, tx2, chatOwner.ID, false); err != nil {
		t.Fatalf("SetPresence(false): %v", err)
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if err := sendWithLink(); err == nil {
		t.Fatal("SendMessage to the same chat after a removal: want an error, got nil")
	}
}
