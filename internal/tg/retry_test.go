package tg

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	ta "github.com/mymmrac/telego/telegoapi"

	"github.com/maratik123/lab-game/internal/backoff"
	"github.com/maratik123/lab-game/internal/tgtest"
)

// newRetryTestClient builds a Client against srv with tr as its transport
// tuning — used throughout this file so every retry-loop scenario shares
// one construction path.
func newRetryTestClient(t *testing.T, srv *tgtest.Server, tr func(*Options)) *Client {
	t.Helper()
	return newTestClient(t, srv, tr)
}

// countingHandler records the real time (the bubble's virtual clock) of
// every request it answers, then delegates to next.
type countingHandler struct {
	mu    sync.Mutex
	times []time.Time
	next  tgtest.Handler
}

func (h *countingHandler) handle(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	h.times = append(h.times, time.Now())
	h.mu.Unlock()
	h.next(w, r)
}

func (h *countingHandler) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.times)
}

func (h *countingHandler) timestamps() []time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]time.Time(nil), h.times...)
}

func TestRetry_RetryAfterHonouredExactly(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		first := true
		h := &countingHandler{next: func(w http.ResponseWriter, r *http.Request) {
			if first {
				first = false
				tgtest.TooManyRequests(2)(w, r)
				return
			}
			tgtest.Success(nil)(w, r)
		}}
		srv := tgtest.New(t, h.handle)
		c := newRetryTestClient(t, srv, nil)

		start := time.Now()
		if _, err := c.API().GetMe(context.Background()); err != nil {
			t.Fatalf("GetMe: %v", err)
		}
		elapsed := time.Since(start)
		if elapsed < 2*time.Second {
			t.Errorf("elapsed = %v, want >= 2s (retry_after must never be shortened)", elapsed)
		}
		if elapsed > 2*time.Second+50*time.Millisecond {
			t.Errorf("elapsed = %v, want close to 2s (retry_after must not be extended by backoff too)", elapsed)
		}
	})
}

func TestRetry_DelaysGrowAndStayPositive(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		h := &countingHandler{next: tgtest.ServerError(http.StatusInternalServerError)}
		srv := tgtest.New(t, h.handle)
		tr := validTransport()
		tr.RetryMaxAttempts = 4
		tr.RetryBaseDelay = 100 * time.Millisecond
		tr.RetryMaxDelay = 10 * time.Second
		c := newRetryTestClient(t, srv, func(o *Options) { o.Transport = tr })

		_, err := c.API().GetMe(context.Background())
		if err == nil {
			t.Fatal("GetMe: expected a give-up error")
		}
		times := h.timestamps()
		if len(times) != 4 {
			t.Fatalf("attempts = %d, want 4", len(times))
		}
		var lastDelay time.Duration
		for i := 1; i < len(times); i++ {
			delay := times[i].Sub(times[i-1])
			if delay <= 0 {
				t.Fatalf("delay[%d] = %v, want strictly positive (no immediate re-attempt)", i, delay)
			}
			if i > 1 && delay < lastDelay {
				t.Errorf("delay[%d] = %v, want >= previous delay %v (growing)", i, delay, lastDelay)
			}
			lastDelay = delay
		}
	})
}

