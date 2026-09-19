package onboard

import (
	"errors"
	"net/url"
	"regexp"
	"testing"
)

// startPayloadCharsetRe is the Bot API's own documented deep-link
// payload character class: A-Z, a-z, 0-9, _ and -.
var startPayloadCharsetRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

const startPayloadMaxLen = 64

func TestChatStart_negativeSupergroupIDRoundTrips(t *testing.T) {
	t.Parallel()

	const chatTelegramID = int64(-1001234567890)
	payload := EncodeChatStart(chatTelegramID)

	got, err := ParseChatStart(payload)
	if err != nil {
		t.Fatalf("ParseChatStart(%q): %v", payload, err)
	}
	if got != chatTelegramID {
		t.Fatalf("ParseChatStart(%q) = %d, want %d", payload, got, chatTelegramID)
	}
}

func TestChatStart_positiveIDRoundTrips(t *testing.T) {
	t.Parallel()

	const chatTelegramID = int64(555)
	payload := EncodeChatStart(chatTelegramID)

	got, err := ParseChatStart(payload)
	if err != nil {
		t.Fatalf("ParseChatStart(%q): %v", payload, err)
	}
	if got != chatTelegramID {
		t.Fatalf("ParseChatStart(%q) = %d, want %d", payload, got, chatTelegramID)
	}
}

func TestEncodeChatStart_withinBotAPICharsetAndLength(t *testing.T) {
	t.Parallel()

	for _, chatTelegramID := range []int64{0, 1, -1, -1001234567890, 9223372036854775807, -9223372036854775808} {
		payload := EncodeChatStart(chatTelegramID)
		if !startPayloadCharsetRe.MatchString(payload) {
			t.Errorf("EncodeChatStart(%d) = %q, contains a character outside the documented charset", chatTelegramID, payload)
		}
		if len(payload) > startPayloadMaxLen {
			t.Errorf("EncodeChatStart(%d) = %q, length %d exceeds the documented %d-character budget", chatTelegramID, payload, len(payload), startPayloadMaxLen)
		}
	}
}

func TestParseChatStart_refusals(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		payload string
		wantErr error
	}{
		{"empty", "", ErrEmptyPayload},
		{"wrong_prefix", "x123", ErrUnknownPayloadPrefix},
		{"remainder_not_an_integer", "cabc", ErrMalformedPayload},
		{"trailing_text_after_integer", "c123abc", ErrMalformedPayload},
		{"prefix_only_no_remainder", "c", ErrMalformedPayload},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseChatStart(tc.payload)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("ParseChatStart(%q) = %v, want it to wrap %v", tc.payload, err, tc.wantErr)
			}
		})
	}
}

func TestChatStartLink_hostPathAndQuery(t *testing.T) {
	t.Parallel()

	const botUsername = "lab_game_bot"
	const chatTelegramID = int64(-1009876543210)

	link, err := ChatStartLink(botUsername, chatTelegramID)
	if err != nil {
		t.Fatalf("ChatStartLink: %v", err)
	}

	u, err := url.Parse(link)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", link, err)
	}
	if u.Scheme != "https" {
		t.Errorf("scheme = %q, want https", u.Scheme)
	}
	if u.Host != "t.me" {
		t.Errorf("host = %q, want t.me", u.Host)
	}
	if u.Path != "/"+botUsername {
		t.Errorf("path = %q, want %q", u.Path, "/"+botUsername)
	}
	gotPayload := u.Query().Get("start")
	wantPayload := EncodeChatStart(chatTelegramID)
	if gotPayload != wantPayload {
		t.Errorf("start query param = %q, want %q", gotPayload, wantPayload)
	}
}

func TestChatStartLink_emptyUsernameRefused(t *testing.T) {
	t.Parallel()

	if _, err := ChatStartLink("", 555); !errors.Is(err, ErrEmptyUsername) {
		t.Fatalf("ChatStartLink(\"\", 555) = %v, want ErrEmptyUsername", err)
	}
}
