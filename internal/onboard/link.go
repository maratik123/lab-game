package onboard

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// chatStartPrefix namespaces a chat's own deep-link payload within the
// /start command's single string argument. A later link kind (a raid
// invite, a referral) reserves its own prefix and its own parser branch
// — never a re-reading of this one.
const chatStartPrefix = "c"

// EncodeChatStart builds the /start payload naming chatTelegramID — a
// group chat's own telegram id, which may be negative for a supergroup.
func EncodeChatStart(chatTelegramID int64) string {
	return chatStartPrefix + strconv.FormatInt(chatTelegramID, 10)
}

// ParseChatStart parses payload as a chat's own /start payload,
// returning the chat's telegram id. Refuses an empty payload
// (ErrEmptyPayload), a payload whose prefix names no recognised
// namespace (ErrUnknownPayloadPrefix), and a payload whose remainder is
// not a base-10 int64 or carries trailing text after one
// (ErrMalformedPayload) — strconv.ParseInt itself is what refuses
// trailing non-digit content, since it requires the whole remainder to
// parse.
func ParseChatStart(payload string) (int64, error) {
	if payload == "" {
		return 0, ErrEmptyPayload
	}
	rest, ok := strings.CutPrefix(payload, chatStartPrefix)
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrUnknownPayloadPrefix, payload)
	}
	chatTelegramID, err := strconv.ParseInt(rest, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q: %w", ErrMalformedPayload, payload, err)
	}
	return chatTelegramID, nil
}

// ChatStartLink builds the Telegram deep link that, followed and
// started, records the presser's membership of the chat named by
// chatTelegramID. botUsername is the caller's own responsibility to
// supply (see the package doc comment); an empty one is refused with
// ErrEmptyUsername.
func ChatStartLink(botUsername string, chatTelegramID int64) (string, error) {
	if botUsername == "" {
		return "", ErrEmptyUsername
	}
	u := url.URL{
		Scheme: "https",
		Host:   "t.me",
		Path:   "/" + botUsername,
	}
	q := url.Values{}
	q.Set("start", EncodeChatStart(chatTelegramID))
	u.RawQuery = q.Encode()
	return u.String(), nil
}
