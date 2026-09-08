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
	"github.com/maratik123/lab-game/internal/store"
	"github.com/maratik123/lab-game/internal/tg"
	"github.com/maratik123/lab-game/internal/tgtest"
)

// stubLookup is a PlayerLookup test double with fixed per-id answers, an
// optional error, and a call counter — the fixture this suite needs to
// prove a cached id issues no second lookup.
type stubLookup struct {
	mu      sync.Mutex
	answers map[int64]bool
	err     error
	calls   map[int64]int
}

func newStubLookup(answers map[int64]bool) *stubLookup {
	return &stubLookup{answers: answers, calls: make(map[int64]int)}
}

func (s *stubLookup) PlayerExists(_ context.Context, telegramID int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls[telegramID]++
	if s.err != nil {
		return false, s.err
	}
	return s.answers[telegramID], nil
}

func (s *stubLookup) callCount(telegramID int64) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[telegramID]
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

func TestGate_allowlistedChatKnownAllowed(t *testing.T) {
	t.Parallel()
	g := NewGate([]int64{555}, newStubLookup(nil))
	err := g.AllowCall(context.Background(), tg.Call{Chat: tg.ChatRef{Key: "555", Target: tg.ChatKnown}})
	if err != nil {
		t.Errorf("AllowCall(allowlisted) = %v, want nil", err)
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

func TestGate_lookupErrorRefuses(t *testing.T) {
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
	lookup.answers[42] = true
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
	if got := lookup.callCount(42); got != 1 {
		t.Errorf("PlayerExists calls for id 42 = %d, want exactly 1 (every later AllowCall served from cache)", got)
	}
}

// TestGate_uncommittedOwnerRowIsInvisible pins this behaviour: a
// handler that creates a player's owner row and DMs that player before
// returning is refused on the attempt, because PlayerLookup runs on its
// own pool connection and the loop's still-open transaction has written
// nothing that connection can see. Only after commit does a second
// AllowCall on the SAME Gate — no restart, no new instance — see it.
func TestGate_uncommittedOwnerRowIsInvisible(t *testing.T) {
	t.Parallel()
	pool := newIngestPool(t)
	ctx := context.Background()
	const telegramID = int64(9001)

	g := NewPoolGate(nil, pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := store.CreateOwner(ctx, tx, store.OwnerPlayer, &[]int64{telegramID}[0]); err != nil {
		t.Fatalf("CreateOwner: %v", err)
	}

	// Not committed yet — refused.
	callErr := g.AllowCall(ctx, tg.Call{Chat: tg.ChatRef{Key: "9001", Target: tg.ChatKnown}})
	if !errors.Is(callErr, ErrChatRefused) {
		t.Fatalf("AllowCall before commit = %v, want it to wrap ErrChatRefused (the row is not yet visible)", callErr)
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Same Gate, same pool, only the commit changed.
	if err := g.AllowCall(ctx, tg.Call{Chat: tg.ChatRef{Key: "9001", Target: tg.ChatKnown}}); err != nil {
		t.Fatalf("AllowCall after commit = %v, want nil", err)
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
	lookup.answers[888] = true
	lookup.mu.Unlock()
	_, err = client.API().SendMessage(context.Background(), &telego.SendMessageParams{
		ChatID: telego.ChatID{ID: 888},
		Text:   "hi",
	})
	if err != nil {
		t.Fatalf("SendMessage to an allowed chat: %v", err)
	}
	if got := serverHits.Load(); got != 1 {
		t.Errorf("server hits after the allowed call = %d, want 1", got)
	}
}