// TestRetry_JitterOptionThreadedThroughToBackoff is self-review round 4
// finding 3's fix: Options.Jitter (client.go) is documented to supply
// design D6's backoff formula's random factor, but no test ever set it —
// TestBackoffDelay_JitterBoundsExactly calls backoff.EqualJitter directly with
// its own function value, and TestRetry_DelaysGrowAndStayPositive above
// asserts only "delay > 0 and non-decreasing", which D6's formula
// guarantees for EVERY draw (real or fixed) and so cannot distinguish an
// injected jitter source from client.go's real one. This test sets a
// fixed jitter=0 through Options and asserts the exact delays D6's
// formula predicts (base/2, doubling), the shape the design's own Test
// Design calls for AC5.
func TestRetry_JitterOptionThreadedThroughToBackoff(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		h := &countingHandler{next: tgtest.ServerError(http.StatusInternalServerError)}
		srv := tgtest.New(t, h.handle)
		tr := validTransport()
		tr.RetryMaxAttempts = 4
		tr.RetryBaseDelay = 100 * time.Millisecond
		tr.RetryMaxDelay = 10 * time.Second
		c := newRetryTestClient(t, srv, func(o *Options) {
			o.Transport = tr
			o.Jitter = func() float64 { return 0 }
		})

		_, err := c.API().GetMe(context.Background())
		if err == nil {
			t.Fatal("GetMe: expected a give-up error")
		}
		times := h.timestamps()
		if len(times) != 4 {
			t.Fatalf("attempts = %d, want 4", len(times))
		}
		// D6: delay_i = d_i/2 + u*d_i/2, d_i = min(base*2^i, maxDelay).
		// With jitter fixed at 0, delay_i = d_i/2 exactly: 50ms, 100ms,
		// 200ms for i = 0, 1, 2.
		want := []time.Duration{50 * time.Millisecond, 100 * time.Millisecond, 200 * time.Millisecond}
		for i := 1; i < len(times); i++ {
			got := times[i].Sub(times[i-1])
			if got != want[i-1] {
				t.Errorf("delay[%d] = %v, want exactly %v (fixed jitter=0 threaded through Options.Jitter)", i, got, want[i-1])
			}
		}
	})
}

// TestRetry_NonDefaultFactorReachesTheCallSite is design D20's
// non-default-factor scenario: the only instrument that discriminates a
// call site passing the configured Transport.RetryFactor from one
// passing backoff.DefaultFactor, because every shipped test above runs
// at the default (2) and would stay green either way.
func TestRetry_NonDefaultFactorReachesTheCallSite(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		h := &countingHandler{next: tgtest.ServerError(http.StatusInternalServerError)}
		srv := tgtest.New(t, h.handle)
		tr := validTransport()
		tr.RetryMaxAttempts = 4
		tr.RetryBaseDelay = 100 * time.Millisecond
		tr.RetryMaxDelay = 10 * time.Second
		tr.RetryFactor = 1.5
		c := newRetryTestClient(t, srv, func(o *Options) {
			o.Transport = tr
			o.Jitter = func() float64 { return 0 }
		})

		_, err := c.API().GetMe(context.Background())
		if err == nil {
			t.Fatal("GetMe: expected a give-up error")
		}
		times := h.timestamps()
		if len(times) != 4 {
			t.Fatalf("attempts = %d, want 4", len(times))
		}
		// D20: delay_i = d_i/2, d_i = min(base*factor^i, maxDelay), jitter
		// fixed at 0. factor=1.5: 50ms, 75ms, 112.5ms for i = 0, 1, 2.
		want := []time.Duration{50 * time.Millisecond, 75 * time.Millisecond, 112500 * time.Microsecond}
		for i := 1; i < len(times); i++ {
			got := times[i].Sub(times[i-1])
			if got != want[i-1] {
				t.Errorf("delay[%d] = %v, want exactly %v (factor 1.5 reaches the call site)", i, got, want[i-1])
			}
		}
	})
}

func TestRetry_AmbiguousMakesExactlyOneAttempt(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		srv := tgtest.New(t, nil)
		h := &countingHandler{next: srv.CloseWithoutResponse()}
		srv.SetHandler(h.handle)
		c := newRetryTestClient(t, srv, nil)

		_, err := c.API().GetMe(context.Background())
		if err == nil {
			t.Fatal("GetMe: expected an error")
		}
		var tgErr *Error
		if !errors.As(err, &tgErr) {
			t.Fatalf("error %v is not a *tg.Error", err)
		}
		if !tgErr.Ambiguous {
			t.Error("Ambiguous = false, want true")
		}
		if tgErr.Attempts != 1 {
			t.Errorf("Attempts = %d, want 1 (ambiguous failures are never retried)", tgErr.Attempts)
		}
		if got := h.count(); got != 1 {
			t.Errorf("server saw %d requests, want 1", got)
		}
	})
}

