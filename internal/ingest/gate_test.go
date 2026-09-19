package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mymmrac/telego"

	"github.com/maratik123/lab-game/internal/backoff"
	"github.com/maratik123/lab-game/internal/config"
	"github.com/maratik123/lab-game/internal/tg"
	"github.com/maratik123/lab-game/internal/tgtest"
)

// stubLookup is a DestinationLookup test double with fixed per-id
// answers for each of the two questions, an optional error, and a call
// counter per question — the fixture this suite needs to prove a cached
// id issues no second lookup and that the branch order is observable
// from the outside.
type stubLookup struct {
	mu sync.Mutex

	players       map[int64]bool
	present       map[int64]bool
	err           error
	playerCalls   map[int64]int
	presenceCalls map[int64]int
}

func newStubLookup(players map[int64]bool) *stubLookup {
	return &stubLookup{
		players:       players,
		present:       make(map[int64]bool),
		playerCalls:   make(map[int64]int),
		presenceCalls: make(map[int64]int),
	}
}

func (s *stubLookup) PlayerExists(_ context.Context, telegramID int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.playerCalls[telegramID]++
	if s.err != nil {
		return false, s.err
	}
	return s.players[telegramID], nil
}

func (s *stubLookup) BotPresentInChat(_ context.Context, telegramID int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.presenceCalls[telegramID]++
	if s.err != nil {
		return false, s.err
	}
	return s.present[telegramID], nil
}

func (s *stubLookup) playerCallCount(telegramID int64) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.playerCalls[telegramID]
}

func (s *stubLookup) presenceCallCount(telegramID int64) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.presenceCalls[telegramID]
}

func TestGate_chatNoneAllowed(t *testing.T) {
	t.Parallel()
	g := NewGate(nil, newStubLookup(nil))
	if err := g.AllowCall(context.Background(), tg.Call{Chat: tg.ChatRef{Target: tg.ChatNone}}); err != nil {
		t.Errorf("AllowCall(ChatNone) = %v, want nil", err)
	}
}

func TestGate_chatUnknownRefused(t *testing.T) {
	t.Parallel()
	g := NewGate(nil, newStubLookup(nil))
	err := g.AllowCall(context.Background(), tg.Call{Chat: tg.ChatRef{Target: tg.ChatUnknown}})
	if !errors.Is(err, ErrChatRefused) {
		t.Errorf("AllowCall(ChatUnknown) = %v, want it to wrap ErrChatRefused", err)
	}
}

func TestGate_allowlistedAndPresentChatAllowed(t *testing.T) {
	t.Parallel()
	lookup := newStubLookup(nil)
	lookup.present[555] = true
	g := NewGate([]int64{555}, lookup)
	err := g.AllowCall(context.Background(), tg.Call{Chat: tg.ChatRef{Key: "555", Target: tg.ChatKnown}})
	if err != nil {
		t.Errorf("AllowCall(allowlisted, present) = %v, want nil", err)
	}
}

// TestGate_removedThenReAddedChat pins presence-gated re-add through the gate: the
// same allowlisted chat is refused after a removal, and allowed again
// after a re-add — proving nothing positive was cached across the
// removal.
func TestGate_removedThenReAddedChat(t *testing.T) {
	t.Parallel()
	lookup := newStubLookup(nil)
	g := NewGate([]int64{555}, lookup)
	call := tg.Call{Chat: tg.ChatRef{Key: "555", Target: tg.ChatKnown}}

	// Not present, not a player: refused.
	if err := g.AllowCall(context.Background(), call); !errors.Is(err, ErrChatRefused) {
		t.Fatalf("AllowCall(allowlisted, not present) = %v, want it to wrap ErrChatRefused", err)
	}

	lookup.mu.Lock()
	lookup.present[555] = true
	lookup.mu.Unlock()
	if err := g.AllowCall(context.Background(), call); err != nil {
		t.Fatalf("AllowCall(allowlisted, present after add) = %v, want nil", err)
	}

	lookup.mu.Lock()
	lookup.present[555] = false
	lookup.mu.Unlock()
	if err := g.AllowCall(context.Background(), call); !errors.Is(err, ErrChatRefused) {
		t.Fatalf("AllowCall(allowlisted, present after removal) = %v, want it to wrap ErrChatRefused (nothing positive was cached)", err)
	}

	lookup.mu.Lock()
	lookup.present[555] = true
	lookup.mu.Unlock()
	if err := g.AllowCall(context.Background(), call); err != nil {
		t.Fatalf("AllowCall(allowlisted, present after re-add) = %v, want nil", err)
	}
}

