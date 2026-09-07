package ingest

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maratik123/lab-game/internal/store"
	"github.com/maratik123/lab-game/internal/tg"
)

// PlayerLookup reports whether a kind='player' owner row exists for a
// telegram id — the single method Gate needs from a read (design D11),
// declared here by the consumer so a test may substitute a stub carrying
// a call counter (AC29, AC35) instead of a mock of internal/store's own
// row-scanning semantics.
type PlayerLookup interface {
	// PlayerExists reports whether a player owner row exists for
	// telegramID.
	PlayerExists(ctx context.Context, telegramID int64) (bool, error)
}

// poolPlayerLookup implements PlayerLookup over a *pgxpool.Pool — the
// gate's own connection, taken at wiring time rather than borrowed from
// any in-flight transaction (design D18: a row a handler's own
// uncommitted transaction wrote is not visible here, by design).
type poolPlayerLookup struct {
	pool *pgxpool.Pool
}

// PlayerExists implements PlayerLookup.
func (p poolPlayerLookup) PlayerExists(ctx context.Context, telegramID int64) (bool, error) {
	return store.PlayerExists(ctx, p.pool, telegramID)
}

// Gate implements tg.Gate: the ALLOWED_CHAT_IDS allowlist plus the
// player carve-out (design D10). ChatNone is always allowed; ChatUnknown
// is always refused — an unverifiable destination is exactly what an
// allowlist exists to stop; a ChatKnown destination is allowed when its
// Key parses as an int64 present in the allowlist, or when a
// PlayerLookup reports a player owner row for it. The cache holds
// positive results only, for the Gate's lifetime — nothing negative is
// remembered, so a player who presses Start after a refusal is allowed
// on the next attempt with no restart (AC35).
type Gate struct {
	allowed map[int64]struct{}
	lookup  PlayerLookup

	// mu guards cache: AllowCall sits on the outbound path, which is
	// concurrent by construction (design D10). The critical section is a
	// map lookup and, on a confirmed miss, one insert; the PlayerLookup
	// call itself happens outside the lock, so a slow lookup never
	// serialises other destinations.
	mu    sync.Mutex
	cache map[int64]struct{}
}

// NewGate builds a Gate over allowedChatIDs and lookup.
func NewGate(allowedChatIDs []int64, lookup PlayerLookup) *Gate {
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

// NewPoolGate builds a Gate whose PlayerLookup reads pool directly —
// design D17's wiring shape: gate, then client, then loop.
func NewPoolGate(allowedChatIDs []int64, pool *pgxpool.Pool) *Gate {
	return NewGate(allowedChatIDs, poolPlayerLookup{pool: pool})
}

// var _ tg.Gate = (*Gate)(nil) pins Gate to tg.Gate's contract at compile
// time.
var _ tg.Gate = (*Gate)(nil)

// AllowCall implements tg.Gate (design D10).
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
// an int64, allow it outright when it is in the allowlist, then consult
// the cache and finally PlayerLookup (design D10, D11).
func (g *Gate) allowKnown(ctx context.Context, key string) error {
	id, err := strconv.ParseInt(key, 10, 64)
	if err != nil {
		return fmt.Errorf("%w: chat id %q is not an integer", ErrChatRefused, key)
	}

	if _, ok := g.allowed[id]; ok {
		return nil
	}

	g.mu.Lock()
	_, cached := g.cache[id]
	g.mu.Unlock()
	if cached {
		return nil
	}

	exists, err := g.lookup.PlayerExists(ctx, id)
	if err != nil {
		return fmt.Errorf("%w: player lookup: %w", ErrChatRefused, err)
	}
	if !exists {
		return fmt.Errorf("%w: chat %d is neither allowlisted nor a known player", ErrChatRefused, id)
	}

	g.mu.Lock()
	g.cache[id] = struct{}{}
	g.mu.Unlock()
	return nil
}