func TestRetry_RetryableCasesAreRetried(t *testing.T) {
	t.Parallel()

	t.Run("never_reached", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			h := &countingHandler{next: tgtest.Success(nil)}
			srv := tgtest.New(t, h.handle)
			srv.FailNextDial()
			tr := validTransport()
			tr.RetryMaxAttempts = 2
			c := newRetryTestClient(t, srv, func(o *Options) { o.Transport = tr })

			if _, err := c.API().GetMe(context.Background()); err != nil {
				t.Fatalf("GetMe: %v", err)
			}
			if got := h.count(); got != 1 {
				t.Errorf("server saw %d requests, want 1 (the failed dial made no request)", got)
			}
		})
	})

	t.Run("429", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			h := &countingHandler{next: tgtest.TooManyRequests(0)}
			srv := tgtest.New(t, h.handle)
			tr := validTransport()
			tr.RetryMaxAttempts = 3
			c := newRetryTestClient(t, srv, func(o *Options) { o.Transport = tr })

			if _, err := c.API().GetMe(context.Background()); err == nil {
				t.Fatal("GetMe: expected a give-up error")
			}
			if got := h.count(); got <= 1 {
				t.Errorf("server saw %d requests, want more than 1", got)
			}
		})
	})

	t.Run("5xx", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			h := &countingHandler{next: tgtest.ServerError(http.StatusBadGateway)}
			srv := tgtest.New(t, h.handle)
			tr := validTransport()
			tr.RetryMaxAttempts = 3
			c := newRetryTestClient(t, srv, func(o *Options) { o.Transport = tr })

			if _, err := c.API().GetMe(context.Background()); err == nil {
				t.Fatal("GetMe: expected a give-up error")
			}
			if got := h.count(); got <= 1 {
				t.Errorf("server saw %d requests, want more than 1", got)
			}
		})
	})
}

func TestRetry_GiveUpFieldsByField(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		srv := tgtest.New(t, tgtest.TooManyRequests(1))
		tr := validTransport()
		tr.RetryMaxAttempts = 2
		c := newRetryTestClient(t, srv, func(o *Options) { o.Transport = tr })

		_, err := c.API().GetMe(context.Background())
		if err == nil {
			t.Fatal("GetMe: expected a give-up error")
		}
		var tgErr *Error
		if !errors.As(err, &tgErr) {
			t.Fatalf("error %v is not a *tg.Error", err)
		}
		if tgErr.Method != "getMe" {
			t.Errorf("Method = %q, want %q", tgErr.Method, "getMe")
		}
		if tgErr.StatusCode != http.StatusTooManyRequests {
			t.Errorf("StatusCode = %d, want 429", tgErr.StatusCode)
		}
		if tgErr.Attempts != 2 {
			t.Errorf("Attempts = %d, want 2", tgErr.Attempts)
		}
		if tgErr.RetryAfter != time.Second {
			t.Errorf("RetryAfter = %v, want 1s", tgErr.RetryAfter)
		}
		if tgErr.Ambiguous {
			t.Error("Ambiguous = true, want false (429 is never ambiguous)")
		}
	})
}

