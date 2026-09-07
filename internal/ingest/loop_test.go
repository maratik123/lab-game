package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mymmrac/telego"
	"github.com/shopspring/decimal"

	"github.com/maratik123/lab-game/internal/config"
	"github.com/maratik123/lab-game/internal/store"
	"github.com/maratik123/lab-game/internal/tg"
	"github.com/maratik123/lab-game/internal/tgtest"
)

// testIngestConfig returns a config.Ingest with millisecond-scale retry
// and poll tuning, so retry/poll-cadence assertions run fast (Test
// Design subtask 9's fixtures).
func testIngestConfig() config.Ingest {
	return config.Ingest{
		PollInterval:     20 * time.Millisecond,
		LongPollTimeout:  time.Second,
		BatchLimit:       10,
		RetryMaxAttempts: 3,
		RetryBaseDelay:   5 * time.Millisecond,
		RetryMaxDelay:    20 * time.Millisecond,
	}
}

// newTestClient builds a tg.Client wired to srv.
func newTestClient(t *testing.T, srv *tgtest.Server) *tg.Client {
	t.Helper()
	c, err := tg.New(tg.Options{
		BaseURL: tgtest.BaseURL,
		Token:   tgtest.Token,
		Transport: config.Transport{
			RetryMaxAttempts: 1,
			RetryBaseDelay:   time.Millisecond,
			RetryMaxDelay:    time.Millisecond,
			AttemptTimeout:   5 * time.Second,
		},
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("tg.New: %v", err)
	}
	return c
}

// newLoop builds a Loop wired to srv and pool, with router and observer
// as given.
func newLoop(t *testing.T, srv *tgtest.Server, pool *pgxpool.Pool, router *Router, observer Observer) *Loop {
	t.Helper()
	l, err := New(Options{
		Client:   newTestClient(t, srv),
		Pool:     pool,
		Router:   router,
		Config:   testIngestConfig(),
		Observer: observer,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return l
}

// updatesJSON marshals updates into the JSON array getUpdates returns.
func updatesJSON(t *testing.T, updates []telego.Update) json.RawMessage {
	t.Helper()
	if updates == nil {
		updates = []telego.Update{}
	}
	body, err := json.Marshal(updates)
	if err != nil {
		t.Fatalf("marshal updates: %v", err)
	}
	return body
}

// getUpdatesRequest is what this test file decodes from a captured
// getUpdates request body — a subset of telego.GetUpdatesParams' own
// json tags.
type getUpdatesRequest struct {
	Offset         *int     `json:"offset"`
	Limit          int      `json:"limit"`
	Timeout        int      `json:"timeout"`
	AllowedUpdates []string `json:"allowed_updates"`
}

// requestCapture records every request body a tgtest.Handler sees, under
// a mutex (concurrent by construction — the same posture recordingObserver
// takes).
type requestCapture struct {
	mu       sync.Mutex
	requests []getUpdatesRequest
}

func (c *requestCapture) record(r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req getUpdatesRequest
	_ = json.Unmarshal(body, &req)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests = append(c.requests, req)
}

func (c *requestCapture) all() []getUpdatesRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]getUpdatesRequest, len(c.requests))
	copy(out, c.requests)
	return out
}

// capturing wraps next so every request it answers is first recorded by
// capture.
func capturing(capture *requestCapture, next tgtest.Handler) tgtest.Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		capture.record(r)
		next(w, r)
	}
}

// postTwicePlayerOperation is a Handler posting a trivial balanced batch
// under u's canonical operation_id — the fixture Test Design subtask 9's
// duplicate scenario needs. Its two postings on the same uncontrolled
// account cancel exactly, so no owner or account setup is required.
type postingHandler struct{}

func (postingHandler) Handle(ctx context.Context, tx pgx.Tx, u Update) error {
	return store.Post(ctx, tx,
		&store.PlayerOperation{Source: store.SourceTelegram, OperationID: u.OperationID},
		store.Posting{AccountID: store.WorldMoney, Amount: decimal.RequireFromString("1")},
		store.Posting{AccountID: store.WorldMoney, Amount: decimal.RequireFromString("-1")},
	)
}

