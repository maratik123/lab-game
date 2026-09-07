package tg

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"time"

	ta "github.com/mymmrac/telego/telegoapi"
)

// defaultJitter is the default source for design D6's equal-jitter
// backoff — a spread over the retry interval, not a security decision, so
// math/rand/v2 (not crypto/rand) is the right choice.
//
//nolint:gosec // G404: jitter is a backoff spread, not a security decision (design D6).
func defaultJitter() float64 {
	return rand.Float64()
}

// outcome classifies one attempt per design D5's table: whether it may be
// retried, whether it is ambiguous (the request may have reached
// Telegram, but the outcome is unknown), and the retry_after wait when the
// attempt was a 429 that carried one.
type outcome struct {
	retryable  bool
	ambiguous  bool
	retryAfter time.Duration
}

// classifyAttempt reports outcome for one attempt, given the real HTTP
// status code received (0 when none), the decoded envelope (nil when
// none), and whether httptrace observed the request being fully written
// (design D5). Any evidence outside the table's rows classifies as
// ambiguous and terminal only when nothing was written; otherwise a
// response of any other shape is terminal, not ambiguous — Telegram
// answered, so the outcome is known even when it is a failure.
func classifyAttempt(httpStatus int, resp *ta.Response, wrote bool) outcome {
	switch {
	case httpStatus == http.StatusTooManyRequests:
		if resp != nil && resp.Error != nil && resp.Parameters != nil && resp.Parameters.RetryAfter > 0 {
			return outcome{retryable: true, retryAfter: time.Duration(resp.Parameters.RetryAfter) * time.Second}
		}
		return outcome{retryable: true}
	case httpStatus >= http.StatusInternalServerError:
		return outcome{retryable: true}
	case httpStatus != 0:
		// Any other HTTP response — success is handled by the caller
		// before classifyAttempt is reached, so this is a non-2xx or
		// ok:false response. Telegram answered: not ambiguous, not
		// retryable (design D5's "never on doubt" applies only when the
		// outcome is unknown).
		return outcome{}
	case wrote:
		// No response, but the request was fully written: ambiguous and
		// terminal (design D5).
		return outcome{ambiguous: true}
	default:
		// No response, and no evidence the request was ever written:
		// provably never reached Telegram, so it is safely retryable.
		return outcome{retryable: true}
	}
}

// sanitizedError wraps an error whose rendering is guaranteed
// token-scrubbed, while still exposing the original cause through Unwrap
// for errors.Is/errors.As (design D8). rendered is computed once, at
// construction.
type sanitizedError struct {
	rendered string
	cause    error
}

func (e *sanitizedError) Error() string { return e.rendered }
func (e *sanitizedError) Unwrap() error { return e.cause }

// sanitizeErr builds a *sanitizedError from err (design D8's
// "sanitised at construction, not a test obligation"):
//   - When err's chain carries a *url.Error, its stored cause is the
//     unwrapped value beneath it — never the *url.Error itself, which
//     net/http formats with the full request URL (and therefore the bot
//     token) embedded (design D8).
//   - Every string this package renders is additionally passed through
//     replacer, a defense-in-depth layer that does not depend on having
//     predicted every path a token could take.
//
// A nil err returns nil.
func sanitizeErr(err error, replacer *strings.Replacer) error {
	if err == nil {
		return nil
	}
	cause := err
	var uerr *url.Error
	if errors.As(err, &uerr) {
		cause = uerr.Unwrap()
		if cause == nil {
			// *url.Error permits a nil Err (e.g. constructed directly
			// rather than by net/http), so uerr.Unwrap() returning nil
			// does not mean "nothing to report" — it means the wrapped
			// cause itself is absent. Rendering uerr.Error() here would
			// reintroduce the request URL (and therefore the bot token)
			// that D8 requires dropped, and uerr.Err is nil so it cannot
			// be rendered anyway. Name only what IS known and safe: the
			// operation that failed.
			cause = fmt.Errorf("%s: no underlying error", uerr.Op)
		}
	}
	return &sanitizedError{rendered: replacer.Replace(cause.Error()), cause: cause}
}

// giveUpError builds the D8 typed error for a call the retry loop gives
// up on, sanitising cause via c's token replacer.
func (c *Client) giveUpError(method string, statusCode int, description string, retryAfter time.Duration, attempts int, ambiguous bool, cause error) *Error {
	return &Error{
		Method:      method,
		StatusCode:  statusCode,
		Description: description,
		RetryAfter:  retryAfter,
		Attempts:    attempts,
		Ambiguous:   ambiguous,
		Err:         sanitizeErr(cause, c.tokenReplacer),
	}
}

// observe reports one Observation to c's Observer, when configured
// (design D11) — exactly once per outbound call, including a gate
// refusal.
func (c *Client) observe(method string, latency time.Duration, statusCode int, rateLimited bool, retries int) {
	if c.observer == nil {
		return
	}
	c.observer.ObserveCall(Observation{
		Method:      method,
		Latency:     latency,
		StatusCode:  statusCode,
		RateLimited: rateLimited,
		Retries:     retries,
	})
}