func TestRetry_DeadlineRefusalInsteadOfSleep(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		srv := tgtest.New(t, tgtest.TooManyRequests(100))
		tr := validTransport()
		tr.RetryMaxAttempts = 3
		c := newRetryTestClient(t, srv, func(o *Options) { o.Transport = tr })

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		start := time.Now()
		_, err := c.API().GetMe(ctx)
		elapsed := time.Since(start)
		if err == nil {
			t.Fatal("GetMe: expected an error")
		}
		// The refusal must fire before the wait, not after sleeping it out
		// — synctest's virtual clock makes "immediately" exact rather than
		// a loose bound: a mutant that disables the pre-attempt deadline
		// check sleeps out the full remaining 5s budget instead.
		if elapsed != 0 {
			t.Errorf("elapsed = %v, want 0 (must refuse immediately instead of sleeping toward the 100s retry_after)", elapsed)
		}
		var tgErr *Error
		if !errors.As(err, &tgErr) {
			t.Fatalf("error %v is not a *tg.Error", err)
		}
		if tgErr.RetryAfter != 100*time.Second {
			t.Errorf("RetryAfter = %v, want 100s (still carried even though not honoured)", tgErr.RetryAfter)
		}
		// The retained cause must be the 429's retry-after classification,
		// never a context error — a mutant that sleeps out the wait
		// instead of refusing pre-attempt swaps this for
		// context.DeadlineExceeded.
		if errors.Is(tgErr.Err, context.DeadlineExceeded) {
			t.Errorf("Err = %v, must not be a context deadline error — the 429 retry-after cause must be retained", tgErr.Err)
		}
		if tgErr.Err == nil || !strings.Contains(tgErr.Err.Error(), "retry after: 100") {
			t.Errorf("Err = %v, want it to carry the 429's retry after: 100", tgErr.Err)
		}
	})
}

// TestCaller_AttemptTimeoutAbandonsAttempt is design D10's AttemptTimeout
// safety valve (docs/DESIGN.md §12.2's wedged-instance incident): a single
// HTTP attempt that outruns AttemptTimeout is abandoned at that timeout,
// not at the caller's own (much longer-lived) context deadline. This test
// deliberately runs in real time rather than under synctest: the fake
// handler's delay outlives the aborted attempt (net/http never interrupts
// a handler mid-flight just because the client gave up), and synctest
// requires every bubble goroutine to finish or be durably blocked before
// the bubble's root returns — a still-sleeping handler goroutine trips
// that "deadlock" check even though nothing is actually deadlocked.
func TestCaller_AttemptTimeoutAbandonsAttempt(t *testing.T) {
	t.Parallel()
	srv := tgtest.New(t, tgtest.Delayed(2*time.Second, tgtest.Success(nil)))
	tr := validTransport()
	tr.AttemptTimeout = 50 * time.Millisecond
	tr.RetryMaxAttempts = 1
	c := newRetryTestClient(t, srv, func(o *Options) { o.Transport = tr })

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	start := time.Now()
	_, err := c.API().GetMe(ctx)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("GetMe: expected an error")
	}
	if elapsed >= time.Second {
		t.Errorf("elapsed = %v, want well under 1s (AttemptTimeout is 50ms) — the attempt must be abandoned "+
			"at AttemptTimeout, well short of the handler's 2s delay and the caller's own 1-minute deadline", elapsed)
	}
	if ctx.Err() != nil {
		t.Errorf("caller's own context Err() = %v, want nil — the call's own deadline must still be live "+
			"when AttemptTimeout is what ended the attempt", ctx.Err())
	}
}

