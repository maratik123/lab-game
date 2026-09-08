package tg

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/maratik123/lab-game/internal/backoff"
	"github.com/maratik123/lab-game/internal/config"
	"github.com/maratik123/lab-game/internal/tgtest"
)

// installedConstructorTypeName reads the dynamic type name of the
// *telego.Bot's own unexported `constructor` field — telego exposes no
// accessor for it — via reflection over the addressable struct field.
// The two codecs' output bytes are equivalent, so only the installed
// seam itself, not any request body, can distinguish jsonConstructor
// from telego's default. Test-only: gosec is excluded on test files, and
// unsafe.Pointer here only lifts reflect's own read-only restriction on
// an already-addressable field — it never mutates anything.
func installedConstructorTypeName(t *testing.T, c *Client) string {
	t.Helper()
	bot := reflect.ValueOf(c.API()).Elem()
	field := bot.FieldByName("constructor")
	if !field.IsValid() {
		t.Fatal("installedConstructorTypeName: *telego.Bot has no field named \"constructor\" — telego's internal layout changed")
	}
	readable := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem()
	return fmt.Sprintf("%T", readable.Interface())
}

// installedLoggerFields reads the exported DebugMode/PrintErrors fields
// off the *telego.Bot's own installed telego.Logger, via
// (*telego.Bot).Logger() — an exported accessor, so no reflection over
// an unexported field is needed here (unlike installedConstructorTypeName
// above, whose target field telego exposes no accessor for at all).
func installedLoggerFields(t *testing.T, c *Client) (debugMode, printErrors bool) {
	t.Helper()
	v := reflect.ValueOf(c.API().Logger())
	for v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	debugField := v.FieldByName("DebugMode")
	printField := v.FieldByName("PrintErrors")
	if !debugField.IsValid() || !printField.IsValid() {
		t.Fatalf("installedLoggerFields: logger type %s has no DebugMode/PrintErrors fields — telego's default logger layout changed", v.Type())
	}
	return debugField.Bool(), printField.Bool()
}

// TestNew_InstallsPackageJSONConstructor guards against a past
// regression: deleting telego.WithRequestConstructor(jsonConstructor{})
// leaves the suite green because telego then falls back to
// its own default constructor, which produces byte-equivalent JSON — an
// import guard scoped to this project's own imports cannot see it
// either, since telego's marshal path is not one of them. The only thing
// that can distinguish the two is the installed seam itself.
func TestNew_InstallsPackageJSONConstructor(t *testing.T) {
	t.Parallel()
	c, err := New(validOptions())
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	const want = "tg.jsonConstructor"
	if got := installedConstructorTypeName(t, c); got != want {
		t.Errorf("installed request constructor = %s, want %s (design D3/KD-2: every outbound body must marshal through this package's own encoding/json constructor)", got, want)
	}
}

// TestNew_InstallsDiscardLoggerByDefault guards a promise Options.Logger's
// own doc comment makes: "Nil means telego.WithDiscardLogger()";
// replacing the nil-Logger else branch's
// telego.WithDiscardLogger() call with a no-op leaves the suite green,
// silently reverting to telego's own default logger (PrintErrors: true)
// instead of the discard logger (PrintErrors: false) this package's own
// typed Error and Observer are meant to be the sole real output for.
func TestNew_InstallsDiscardLoggerByDefault(t *testing.T) {
	t.Parallel()
	opts := validOptions()
	opts.Logger = nil
	c, err := New(opts)
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	debugMode, printErrors := installedLoggerFields(t, c)
	if debugMode {
		t.Error("installed logger DebugMode = true, want false (telego.WithDiscardLogger())")
	}
	if printErrors {
		t.Error("installed logger PrintErrors = true, want false (telego.WithDiscardLogger()) — a nil Options.Logger must not fall back to telego's own noisy default logger")
	}
}

// recordingLogger is a minimal telego.Logger with exported DebugMode /
// PrintErrors fields, shaped so installedLoggerFields' reflection over
// *telego.Bot's installed logger can read them back regardless of the
// concrete type telego.WithLogger stores.
type recordingLogger struct {
	DebugMode   bool
	PrintErrors bool
}

func (l *recordingLogger) Debugf(string, ...any) {}
func (l *recordingLogger) Errorf(string, ...any) {}

// TestNew_InstallsProvidedLoggerWhenNonNil covers the branch its sibling
// TestNew_InstallsDiscardLoggerByDefault leaves untouched: the non-nil
// if branch (telego.WithLogger(opts.Logger)) — a caller-supplied Logger
// silently reaching telego.NewBot. This constructs with a recognisable
// Options.Logger (DebugMode true, PrintErrors false — the opposite of
// telego's own DiscardLogger, which is DebugMode false / PrintErrors
// false) and asserts it, not the discard logger, ends up installed.
func TestNew_InstallsProvidedLoggerWhenNonNil(t *testing.T) {
	t.Parallel()
	opts := validOptions()
	opts.Logger = &recordingLogger{DebugMode: true, PrintErrors: false}
	c, err := New(opts)
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	debugMode, printErrors := installedLoggerFields(t, c)
	if !debugMode {
		t.Error("installed logger DebugMode = false, want true (Options.Logger, when non-nil, must be installed as-is via telego.WithLogger)")
	}
	if printErrors {
		t.Error("installed logger PrintErrors = true, want false (must be the provided Options.Logger's own value, not telego's default logger's true)")
	}
}

// validTransport returns a Transport value that passes New's validation
// — every retry field positive, every rate-limit class unbounded (the
// zero value), which is a legal configuration: an unbounded value
// contributes no window.
func validTransport() config.Transport {
	return config.Transport{
		RetryMaxAttempts: 3,
		RetryBaseDelay:   500 * time.Millisecond,
		RetryMaxDelay:    30 * time.Second,
		RetryFactor:      backoff.DefaultFactor,
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
		{"retry factor exactly one", func(o *Options) { o.Transport.RetryFactor = 1 }, "Transport.RetryFactor"},
		{"retry factor positive infinity", func(o *Options) { o.Transport.RetryFactor = math.Inf(1) }, "Transport.RetryFactor"},
		{"retry factor NaN", func(o *Options) { o.Transport.RetryFactor = math.NaN() }, "Transport.RetryFactor"},
		{
			"invalid retry factor beside an earlier-invalid field names the earlier field",
			func(o *Options) {
				o.Transport.RetryBaseDelay = 0
				o.Transport.RetryFactor = 1
			},
			"Transport.RetryBaseDelay",
		},
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
		// The default arm: ChatTarget is exported and this package
		// defines no value outside 0-2, so an out-of-range value is a
		// genuine caller mistake, not a fabricated scenario — the same
		// fallback ChatUnknown already answers, the conservative,
		// refuse-by-default state.
		{ChatTarget(99), "ChatUnknown"},
	}
	for _, tc := range cases {
		if got := tc.target.String(); got != tc.want {
			t.Errorf("%v.String() = %q, want %q", int(tc.target), got, tc.want)
		}
	}
}
