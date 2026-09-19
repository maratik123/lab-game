package onboard

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mymmrac/telego"

	"github.com/maratik123/lab-game/internal/chat"
	"github.com/maratik123/lab-game/internal/ingest"
	"github.com/maratik123/lab-game/internal/store"
)

// presenceEventPayload is the JSON shape both presence events share:
// which chat, its type and title, who acted, when, and the raw
// old/new statuses the update itself carried.
type presenceEventPayload struct {
	ChatTelegramID  int64     `json:"chat_telegram_id"`
	ChatType        string    `json:"chat_type"`
	ChatTitle       string    `json:"chat_title"`
	ActorTelegramID int64     `json:"actor_telegram_id"`
	Date            time.Time `json:"date"`
	OldStatus       string    `json:"old_status"`
	NewStatus       string    `json:"new_status"`
}

// kickedEventPayload adds the update id to presenceEventPayload — the
// terminal product metric every bot_kicked occurrence is, so the exact
// update that produced it is worth keeping.
type kickedEventPayload struct {
	presenceEventPayload
	UpdateID int `json:"update_id"`
}

// PresenceHandler handles a my_chat_member update: it ensures the
// chat's own owner row and home scope exist, writes the bot's current
// presence on every handled update, and appends bot_added_to_chat or
// bot_kicked exactly when that presence actually changes.
type PresenceHandler struct{}

// NewPresenceHandler builds a PresenceHandler.
func NewPresenceHandler() *PresenceHandler {
	return &PresenceHandler{}
}

// This pins PresenceHandler to the update-routing package's own Handler
// interface at compile time.
var _ ingest.Handler = (*PresenceHandler)(nil)

// Handle runs u's effects on tx, per the routing package's own Handler
// contract.
func (h *PresenceHandler) Handle(ctx context.Context, tx pgx.Tx, u ingest.Update) error {
	cm := u.Raw.MyChatMember
	if cm == nil || cm.OldChatMember == nil || cm.NewChatMember == nil {
		return fmt.Errorf("onboard: my_chat_member update carries no chat member change")
	}

	switch cm.Chat.Type {
	case telego.ChatTypeGroup, telego.ChatTypeSupergroup:
	default:
		// A settlement is a group chat; a private or channel
		// my_chat_member is a no-op.
		return nil
	}

	chatTelegramID := cm.Chat.ID
	owner, _, err := store.EnsureOwner(ctx, tx, store.OwnerChat, &chatTelegramID)
	if err != nil {
		return fmt.Errorf("onboard: ensure chat owner: %w", err)
	}

	// The prior value is the stored row when one exists, and otherwise
	// the update's own old_chat_member membership — so the very first
	// update the bot ever sees for a chat is judged against what
	// Telegram says the prior state was, rather than against an
	// absence.
	priorPresent, hadRow, err := chat.Present(ctx, tx, owner.ID)
	if err != nil {
		return fmt.Errorf("onboard: read presence: %w", err)
	}
	if !hadRow {
		priorPresent = cm.OldChatMember.MemberIsMember()
	}
	newPresent := cm.NewChatMember.MemberIsMember()

	// The presence row is written on every handled update, change or
	// no change — only the event below is conditional.
	if _, err := chat.SetPresence(ctx, tx, owner.ID, newPresent); err != nil {
		return fmt.Errorf("onboard: set presence: %w", err)
	}

	if priorPresent == newPresent {
		return nil
	}

	payload := presenceEventPayload{
		ChatTelegramID:  chatTelegramID,
		ChatType:        cm.Chat.Type,
		ChatTitle:       cm.Chat.Title,
		ActorTelegramID: cm.From.ID,
		Date:            time.Unix(cm.Date, 0).UTC(),
		OldStatus:       cm.OldChatMember.MemberStatus(),
		NewStatus:       cm.NewChatMember.MemberStatus(),
	}

	if newPresent {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("onboard: marshal bot_added_to_chat payload: %w", err)
		}
		if _, err := store.AppendEvent(ctx, tx, store.Event{
			Type:    store.EventBotAddedToChat,
			ChatID:  &owner.ID,
			Payload: data,
		}); err != nil {
			return fmt.Errorf("onboard: append bot_added_to_chat: %w", err)
		}
		return nil
	}

	data, err := json.Marshal(kickedEventPayload{presenceEventPayload: payload, UpdateID: u.Raw.UpdateID})
	if err != nil {
		return fmt.Errorf("onboard: marshal bot_kicked payload: %w", err)
	}
	if _, err := store.AppendEvent(ctx, tx, store.Event{
		Type:    store.EventBotKicked,
		ChatID:  &owner.ID,
		Payload: data,
	}); err != nil {
		return fmt.Errorf("onboard: append bot_kicked: %w", err)
	}
	return nil
}