func TestRetry_CancellationAtEveryWaitingSite(t *testing.T) {
	t.Parallel()

	t.Run("backoff_wait", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			srv := tgtest.New(t, tgtest.ServerError(http.StatusInternalServerError))
			tr := validTransport()
			tr.RetryMaxAttempts = 5
			tr.RetryBaseDelay = time.Hour
			tr.RetryMaxDelay = time.Hour
			c := newRetryTestClient(t, srv, func(o *Options) { o.Transport = tr })

			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				time.Sleep(10 * time.Millisecond)
				cancel()
			}()
			start := time.Now()
			_, err := c.API().GetMe(ctx)
			elapsed := time.Since(start)
			if err == nil {
				t.Fatal("GetMe: expected an error")
			}
			if elapsed >= time.Hour {
				t.Errorf("elapsed = %v, must return promptly on cancellation rather than after the full backoff", elapsed)
			}
		})
	})

	t.Run("retry_after_wait", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			srv := tgtest.New(t, tgtest.TooManyRequests(3600))
			c := newRetryTestClient(t, srv, nil)

			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				time.Sleep(10 * time.Millisecond)
				cancel()
			}()
			start := time.Now()
			_, err := c.API().GetMe(ctx)
			elapsed := time.Since(start)
			if err == nil {
				t.Fatal("GetMe: expected an error")
			}
			if elapsed >= time.Hour {
				t.Errorf("elapsed = %v, must return promptly on cancellation rather than after the full retry_after", elapsed)
			}
		})
	})

	t.Run("limiter_wait", func(t *testing.T) {
		t.Parallel()
		synctest.Test(t, func(t *testing.T) {
			srv := tgtest.New(t, tgtest.Success(nil))
			tr := validTransport()
			c := newRetryTestClient(t, srv, func(o *Options) {
				o.Transport = tr
				o.Transport.Limits.Other.Global.Count = 1
				o.Transport.Limits.Other.Global.Per = time.Hour
			})
			if _, err := c.API().GetMe(context.Background()); err != nil {
				t.Fatalf("GetMe (1st): %v", err)
			}

			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				time.Sleep(10 * time.Millisecond)
				cancel()
			}()
			start := time.Now()
			_, err := c.API().GetMe(ctx)
			elapsed := time.Since(start)
			if err == nil {
				t.Fatal("GetMe (2nd): expected an error")
			}
			if elapsed >= time.Hour {
				t.Errorf("elapsed = %v, must return promptly on cancellation rather than after the full limiter wait", elapsed)
			}
		})
	})
}

func TestRetry_AttemptCap(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		h := &countingHandler{next: tgtest.ServerError(http.StatusInternalServerError)}
		srv := tgtest.New(t, h.handle)
		tr := validTransport()
		tr.RetryMaxAttempts = 5
		c := newRetryTestClient(t, srv, func(o *Options) { o.Transport = tr })

		_, err := c.API().GetMe(context.Background())
		if err == nil {
			t.Fatal("GetMe: expected a give-up error")
		}
		if got := h.count(); got != 5 {
			t.Errorf("attempts = %d, want exactly 5", got)
		}
		var tgErr *Error
		if !errors.As(err, &tgErr) {
			t.Fatalf("error %v is not a *tg.Error", err)
		}
		if tgErr.Attempts != 5 {
			t.Errorf("Error.Attempts = %d, want 5", tgErr.Attempts)
		}
	})
}

func TestRetry_TokenAbsentFromRenderedError(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		srv := tgtest.New(t, tgtest.Success(nil))
		srv.FailNextDial()
		tr := validTransport()
		tr.RetryMaxAttempts = 1
		c := newRetryTestClient(t, srv, func(o *Options) { o.Transport = tr })

		_, err := c.API().GetMe(context.Background())
		if err == nil {
			t.Fatal("GetMe: expected an error from the failed dial")
		}
		rendered := err.Error()
		if strings.Contains(rendered, tgtest.Token) {
			t.Errorf("rendered error %q contains the bot token", rendered)
		}
	})
}

// recordingObserver collects every Observation reported to it.
type recordingObserver struct {
	mu  sync.Mutex
	obs []Observation
}

func (o *recordingObserver) ObserveCall(ob Observation) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.obs = append(o.obs, ob)
}

func (o *recordingObserver) all() []Observation {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]Observation(nil), o.obs...)
}

