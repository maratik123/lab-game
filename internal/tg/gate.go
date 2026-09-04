package tg

import "context"

// ChatTarget describes what this package could determine about a call's
// destination chat, from the derivation design D4 specifies. It is three
// states, not a boolean, because the two "no chat id" branches need
// opposite treatment from an outbound Gate — a boolean would collapse
// them (design D12).
type ChatTarget int

const (
	// ChatNone means the method addresses no chat at all — getMe,
	// getUpdates, an inline-message edit. A Gate must allow these: there
	// is no destination to check, and refusing here would break the
	// update loop.
	ChatNone ChatTarget = iota
	// ChatUnknown means the method addresses a chat, but this package
	// could not read which one — a multipart request, whose body is a
	// stream rather than raw bytes (design D4). A Gate must refuse these:
	// an unverifiable destination is exactly what an allowlist exists to
	// stop.
	ChatUnknown
	// ChatKnown means Key holds the destination chat id (as the raw JSON
	// token from the request body's chat_id field). A Gate checks Key.
	ChatKnown
)

// String renders t's name, for logs and test failure messages.
func (t ChatTarget) String() string {
	switch t {
	case ChatNone:
		return "ChatNone"
	case ChatUnknown:
		return "ChatUnknown"
	case ChatKnown:
		return "ChatKnown"
	default:
		return "ChatUnknown"
	}
}

// ChatRef names a call's destination chat, or states that none exists or
// could not be read (design D4, D12).
type ChatRef struct {
	// Key is the destination chat id's raw JSON token (a chat_id may be a
	// @channelusername string rather than a number) — meaningful only
	// when Target is ChatKnown.
	Key string
	// Target is what this package could determine about the destination.
	Target ChatTarget
}

// Call describes one outbound Bot API call for the purposes of the
// outbound Gate and the rate limiter — both of which need the method
// name, its MethodClass and its destination chat (design D4, D9, D12).
type Call struct {
	// Method is the Bot API method name, e.g. "sendMessage".
	Method string
	// Class is Method's MethodClass (design D4).
	Class MethodClass
	// Chat is Method's destination, or the absence of one (design D4).
	Chat ChatRef
}

// Gate is the outbound seam #22 installs its ALLOWED_CHAT_IDS allowlist
// into (design D12). It is consulted before any attempt and before any
// limiter wait, so a refused call costs no allowance. AllowCall returning
// a non-nil error refuses the call; this package wraps that error in its
// own typed Error with Attempts 0, StatusCode 0 and Ambiguous false.
//
// This package installs no allowlist itself — the obligation here is
// negative and structural: the seam exists, it is reachable from outside
// the package via Options.Gate, and no exported API lets a caller issue
// an outbound call that bypasses it (design D12, AC27).
type Gate interface {
	// AllowCall reports whether call may proceed, given ctx.
	AllowCall(ctx context.Context, call Call) error
}
