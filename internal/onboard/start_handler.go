package onboard

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/mymmrac/telego"

	"github.com/maratik123/lab-game/internal/chat"
	"github.com/maratik123/lab-game/internal/ingest"
	"github.com/maratik123/lab-game/internal/store"
)

// startCommand is the exact command word this handler recognises. A
// message equal to it, or starting with it followed by a space, is a
// Start; anything else — including a group message — is a no-op.
const startCommand = "/start"

// startEventPayload is player_started's payload: the player's own
// telegram id; the chat telegram id the link carried, when one parsed,
// whether or not it resolved to an owner row; and which of the two
// arrivals this occurrence is — the player's own creation, a new
// membership, or both in one transaction — so a reader of the log can
// tell a first arrival from a later one without a join back to owner.
type startEventPayload struct {
	PlayerTelegramID  int64  `json:"player_telegram_id"`
	ChatTelegramID    *int64 `json:"chat_telegram_id,omitempty"`
	PlayerCreated     bool   `json:"player_created"`
	MembershipCreated bool   `json:"membership_created"`
}

// StartHandler handles a private /start message: it ensures the
// player's own owner row, parses an optional chat deep-link payload and
// records the membership when the payload names a chat that exists in
// the game, and appends player_started exactly when this transaction
// created the player row or a membership row.
type StartHandler struct{}

// NewStartHandler builds a StartHandler.
func NewStartHandler() *StartHandler {
	return &StartHandler{}
}

// This pins StartHandler to the update-routing package's own Handler
// interface at compile time.
var _ ingest.Handler = (*StartHandler)(nil)

// Handle runs u's effects on tx, per the routing package's own Handler
// contract.
func (h *StartHandler) Handle(ctx context.Context, tx pgx.Tx, u ingest.Update) error {
	msg := u.Raw.Message
	if msg == nil {
		return nil
	}
	if msg.Chat.Type != telego.ChatTypePrivate {
		// All game commands happen in DM by design; a group message
		// creates no player, no membership and no event.
		return nil
	}
	if msg.Text != startCommand && !strings.HasPrefix(msg.Text, startCommand+" ") {
		return nil
	}
	if msg.From == nil {
		return fmt.Errorf("onboard: /start message carries no sender")
	}

	// A malformed payload is treated as no payload, not as an error
	// that dead-letters the update.
	var chatTelegramID *int64
	if rawPayload := strings.TrimSpace(strings.TrimPrefix(msg.Text, startCommand)); rawPayload != "" {
		if id, err := ParseChatStart(rawPayload); err == nil {
			chatTelegramID = &id
		}
	}

	playerTelegramID := msg.From.ID
	player, playerCreated, err := store.EnsureOwner(ctx, tx, store.OwnerPlayer, &playerTelegramID)
	if err != nil {
		return fmt.Errorf("onboard: ensure player owner: %w", err)
	}

	// The chat side is a read, never a create (see the package's own
	// design decision): a payload naming a chat with no owner row
	// records no membership rather than conjuring one.
	var chatOwnerID store.OwnerID
	var chatResolved, membershipCreated bool
	if chatTelegramID != nil {
		id, found, lookupErr := store.ChatOwnerID(ctx, tx, *chatTelegramID)
		switch {
		case lookupErr != nil:
			return fmt.Errorf("onboard: read chat owner: %w", lookupErr)
		case found:
			chatOwnerID = id
			chatResolved = true
			created, addErr := chat.AddMembership(ctx, tx, chatOwnerID, player.ID)
			if addErr != nil {
				return fmt.Errorf("onboard: add membership: %w", addErr)
			}
			membershipCreated = created
		default:
			// The link names a chat the bot was never added to.
		}
	}

	if !playerCreated && !membershipCreated {
		// Neither row is new: a repeat bare Start, or a repeat Start
		// through a link for a chat the player already belongs to.
		return nil
	}

	payload := startEventPayload{
		PlayerTelegramID:  playerTelegramID,
		ChatTelegramID:    chatTelegramID,
		PlayerCreated:     playerCreated,
		MembershipCreated: membershipCreated,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("onboard: marshal player_started payload: %w", err)
	}
	var chatDim *store.OwnerID
	if chatResolved {
		chatDim = &chatOwnerID
	}
	if _, err := store.AppendEvent(ctx, tx, store.Event{
		Type:     store.EventPlayerStarted,
		PlayerID: &player.ID,
		ChatID:   chatDim,
		Payload:  data,
	}); err != nil {
		return fmt.Errorf("onboard: append player_started: %w", err)
	}
	return nil
}