func TestRetry_Observation(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		obs := &recordingObserver{}
		srv := tgtest.New(t, tgtest.Success(nil))
		c := newRetryTestClient(t, srv, func(o *Options) { o.Observer = obs })

		// A plain success.
		if _, err := c.API().GetMe(context.Background()); err != nil {
			t.Fatalf("GetMe: %v", err)
		}

		// A retried success.
		first := true
		srv.SetHandler(func(w http.ResponseWriter, r *http.Request) {
			if first {
				first = false
				tgtest.TooManyRequests(1)(w, r)
				return
			}
			tgtest.Success(nil)(w, r)
		})
		if _, err := c.API().GetMe(context.Background()); err != nil {
			t.Fatalf("GetMe (retried): %v", err)
		}

		// A give-up.
		srv.SetHandler(tgtest.ServerError(http.StatusInternalServerError))
		if _, err := c.API().GetMe(context.Background()); err == nil {
			t.Fatal("GetMe (give-up): expected an error")
		}

		got := obs.all()
		if len(got) != 3 {
			t.Fatalf("observations = %d, want 3", len(got))
		}
		// Pinned exact values, not loose bounds — finding 5's whole
		// point is that a bound like "Retries > 0" stays green even when
		// every attempts-1 call site is silently changed to attempts.
		if got[0].Retries != 0 || got[0].RateLimited || got[0].StatusCode != http.StatusOK {
			t.Errorf("success observation = %+v, want Retries 0, RateLimited false, StatusCode 200", got[0])
		}
		if got[1].Retries != 1 || !got[1].RateLimited || got[1].StatusCode != http.StatusOK {
			t.Errorf("retried-success observation = %+v, want Retries 1, RateLimited true, StatusCode 200", got[1])
		}
		// validTransport's RetryMaxAttempts is 3: three failed 500
		// attempts, so exactly 2 retries beyond the first.
		if got[2].Retries != 2 || got[2].RateLimited || got[2].StatusCode != http.StatusInternalServerError {
			t.Errorf("give-up observation = %+v, want Retries 2, RateLimited false, StatusCode 500", got[2])
		}
		for i, ob := range got {
			if ob.Method != "getMe" {
				t.Errorf("observation[%d].Method = %q, want getMe", i, ob.Method)
			}
			if ob.Latency < 0 {
				t.Errorf("observation[%d].Latency = %v, want >= 0", i, ob.Latency)
			}
		}
		// The retried-success and give-up calls both waited (retry_after
		// / backoff between attempts); their Latency must reflect that,
		// unlike the plain first success.
		if got[1].Latency <= 0 {
			t.Errorf("retried-success observation.Latency = %v, want > 0 (a retry_after wait happened)", got[1].Latency)
		}
		if got[2].Latency <= 0 {
			t.Errorf("give-up observation.Latency = %v, want > 0 (backoff waits happened before giving up)", got[2].Latency)
		}
	})
}

// TestClassifyAttempt_NonRetryableHTTPStatusIsTerminalNotAmbiguous is
// self-review round 5's coverage sweep: classifyAttempt's
// "httpStatus != 0" case (a real, non-429, non-5xx HTTP response — e.g.
// a 400 Bad Request) had zero coverage hits. It asserts design D5's
// table directly: Telegram answered, so the outcome is known — not
// ambiguous — and not retryable ("never on doubt" applies only when the
// outcome is unknown, which this is not).
func TestClassifyAttempt_NonRetryableHTTPStatusIsTerminalNotAmbiguous(t *testing.T) {
	t.Parallel()
	got := classifyAttempt(http.StatusBadRequest, &ta.Response{Ok: false}, true)
	want := outcome{}
	if got != want {
		t.Errorf("classifyAttempt(400, ok:false, wrote:true) = %+v, want %+v (a real non-5xx/429 response is terminal, not ambiguous, not retryable)", got, want)
	}
}

// TestSanitizeErr_NilErrorReturnsNil is self-review round 5's coverage
// sweep: sanitizeErr's nil-err guard had zero coverage hits — every
// existing caller passes a non-nil cause. The doc comment promises "A
// nil err returns nil" (design D8); this checks that promise directly.
func TestSanitizeErr_NilErrorReturnsNil(t *testing.T) {
	t.Parallel()
	replacer := strings.NewReplacer(tgtest.Token, "[REDACTED_TOKEN]")
	if got := sanitizeErr(nil, replacer); got != nil {
		t.Errorf("sanitizeErr(nil, ...) = %v, want nil", got)
	}
}

