// Package onboard owns the front door: a chat's own deep link, and the
// two update handlers that turn a chat-membership-changed update into a
// chat's arrival and a private /start into a player's — recording that
// player's membership of the chat named by the link, when one was
// followed.
//
// The package's deep-link codec and builder are pure: no database, no
// Telegram call. ChatStartLink takes the bot's own username as an
// argument rather than resolving it itself — the Telegram client
// library's own username accessor resolves through a fresh network call
// and reports the empty string on failure, which would turn a Bot API
// outage into a silently dead link; resolving it at process start-up
// would add a synchronous dependency to the all-fatal start-up order for
// a value this package never otherwise needs. The caller that sends a
// notification carrying this link supplies the username it already
// holds.
package onboard
