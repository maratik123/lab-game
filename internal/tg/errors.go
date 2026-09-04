package tg

import (
	"fmt"
	"time"
)

// Error is the one typed error this package's caller ever returns for a
// failed outbound call — carrying everything #43 needs to branch on
// "retry later at time T" (RetryAfter > 0), "this chat is gone"
// (StatusCode plus Description), and "unknown — do not re-send"
// (Ambiguous) (design D8). Err is sanitised at construction time so that
// this value is safe to render or log for the rest of its life: the bot
// token never appears in Err, in Error()'s rendering, or in any
// instrumentation observation (design D8, AC26).
type Error struct {
	// Method is the Bot API method name — e.g. "sendMessage" — never the
	// request URL and never the token.
	Method string
	// StatusCode is the last HTTP status code received, or 0 when no
	// response was ever received.
	StatusCode int
	// Description is Telegram's own error description, empty when none
	// was received.
	Description string
	// RetryAfter is the wait Telegram asked for, converted from
	// retry_after seconds; 0 when the last failure carried none.
	RetryAfter time.Duration
	// Attempts is how many attempts this call made before returning Error.
	Attempts int
	// Ambiguous is true when the last attempt's outcome is unknown — the
	// request may or may not have reached Telegram — so the caller must
	// not retry or re-send (design D5).
	Ambiguous bool
	// Err is the underlying cause, already token-sanitised (design D8).
	Err error
}

// Error renders e as a single line naming the method, the last status
// code, the attempt count, the ambiguity flag and the underlying cause —
// never the request URL, never the bot token.
func (e *Error) Error() string {
	return fmt.Sprintf("tg: %s: status=%d attempts=%d ambiguous=%t: %v",
		e.Method, e.StatusCode, e.Attempts, e.Ambiguous, e.Err)
}

// Unwrap returns e.Err, so errors.Is/errors.As see through to the
// underlying cause — e.g. errors.Is(err, context.DeadlineExceeded).
func (e *Error) Unwrap() error {
	return e.Err
}