// TestGate_presentNonAllowlistedChatRefusedWithoutPresenceAsked pins the
// branch order from the outside: a chat that is present but not
// allowlisted is refused without the presence question ever being asked
// — the allowlist stays an outer bound, necessary but not sufficient.
func TestGate_presentNonAllowlistedChatRefusedWithoutPresenceAsked(t *testing.T) {
	t.Parallel()
	lookup := newStubLookup(nil)
	lookup.present[42] = true
	g := NewGate(nil, lookup)
	err := g.AllowCall(context.Background(), tg.Call{Chat: tg.ChatRef{Key: "42", Target: tg.ChatKnown}})
	if !errors.Is(err, ErrChatRefused) {
		t.Fatalf("AllowCall(present, not allowlisted) = %v, want it to wrap ErrChatRefused", err)
	}
	if got := lookup.presenceCallCount(42); got != 0 {
		t.Errorf("presence lookup calls = %d, want 0 (never allowlisted, so the presence question is never asked)", got)
	}
}

// TestGate_allowlistedPlayerDMStillAllowed holds open the carve-out this
// change could silently have closed: an allowlisted destination that is
// a player's own DM — allowlisted, but never a present chat — is still
// allowed, through the fall-through to the player lookup.
func TestGate_allowlistedPlayerDMStillAllowed(t *testing.T) {
	t.Parallel()
	lookup := newStubLookup(map[int64]bool{555: true})
	g := NewGate([]int64{555}, lookup)
	err := g.AllowCall(context.Background(), tg.Call{Chat: tg.ChatRef{Key: "555", Target: tg.ChatKnown}})
	if err != nil {
		t.Errorf("AllowCall(allowlisted player DM, never present) = %v, want nil", err)
	}
}

func TestGate_playerOwnerAllowsAChatOutsideTheAllowlist(t *testing.T) {
	t.Parallel()
	lookup := newStubLookup(map[int64]bool{42: true})
	g := NewGate([]int64{555}, lookup)
	err := g.AllowCall(context.Background(), tg.Call{Chat: tg.ChatRef{Key: "42", Target: tg.ChatKnown}})
	if err != nil {
		t.Errorf("AllowCall(player owner, not allowlisted) = %v, want nil", err)
	}
}

func TestGate_noOwnerRefused(t *testing.T) {
	t.Parallel()
	lookup := newStubLookup(map[int64]bool{})
	g := NewGate([]int64{555}, lookup)
	err := g.AllowCall(context.Background(), tg.Call{Chat: tg.ChatRef{Key: "42", Target: tg.ChatKnown}})
	if !errors.Is(err, ErrChatRefused) {
		t.Errorf("AllowCall(no owner, not allowlisted) = %v, want it to wrap ErrChatRefused", err)
	}
}

func TestGate_nonIntegerTokenRefused(t *testing.T) {
	t.Parallel()
	g := NewGate([]int64{555}, newStubLookup(nil))
	err := g.AllowCall(context.Background(), tg.Call{Chat: tg.ChatRef{Key: "@channelusername", Target: tg.ChatKnown}})
	if !errors.Is(err, ErrChatRefused) {
		t.Errorf("AllowCall(non-integer token) = %v, want it to wrap ErrChatRefused", err)
	}
}

// TestGate_presenceLookupErrorRefuses pins the moved failure mode: an
// allowlisted destination whose presence lookup errors is refused —
// fail-closed — rather than allowed unconditionally the way the bare
// allowlist check used to.
func TestGate_presenceLookupErrorRefuses(t *testing.T) {
	t.Parallel()
	lookup := newStubLookup(nil)
	lookup.err = errors.New("boom")
	g := NewGate([]int64{42}, lookup)
	err := g.AllowCall(context.Background(), tg.Call{Chat: tg.ChatRef{Key: "42", Target: tg.ChatKnown}})
	if !errors.Is(err, ErrChatRefused) {
		t.Errorf("AllowCall(presence lookup error) = %v, want it to wrap ErrChatRefused", err)
	}
}

func TestGate_playerLookupErrorRefuses(t *testing.T) {
	t.Parallel()
	lookup := newStubLookup(nil)
	lookup.err = errors.New("boom")
	g := NewGate(nil, lookup)
	err := g.AllowCall(context.Background(), tg.Call{Chat: tg.ChatRef{Key: "42", Target: tg.ChatKnown}})
	if !errors.Is(err, ErrChatRefused) {
		t.Errorf("AllowCall(lookup error) = %v, want it to wrap ErrChatRefused", err)
	}
}

func TestGate_refusedThenAllowedWithNoRestart(t *testing.T) {
	t.Parallel()
	lookup := newStubLookup(map[int64]bool{42: false})
	g := NewGate(nil, lookup)

	if err := g.AllowCall(context.Background(), tg.Call{Chat: tg.ChatRef{Key: "42", Target: tg.ChatKnown}}); !errors.Is(err, ErrChatRefused) {
		t.Fatalf("first AllowCall = %v, want it to wrap ErrChatRefused", err)
	}

	lookup.mu.Lock()
	lookup.players[42] = true
	lookup.mu.Unlock()

	if err := g.AllowCall(context.Background(), tg.Call{Chat: tg.ChatRef{Key: "42", Target: tg.ChatKnown}}); err != nil {
		t.Fatalf("second AllowCall (after the row now exists) = %v, want nil — same Gate, no restart", err)
	}
}