// TestSanitizeErr_URLErrorWithNilUnderlyingErrDoesNotPanic is the fix for
// the panic this coverage sweep surfaced: retry.go's "cause == nil"
// fallback (uerr.Unwrap() returning nil because uerr.Err is nil) used to
// call uerr.Err.Error() on that same nil error interface, which panics
// (nil pointer dereference) rather than falling back to any message.
// Confirmed against the pre-fix code with a scratch test constructing
// &url.Error{Err: nil} and calling sanitizeErr on it directly: `panic:
// runtime error: invalid memory address or nil pointer dereference` at
// retry.go:121 (goroutine trace through sanitizeErr), deleted after
// confirming and before writing this permanent test. This test asserts
// the fixed behaviour: no panic, and the rendered/stored cause names the
// operation (uerr.Op) without reintroducing the URL — D8 requires the URL
// dropped, not merely token-scrubbed, and there is no real underlying
// cause to preserve when uerr.Err is nil.
func TestSanitizeErr_URLErrorWithNilUnderlyingErrDoesNotPanic(t *testing.T) {
	t.Parallel()
	replacer := strings.NewReplacer(tgtest.Token, "[REDACTED_TOKEN]")
	uerr := &url.Error{Op: "Post", URL: "http://bot-api.invalid/bot" + tgtest.Token + "/getMe", Err: nil}

	got := sanitizeErr(uerr, replacer)
	if got == nil {
		t.Fatal("sanitizeErr: got nil")
	}
	rendered := got.Error()
	if strings.Contains(rendered, uerr.URL) {
		t.Errorf("sanitizeErr: rendered %q still contains the URL %q", rendered, uerr.URL)
	}
	if strings.Contains(rendered, tgtest.Token) {
		t.Errorf("sanitizeErr: rendered %q still contains the token", rendered)
	}
	if !strings.Contains(rendered, uerr.Op) {
		t.Errorf("sanitizeErr: rendered %q, want it to name the operation %q", rendered, uerr.Op)
	}
}

// TestSanitizeErr_UnwrapsURLErrorAndDropsURL asserts D8's primary
// token-leak defence directly (finding 6 — deleting the errors.As
// unwrap block left the suite green, because
// TestRetry_TokenAbsentFromRenderedError only checks the token
// substring, which the trailing strings.Replacer alone already
// satisfies). Two things must both hold: the stored cause is the
// UNWRAPPED value beneath the *url.Error, not the *url.Error itself
// (checked via errors.Is), and the rendered string must not contain the
// URL at all — not merely have its token scrubbed.
func TestSanitizeErr_UnwrapsURLErrorAndDropsURL(t *testing.T) {
	t.Parallel()
	replacer := strings.NewReplacer(tgtest.Token, "[REDACTED_TOKEN]")
	cause := errors.New("boom")
	uerr := &url.Error{Op: "Post", URL: "http://bot-api.invalid/bot" + tgtest.Token + "/getMe", Err: cause}

	got := sanitizeErr(uerr, replacer)
	if got == nil {
		t.Fatal("sanitizeErr: got nil")
	}
	if !errors.Is(got, cause) {
		t.Errorf("sanitizeErr: errors.Is(got, cause) = false, want true (Unwrap must return the *url.Error's own unwrapped cause)")
	}
	if errors.Is(got, error(uerr)) {
		t.Errorf("sanitizeErr: errors.Is(got, uerr) = true, want false (the *url.Error itself must not be the stored cause)")
	}
	rendered := got.Error()
	if strings.Contains(rendered, uerr.URL) {
		t.Errorf("sanitizeErr: rendered %q still contains the URL %q", rendered, uerr.URL)
	}
	if rendered != cause.Error() {
		t.Errorf("sanitizeErr: rendered = %q, want exactly %q (the URL must be DROPPED, not merely scrubbed)", rendered, cause.Error())
	}
}

