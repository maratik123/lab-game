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
// exercised backoffDelay or jitter directly; returning the full delay
// with no jitter at all left the suite green).
func TestBackoffDelay_JitterBoundsExactly(t *testing.T) {
	t.Parallel()
	const base = 100 * time.Millisecond
	const maxDelay = 10 * time.Second

	if got := backoffDelay(base, maxDelay, 0, func() float64 { return 0 }); got != base/2 {
		t.Errorf("backoffDelay(attempt=0, jitter=0) = %v, want %v (half, no jitter added)", got, base/2)
	}
	if got := backoffDelay(base, maxDelay, 0, func() float64 { return 1 }); got != base {
		t.Errorf("backoffDelay(attempt=0, jitter=1) = %v, want %v (half plus the full other half)", got, base)
	}
}
