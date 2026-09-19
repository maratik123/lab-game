package ingest

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/maratik123/lab-game/internal/tg"
)

// DestinationLookup answers the two questions Gate needs from a read: is
// a telegram id a known player, and is the bot currently present in a
// telegram id's chat — declared here by the consumer so a test may
// substitute a stub carrying a call counter per question instead of a
// mock of another package's own row-scanning semantics.
type DestinationLookup interface {
	// PlayerExists reports whether a player owner row exists for
	// telegramID.
	PlayerExists(ctx context.Context, telegramID int64) (bool, error)
	// BotPresentInChat reports whether the bot is currently present in
	// the chat named by telegramID.
	BotPresentInChat(ctx context.Context, telegramID int64) (bool, error)
}

// Gate implements the Telegram client's outbound Gate interface: the
// ALLOWED_CHAT_IDS allowlist, now conditioned on the bot's own current
// presence, plus the player carve-out. ChatNone is always allowed;
// ChatUnknown is always refused — an unverifiable destination is exactly
// what an allowlist exists to stop; a ChatKnown destination is allowed
// when its Key parses as an int64 present in the allowlist AND the bot
// is currently present there, or when a DestinationLookup reports a
// player owner row for it. The cache holds positive PLAYER results
// only, for the Gate's lifetime — nothing negative is remembered, and
// the presence answer is never cached, since a removal must take effect
// at once.
type Gate struct {
	allowed map[int64]struct{}
	lookup  DestinationLookup

	// mu guards cache: AllowCall sits on the outbound path, which is
	// concurrent by construction. The critical section is a
	// map lookup and, on a confirmed miss, one insert; the
	// DestinationLookup call itself happens outside the lock, so a slow
	// lookup never serialises other destinations.
	mu    sync.Mutex
	cache map[int64]struct{}
}

// NewGate builds a Gate over allowedChatIDs and lookup.
func NewGate(allowedChatIDs []int64, lookup DestinationLookup) *Gate {
	allowed := make(map[int64]struct{}, len(allowedChatIDs))
	for _, id := range allowedChatIDs {
		allowed[id] = struct{}{}
	}
	return &Gate{
		allowed: allowed,
		lookup:  lookup,
		cache:   make(map[int64]struct{}),
	}
}

// This pins Gate to the Telegram client's outbound Gate interface at
// compile time.
var _ tg.Gate = (*Gate)(nil)

// AllowCall implements the outbound Gate interface.
func (g *Gate) AllowCall(ctx context.Context, call tg.Call) error {
	switch call.Chat.Target {
	case tg.ChatNone:
		return nil
	case tg.ChatUnknown:
		return fmt.Errorf("%w: unverifiable destination", ErrChatRefused)
	case tg.ChatKnown:
		return g.allowKnown(ctx, call.Chat.Key)
	default:
		return fmt.Errorf("%w: unknown ChatTarget %d", ErrChatRefused, call.Chat.Target)
	}
}

// allowKnown implements the ChatKnown branch of AllowCall: parse key as
// an int64; consult the positive player cache first; then, when id is
// allowlisted, consult the bot's own current presence there — allowed
// when present, a fall-through (not a refusal) when not, so an
// allowlisted player's own DM still reaches the player lookup below;
// finally the player lookup itself.
func (g *Gate) allowKnown(ctx context.Context, key string) error {
	id, err := strconv.ParseInt(key, 10, 64)
	if err != nil {
		return fmt.Errorf("%w: chat id %q is not an integer", ErrChatRefused, key)
	}

	g.mu.Lock()
	_, cached := g.cache[id]
	g.mu.Unlock()
	if cached {
		return nil
	}

	if _, ok := g.allowed[id]; ok {
		present, presenceErr := g.lookup.BotPresentInChat(ctx, id)
		if presenceErr != nil {
			return fmt.Errorf("%w: presence lookup: %w", ErrChatRefused, presenceErr)
		}
		if present {
			return nil
		}
	}

	exists, err := g.lookup.PlayerExists(ctx, id)
	if err != nil {
		return fmt.Errorf("%w: player lookup: %w", ErrChatRefused, err)
	}
	if !exists {
		return fmt.Errorf("%w: chat %d is neither a present allowlisted chat nor a known player", ErrChatRefused, id)
	}

	g.mu.Lock()
	g.cache[id] = struct{}{}
	g.mu.Unlock()
	return nil
}
