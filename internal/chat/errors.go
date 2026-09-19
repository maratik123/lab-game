package chat

import "errors"

// ErrNotAChat is returned when the given owner id does not name a chat.
var ErrNotAChat = errors.New("chat: owner is not a chat")

// ErrNotAPlayer is returned when the given owner id does not name a
// player.
var ErrNotAPlayer = errors.New("chat: owner is not a player")
