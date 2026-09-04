package tg

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/maratik123/lab-game/internal/config"
	"github.com/maratik123/lab-game/internal/tgtest"
)

// validTransport returns a config.Transport that passes New's validation
// — every retry field positive, every rate-limit class unbounded (the
// zero value), which is a legal configuration (design D9's "an unbounded
// value contributes no window").
func validTransport() config.Transport {
	return config.Transport{
		RetryMaxAttempts: 3,
		RetryBaseDelay:   500 * time.Millisecond,
		RetryMaxDelay:    30 * time.Second,
		AttemptTimeout:   30 * time.Second,
	}
}

func validOptions() Options {
	return Options{
		BaseURL:   tgtest.BaseURL,
		Token:     tgtest.Token,
		Transport: validTransport(),
	}
}

func TestNew_HappyPath(t *testing.T) {
	t.Parallel()
	c, err := New(validOptions())
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	if c.API() == nil {
		t.Fatal("API() = nil")
	}
}

func TestNew_ValidatesEachField(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		mutate    func(*Options)
		wantField string
	}{
		{"empty base URL", func(o *Options) { o.BaseURL = "" }, "BaseURL"},
		{"empty token", func(o *Options) { o.Token = "" }, "Token"},
		{"zero retry max attempts", func(o *Options) { o.Transport.RetryMaxAttempts = 0 }, "Transport.RetryMaxAttempts"},
		{"negative retry max attempts", func(o *Options) { o.Transport.RetryMaxAttempts = -1 }, "Transport.RetryMaxAttempts"},
		{"zero retry base delay", func(o *Options) { o.Transport.RetryBaseDelay = 0 }, "Transport.RetryBaseDelay"},
		{"negative retry base delay", func(o *Options) { o.Transport.RetryBaseDelay = -1 }, "Transport.RetryBaseDelay"},
		{"zero retry max delay", func(o *Options) { o.Transport.RetryMaxDelay = 0 }, "Transport.RetryMaxDelay"},
		{"retry max delay below base delay", func(o *Options) { o.Transport.RetryMaxDelay = o.Transport.RetryBaseDelay - time.Millisecond }, "Transport.RetryMaxDelay"},
		{"zero attempt timeout", func(o *Options) { o.Transport.AttemptTimeout = 0 }, "Transport.AttemptTimeout"},
		{"malformed token", func(o *Options) { o.Token = "not-a-token" }, "Token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts := validOptions()
			tc.mutate(&opts)
			_, err := New(opts)
			if err == nil {
				t.Fatal("New: expected an error, got nil")
			}
			var optErr *OptionError
			if !errors.As(err, &optErr) {
				t.Fatalf("New: error %v is not an *OptionError", err)
			}
			if optErr.Field != tc.wantField {
				t.Errorf("Field = %q, want %q", optErr.Field, tc.wantField)
			}
		})
	}
}

func TestOptionError_ErrorAndUnwrap(t *testing.T) {
	t.Parallel()
	cause := errors.New("boom")
	e := &OptionError{Field: "BaseURL", Err: cause}
	if !errors.Is(e, cause) {
		t.Errorf("errors.Is(e, cause) = false")
	}
	if got := e.Error(); got == "" {
		t.Errorf("Error() is empty")
	}
}

func TestError_ErrorAndUnwrap(t *testing.T) {
	t.Parallel()
	e := &Error{
		Method:      "sendMessage",
		StatusCode:  429,
		Description: "Too Many Requests",
		RetryAfter:  7 * time.Second,
		Attempts:    2,
		Ambiguous:   false,
		Err:         context.DeadlineExceeded,
	}
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Errorf("errors.Is(e, context.DeadlineExceeded) = false")
	}
	msg := e.Error()
	if msg == "" {
		t.Fatal("Error() is empty")
	}
	for _, want := range []string{"sendMessage", "429", "2", "false"} {
		if !strings.Contains(msg, want) {
			t.Errorf("Error() = %q, want it to contain %q", msg, want)
		}
	}
}

func TestChatTarget_String(t *testing.T) {
	t.Parallel()
	cases := []struct {
		target ChatTarget
		want   string
	}{
		{ChatNone, "ChatNone"},
		{ChatUnknown, "ChatUnknown"},
		{ChatKnown, "ChatKnown"},
	}
	for _, tc := range cases {
		if got := tc.target.String(); got != tc.want {
			t.Errorf("%v.String() = %q, want %q", int(tc.target), got, tc.want)
		}
	}
}