func TestGate_positiveResultIsCachedWithNoSecondLookup(t *testing.T) {
	t.Parallel()
	lookup := newStubLookup(map[int64]bool{42: true})
	g := NewGate(nil, lookup)

	for range 3 {
		if err := g.AllowCall(context.Background(), tg.Call{Chat: tg.ChatRef{Key: "42", Target: tg.ChatKnown}}); err != nil {
			t.Fatalf("AllowCall = %v, want nil", err)
		}
	}
	if got := lookup.playerCallCount(42); got != 1 {
		t.Errorf("PlayerExists calls for id 42 = %d, want exactly 1 (every later AllowCall served from cache)", got)
	}
}

// TestGate_presenceIsNeverCached asserts the presence answer is
// consulted on every call, unlike the player cache: a flip must take
// effect on the very next AllowCall.
func TestGate_presenceIsNeverCached(t *testing.T) {
	t.Parallel()
	lookup := newStubLookup(nil)
	lookup.present[42] = true
	g := NewGate([]int64{42}, lookup)
	call := tg.Call{Chat: tg.ChatRef{Key: "42", Target: tg.ChatKnown}}

	for range 3 {
		if err := g.AllowCall(context.Background(), call); err != nil {
			t.Fatalf("AllowCall = %v, want nil", err)
		}
	}
	if got := lookup.presenceCallCount(42); got != 3 {
		t.Errorf("presence lookup calls = %d, want 3 (the presence answer is never cached)", got)
	}
}

func TestGate_uncommittedOwnerRowIsInvisible(t *testing.T) {
	t.Parallel()
	lookup := newStubLookup(map[int64]bool{})
	g := NewGate(nil, lookup)

	callErr := g.AllowCall(context.Background(), tg.Call{Chat: tg.ChatRef{Key: "9001", Target: tg.ChatKnown}})
	if !errors.Is(callErr, ErrChatRefused) {
		t.Fatalf("AllowCall before the row is visible to this lookup = %v, want it to wrap ErrChatRefused", callErr)
	}

	lookup.mu.Lock()
	lookup.players[9001] = true
	lookup.mu.Unlock()

	if err := g.AllowCall(context.Background(), tg.Call{Chat: tg.ChatRef{Key: "9001", Target: tg.ChatKnown}}); err != nil {
		t.Fatalf("AllowCall once the row is visible = %v, want nil", err)
	}
}

func TestGate_concurrentAllowCall(t *testing.T) {
	answers := map[int64]bool{1: true, 2: true, 3: false, 4: false}
	lookup := newStubLookup(answers)
	g := NewGate(nil, lookup)

	const goroutinesPerID = 20
	var wg sync.WaitGroup
	var mismatches atomic.Int32
	for id, want := range answers {
		for range goroutinesPerID {
			wg.Add(1)
			go func(id int64, want bool) {
				defer wg.Done()
				err := g.AllowCall(context.Background(), tg.Call{Chat: tg.ChatRef{Key: strconv.FormatInt(id, 10), Target: tg.ChatKnown}})
				allowed := err == nil
				if allowed != want {
					mismatches.Add(1)
				}
			}(id, want)
		}
	}
	wg.Wait()
	if n := mismatches.Load(); n != 0 {
		t.Errorf("%d of %d concurrent AllowCall results disagreed with the stub's fixed answer", n, goroutinesPerID*len(answers))
	}
}

func TestGate_integrationRefusedCallNeverReachesTheServer(t *testing.T) {
	t.Parallel()
	var serverHits atomic.Int32
	srv := tgtest.New(t, func(w http.ResponseWriter, r *http.Request) {
		serverHits.Add(1)
		tgtest.Success(json.RawMessage(`{"message_id":1,"date":0,"chat":{"id":1,"type":"private"}}`))(w, r)
	})

	lookup := newStubLookup(map[int64]bool{})
	gate := NewGate(nil, lookup)

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

	_, err = client.API().SendMessage(context.Background(), &telego.SendMessageParams{
		ChatID: telego.ChatID{ID: 777},
		Text:   "hi",
	})
	if err == nil {
		t.Fatal("SendMessage to a refused chat: want an error, got nil")
	}
	if got := serverHits.Load(); got != 0 {
		t.Errorf("server hits = %d, want 0 (the gate must refuse before any attempt)", got)
	}

	// A subsequent allowed call still reaches the server.
	lookup.mu.Lock()
	lookup.players[888] = true
	lookup.mu.Unlock()
	_, err = client.API().SendMessage(context.Background(), &telego.SendMessageParams{
		ChatID: telego.ChatID{ID: 888},
		Text:   "hi",
	})
	if err != nil {
		t.Fatalf("SendMessage to an allowed chat: %v", err)
	}
	if got := serverHits.Load(); got != 1 {
		t.Errorf("server hits = %d, want 1", got)
	}
}
