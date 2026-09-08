package tg

import "strings"

// MethodClass groups Bot API methods for rate-limiting purposes:
// ClassMessage carries the published send-rate figures, ClassEdit
// and ClassOther exist as their own keys so an operator can bound them
// later with no code change, and both default to unbounded because no
// published figure supports a default bound.
type MethodClass int

const (
	// ClassMessage is every method that delivers, copies or forwards a
	// message (the send*/copyMessage*/forwardMessage* prefixes).
	ClassMessage MethodClass = iota
	// ClassEdit is every method that edits or deletes a message, or stops
	// a live location or a poll (the editMessage*/deleteMessage*
	// prefixes plus the two exact names).
	ClassEdit
	// ClassOther is every other Bot API method.
	ClassOther
)

// String renders c's name, for logs and test failure messages.
func (c MethodClass) String() string {
	switch c {
	case ClassMessage:
		return "ClassMessage"
	case ClassEdit:
		return "ClassEdit"
	case ClassOther:
		return "ClassOther"
	default:
		return "ClassOther"
	}
}

// classifyMethod maps a Bot API method name to its MethodClass by prefix.
// The prefix form is deliberately
// fail-safe: a method added later that delivers a message (any send*
// name) is throttled by default rather than escaping the limiter, and the
// edit/delete side is spelled out to the "Message" segment on purpose —
// the bare prefixes would also sweep in editChatInviteLink,
// editForumTopic, deleteWebhook, deleteMyCommands and more, which are not
// message operations.
func classifyMethod(name string) MethodClass {
	switch {
	case strings.HasPrefix(name, "send"),
		strings.HasPrefix(name, "copyMessage"),
		strings.HasPrefix(name, "forwardMessage"):
		return ClassMessage
	case strings.HasPrefix(name, "editMessage"),
		strings.HasPrefix(name, "editEphemeralMessage"),
		strings.HasPrefix(name, "deleteMessage"),
		strings.HasPrefix(name, "deleteEphemeralMessage"),
		name == "stopMessageLiveLocation",
		name == "stopPoll":
		return ClassEdit
	default:
		return ClassOther
	}
}