// countRows returns the row count of table, for the ledger-row assertions
// the duplicate scenario needs.
func countRows(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestPollOnce_requestShape(t *testing.T) {
	t.Parallel()

	t.Run("empty_router_transmits_the_D3_sentinel_and_no_offset", func(t *testing.T) {
		t.Parallel()
		pool := newIngestPool(t)
		capReq := &requestCapture{}
		srv := tgtest.New(t, capturing(capReq, tgtest.Success(updatesJSON(t, nil))))
		router, err := NewRouter()
		if err != nil {
			t.Fatalf("NewRouter: %v", err)
		}
		l := newLoop(t, srv, pool, router, nil)

		if err := l.PollOnce(context.Background()); err != nil {
			t.Fatalf("PollOnce: %v", err)
		}

		reqs := capReq.all()
		if len(reqs) != 1 {
			t.Fatalf("requests captured = %d, want 1", len(reqs))
		}
		if reqs[0].Offset != nil {
			t.Errorf("Offset = %v, want absent (seeded offset is 0, erased by omitempty)", *reqs[0].Offset)
		}
		if len(reqs[0].AllowedUpdates) != 1 || reqs[0].AllowedUpdates[0] != string(telego.ShippingQueryUpdates) {
			t.Errorf("AllowedUpdates = %v, want the D3 sentinel [%q]", reqs[0].AllowedUpdates, telego.ShippingQueryUpdates)
		}
		if reqs[0].Timeout != int(testIngestConfig().LongPollTimeout/time.Second) {
			t.Errorf("Timeout = %d, want %d", reqs[0].Timeout, int(testIngestConfig().LongPollTimeout/time.Second))
		}
		if reqs[0].Limit != testIngestConfig().BatchLimit {
			t.Errorf("Limit = %d, want %d", reqs[0].Limit, testIngestConfig().BatchLimit)
		}
	})

	t.Run("populated_router_transmits_every_registered_kind", func(t *testing.T) {
		t.Parallel()
		pool := newIngestPool(t)
		capReq := &requestCapture{}
		srv := tgtest.New(t, capturing(capReq, tgtest.Success(updatesJSON(t, nil))))
		router, err := NewRouter(
			Route{Kind: KindMessage, Handler: noopHandler{}},
			Route{Kind: KindCallbackQuery, Handler: noopHandler{}},
		)
		if err != nil {
			t.Fatalf("NewRouter: %v", err)
		}
		l := newLoop(t, srv, pool, router, nil)

		if err := l.PollOnce(context.Background()); err != nil {
			t.Fatalf("PollOnce: %v", err)
		}

		reqs := capReq.all()
		want := []string{string(KindCallbackQuery), string(KindMessage)} // Kinds() sorts
		if len(reqs) != 1 || len(reqs[0].AllowedUpdates) != 2 ||
			reqs[0].AllowedUpdates[0] != want[0] || reqs[0].AllowedUpdates[1] != want[1] {
			t.Fatalf("AllowedUpdates = %v, want %v", reqs[0].AllowedUpdates, want)
		}
		if reqs[0].Limit != testIngestConfig().BatchLimit {
			t.Errorf("Limit = %d, want %d", reqs[0].Limit, testIngestConfig().BatchLimit)
		}
	})
}

func TestLoop_happyPath(t *testing.T) {
	t.Parallel()
	pool := newIngestPool(t)
	rec := &recordingObserver{}
	srv := tgtest.New(t, nil)

	const backdate = 3 * time.Second
	raw := telego.Update{UpdateID: 5, Message: &telego.Message{Date: time.Now().Add(-backdate).Unix(), Chat: telego.Chat{ID: 1}}}
	srv.SetHandler(tgtest.Success(updatesJSON(t, []telego.Update{raw})))

	router, err := NewRouter(Route{Kind: KindMessage, Handler: postingHandler{}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	l := newLoop(t, srv, pool, router, rec)

	if err := l.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}

	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	next, err := readOffset(context.Background(), tx)
	if err != nil {
		t.Fatalf("readOffset: %v", err)
	}
	if next != 6 {
		t.Errorf("offset after settling update_id=5 = %d, want 6", next)
	}

	updates := rec.Updates()
	if len(updates) != 1 || updates[0].Outcome != OutcomeHandled || updates[0].Kind != KindMessage {
		t.Fatalf("Updates() = %+v, want exactly one OutcomeHandled/KindMessage observation", updates)
	}
	// AC18 (design D13): a KindMessage update carries its own date, so
	// LagKnown must be true and Lag must reflect it — not a value read
	// as a healthy zero. The message is backdated by 3s so a mutant
	// lagFor that returns (0, true) unconditionally is caught.
	if !updates[0].LagKnown {
		t.Error("Updates()[0].LagKnown = false, want true (KindMessage carries a date)")
	}
	if updates[0].Lag < backdate/2 {
		t.Errorf("Updates()[0].Lag = %v, want at least ~%v (the message was backdated by %v)", updates[0].Lag, backdate/2, backdate)
	}
}

func TestLoop_transmittedOffsetAfterSettlement(t *testing.T) {
	t.Parallel()
	pool := newIngestPool(t)
	capReq := &requestCapture{}
	srv := tgtest.New(t, nil)

	raw := telego.Update{UpdateID: 99, Message: &telego.Message{Date: time.Now().Unix(), Chat: telego.Chat{ID: 1}}}
	srv.SetHandler(capturing(capReq, tgtest.Success(updatesJSON(t, []telego.Update{raw}))))

	router, err := NewRouter(Route{Kind: KindMessage, Handler: postingHandler{}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	l := newLoop(t, srv, pool, router, nil)

	if err := l.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce (first cycle, settles update_id=99): %v", err)
	}
	srv.SetHandler(capturing(capReq, tgtest.Success(updatesJSON(t, nil))))
	if err := l.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce (second cycle): %v", err)
	}

	reqs := capReq.all()
	if len(reqs) != 2 {
		t.Fatalf("requests captured = %d, want 2", len(reqs))
	}
	if reqs[0].Offset != nil {
		t.Errorf("first request Offset = %v, want absent (seeded 0)", *reqs[0].Offset)
	}
	if reqs[1].Offset == nil || *reqs[1].Offset != 100 {
		t.Errorf("second request Offset = %v, want 100 (update_id 99 + 1)", reqs[1].Offset)
	}
}

func TestLoop_unrouted(t *testing.T) {
	t.Parallel()
	pool := newIngestPool(t)
	rec := &recordingObserver{}
	srv := tgtest.New(t, nil)

	raw := telego.Update{UpdateID: 7, CallbackQuery: &telego.CallbackQuery{ID: "cbq"}}
	srv.SetHandler(tgtest.Success(updatesJSON(t, []telego.Update{raw})))

	router, err := NewRouter(Route{Kind: KindMessage, Handler: postingHandler{}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	l := newLoop(t, srv, pool, router, rec)

	if err := l.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}

	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	next, err := readOffset(context.Background(), tx)
	if err != nil {
		t.Fatalf("readOffset: %v", err)
	}
	if next != 8 {
		t.Errorf("offset after settling unrouted update_id=7 = %d, want 8", next)
	}

	updates := rec.Updates()
	if len(updates) != 1 || updates[0].Outcome != OutcomeUnrouted || updates[0].Kind != KindCallbackQuery {
		t.Fatalf("Updates() = %+v, want exactly one OutcomeUnrouted/KindCallbackQuery observation", updates)
	}
	// AC18 (design D13): KindCallbackQuery declares no date, so LagKnown
	// must be false — a mutant lagFor that returns (0, true) always
	// would make this indistinguishable from a genuinely healthy zero
	// lag.
	if updates[0].LagKnown {
		t.Errorf("Updates()[0].LagKnown = true, want false (KindCallbackQuery declares no date)")
	}
}

// TestLoop_malformedUpdateReportsDerivationError covers R1-10: raw
// carries an empty CallbackQuery.ID, which NewUpdate rejects with
// ErrEmptyID (operation.go's operationID). processUpdate must not
// discard that error — it settles the update as unrouted (still
// advancing the offset, so one bad payload never stalls the batch) and
// carries the error onto the resulting Observation.
func TestLoop_malformedUpdateReportsDerivationError(t *testing.T) {
	t.Parallel()
	pool := newIngestPool(t)
	rec := &recordingObserver{}
	srv := tgtest.New(t, nil)

	raw := telego.Update{UpdateID: 9, CallbackQuery: &telego.CallbackQuery{ID: ""}}
	srv.SetHandler(tgtest.Success(updatesJSON(t, []telego.Update{raw})))

	router, err := NewRouter(Route{Kind: KindMessage, Handler: postingHandler{}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	l := newLoop(t, srv, pool, router, rec)

	if err := l.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce: %v", err)
	}

	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	next, err := readOffset(context.Background(), tx)
	if err != nil {
		t.Fatalf("readOffset: %v", err)
	}
	if next != 10 {
		t.Errorf("offset after settling the malformed update_id=9 = %d, want 10 (one bad payload never stalls the batch)", next)
	}

	updates := rec.Updates()
	if len(updates) != 1 || updates[0].Outcome != OutcomeUnrouted {
		t.Fatalf("Updates() = %+v, want exactly one OutcomeUnrouted observation", updates)
	}
	if !errors.Is(updates[0].Err, ErrEmptyID) {
		t.Errorf("Updates()[0].Err = %v, want ErrEmptyID (NewUpdate's derivation error must not be discarded)", updates[0].Err)
	}
}

func TestLoop_duplicate(t *testing.T) {
	t.Parallel()
	pool := newIngestPool(t)
	rec := &recordingObserver{}
	srv := tgtest.New(t, nil)

	raw := telego.Update{UpdateID: 11, Message: &telego.Message{Date: time.Now().Unix(), Chat: telego.Chat{ID: 1}}}

	router, err := NewRouter(Route{Kind: KindMessage, Handler: postingHandler{}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	l := newLoop(t, srv, pool, router, rec)

	srv.SetHandler(tgtest.Success(updatesJSON(t, []telego.Update{raw})))
	if err := l.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce (first delivery): %v", err)
	}
	// Rewind the offset so the loop re-fetches (and re-settles) the SAME
	// update — the "second delivery" this scenario needs, without a
	// second live update_id from the fake server.
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Exec(context.Background(), "UPDATE ingest_offset SET next_update_id = 0 WHERE id = 1"); err != nil {
		t.Fatalf("rewind offset: %v", err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatalf("commit rewind: %v", err)
	}

	if err := l.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce (second delivery): %v", err)
	}

	if got := countRows(t, pool, "player_operation"); got != 1 {
		t.Errorf("player_operation rows = %d, want 1", got)
	}
	if got := countRows(t, pool, "journal_entry"); got != 1 {
		t.Errorf("journal_entry rows = %d, want 1", got)
	}
	if got := countRows(t, pool, "posting"); got != 2 {
		t.Errorf("posting rows = %d, want 2 (one balanced batch)", got)
	}

	updates := rec.Updates()
	var duplicates int
	for _, o := range updates {
		if o.Outcome == OutcomeDuplicate {
			duplicates++
		}
	}
	if duplicates != 1 {
		t.Fatalf("Updates() = %+v, want exactly one OutcomeDuplicate observation", updates)
	}
}

func TestNew_optionValidation(t *testing.T) {
	t.Parallel()

	router, err := NewRouter()
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	pool := newIngestPool(t)
	srv := tgtest.New(t, tgtest.Success(updatesJSON(t, nil)))
	client := newTestClient(t, srv)
	validConfig := testIngestConfig()

	cases := []struct {
		name  string
		opts  Options
		field string
	}{
		{"nil_client", Options{Pool: pool, Router: router, Config: validConfig}, "Client"},
		{"nil_pool", Options{Client: client, Router: router, Config: validConfig}, "Pool"},
		{"nil_router", Options{Client: client, Pool: pool, Config: validConfig}, "Router"},
		{"zero_poll_interval", Options{Client: client, Pool: pool, Router: router, Config: config.Ingest{
			LongPollTimeout: time.Second, BatchLimit: 1, RetryMaxAttempts: 1, RetryBaseDelay: time.Millisecond, RetryMaxDelay: time.Millisecond,
		}}, "Config.PollInterval"},
		{"zero_long_poll_timeout", Options{Client: client, Pool: pool, Router: router, Config: config.Ingest{
			PollInterval: time.Millisecond, BatchLimit: 1, RetryMaxAttempts: 1, RetryBaseDelay: time.Millisecond, RetryMaxDelay: time.Millisecond,
		}}, "Config.LongPollTimeout"},
		{"zero_retry_base_delay", Options{Client: client, Pool: pool, Router: router, Config: config.Ingest{
			PollInterval: time.Millisecond, LongPollTimeout: time.Second, BatchLimit: 1, RetryMaxAttempts: 1, RetryMaxDelay: time.Millisecond,
		}}, "Config.RetryBaseDelay"},
		{"zero_retry_max_delay", Options{Client: client, Pool: pool, Router: router, Config: config.Ingest{
			PollInterval: time.Millisecond, LongPollTimeout: time.Second, BatchLimit: 1, RetryMaxAttempts: 1, RetryBaseDelay: time.Millisecond,
		}}, "Config.RetryMaxDelay"},
		{"zero_batch_limit", Options{Client: client, Pool: pool, Router: router, Config: config.Ingest{
			PollInterval: time.Millisecond, LongPollTimeout: time.Second, RetryMaxAttempts: 1, RetryBaseDelay: time.Millisecond, RetryMaxDelay: time.Millisecond,
		}}, "Config.BatchLimit"},
		{"zero_retry_max_attempts", Options{Client: client, Pool: pool, Router: router, Config: config.Ingest{
			PollInterval: time.Millisecond, LongPollTimeout: time.Second, BatchLimit: 1, RetryBaseDelay: time.Millisecond, RetryMaxDelay: time.Millisecond,
		}}, "Config.RetryMaxAttempts"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := New(tc.opts)
			var optErr *OptionError
			if err == nil {
				t.Fatalf("New(%s): want an error, got nil", tc.name)
			}
			if !errors.As(err, &optErr) || optErr.Field != tc.field {
				t.Fatalf("New(%s): error = %v, want an *OptionError naming %q", tc.name, err, tc.field)
			}
		})
	}
}

func TestOptionError_rendersFieldAndReason(t *testing.T) {
	t.Parallel()

	err := &OptionError{Field: "Pool", Reason: "must not be nil"}
	want := "ingest: Pool: must not be nil"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