// TestBackoffDelay_JitterBoundsExactly asserts D6's equal-jitter formula
// directly at the two ends of the jitter draw (finding 7 — no test
// exercised backoff.EqualJitter or jitter directly; returning the full
// delay with no jitter at all left the suite green).
func TestBackoffDelay_JitterBoundsExactly(t *testing.T) {
	t.Parallel()
	const base = 100 * time.Millisecond
	const maxDelay = 10 * time.Second

	if got := backoff.EqualJitter(0, base, maxDelay, 2, func() float64 { return 0 }); got != base/2 {
		t.Errorf("backoff.EqualJitter(attempt=0, jitter=0) = %v, want %v (half, no jitter added)", got, base/2)
	}
	if got := backoff.EqualJitter(0, base, maxDelay, 2, func() float64 { return 1 }); got != base {
		t.Errorf("backoff.EqualJitter(attempt=0, jitter=1) = %v, want %v (half plus the full other half)", got, base)
	}

	// Self-review round 5 finding 1: backoff.EqualJitter's RetryMaxDelay
	// enforcement had never been exercised at all — deleting it left the
	// suite green. (The finding was written against the doubling loop's
	// two enforcement branches, an in-loop early break and a post-loop
	// clamp; design D20 replaced that loop with math.Pow and a single
	// clamp before the time.Duration conversion, so both rows below now
	// return through the same one.) These two rows measure the capped
	// region at the shipped defaults (RetryBaseDelay 500ms, RetryMaxDelay
	// 30s) that design D10's rationale states in words: "30s caps the
	// scale so a higher configured attempt count cannot grow the wait
	// without bound." Verified against the shipped tree (GREEN) and
	// against a scratch deletion of the enforcement (RED: "got 16s want
	// 15s" at attempt=6, "got 8m32s want 30s" at attempt=10) before being
	// added here.
	const shippedBase = 500 * time.Millisecond
	const shippedMax = 30 * time.Second
	if got := backoff.EqualJitter(6, shippedBase, shippedMax, 2, func() float64 { return 0 }); got != 15*time.Second {
		t.Errorf("backoff.EqualJitter(attempt=6, jitter=0) = %v, want %v (clamped: 500ms*2^6=32s >= max, so the ceiling is returned unconverted, half=15s)", got, 15*time.Second)
	}
	if got := backoff.EqualJitter(10, shippedBase, shippedMax, 2, func() float64 { return 1 }); got != shippedMax {
		t.Errorf("backoff.EqualJitter(attempt=10, jitter=1) = %v, want %v (clamped: 500ms*2^10=512s >= max, so the ceiling is returned unconverted)", got, shippedMax)
	}

	// Self-review round 1 finding R1-8 instrumented the two rows above
	// against a verbatim copy of Exponential, to attribute each to one of
	// the doubling loop's two exit branches. Design D20 deleted that loop
	// — the ramp is float64(base)*math.Pow(factor, attempt) with one
	// clamp before the time.Duration conversion — so the two rows no
	// longer discriminate two branches; they pin that single clamp at two
	// magnitudes, just past the ceiling (32s) and far past it (512s).
	// New rejects
	// Transport.RetryMaxDelay < Transport.RetryBaseDelay
	// (client.go:116-117), so base > maxDelay cannot occur through the
	// public Client constructor; backoff.EqualJitter is still called
	// directly by every other case in this test. This row does not exist
	// for coverage — deleting it leaves the clamp's block at a non-zero
	// count, measured — it exists to pin the VALUE the clamp returns when
	// base alone already exceeds maxDelay, which no other case asserts.
	if got := backoff.EqualJitter(0, 40*time.Second, shippedMax, 2, func() float64 { return 0 }); got != shippedMax/2 {
		t.Errorf("backoff.EqualJitter(base>maxDelay, attempt=0, jitter=0) = %v, want %v (clamped: base alone already exceeds maxDelay)", got, shippedMax/2)
	}
}
