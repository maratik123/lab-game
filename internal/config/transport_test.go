package config

import (
	"testing"
	"time"

	"github.com/maratik123/lab-game/internal/backoff"
)

func TestLoadTransport_AllAbsentYieldsDefaults(t *testing.T) {
	t.Parallel()
	tr, err := loadTransport(mapLookup(map[string]string{}))
	if err != nil {
		t.Fatalf("loadTransport: unexpected error: %v", err)
	}
	want := defaultTransport()
	if *tr != want {
		t.Errorf("loadTransport(empty) = %+v, want defaults %+v", *tr, want)
	}
}

// TestLoadTransport_ExampleMatchesDefaults is AC22's and AC23's
// "documented default for each" clause: .env.example's own LAB_GAME_TG_*
// values, run through loadTransport, must equal defaultTransport() — so a
// change to either side that is not mirrored in the other goes RED. A
// bare `*tr == defaultTransport()` comparison against an empty
// environment (as TestLoadTransport_AllAbsentYieldsDefaults does above)
// cannot detect that drift: it would pass for any value
// defaultTransport() happens to return, whatever .env.example says.
func TestLoadTransport_ExampleMatchesDefaults(t *testing.T) {
	t.Parallel()
	example := readEnvExampleKeys(t)
	tr, err := loadTransport(mapLookup(example))
	if err != nil {
		t.Fatalf("loadTransport(.env.example): unexpected error: %v", err)
	}
	want := defaultTransport()
	if *tr != want {
		t.Errorf("loadTransport(.env.example) = %+v, want defaultTransport() %+v — "+
			".env.example's LAB_GAME_TG_* values and the compiled-in defaults have drifted apart",
			*tr, want)
	}
}

// TestLoadTransport_DefaultRetryFactorIsBackoffDefaultFactor pins the
// default to the shared constant directly, not merely to
// defaultTransport()'s own return — so the config default and the shared
// boundary cannot drift apart (design D20).
func TestLoadTransport_DefaultRetryFactorIsBackoffDefaultFactor(t *testing.T) {
	t.Parallel()
	tr, err := loadTransport(mapLookup(map[string]string{}))
	if err != nil {
		t.Fatalf("loadTransport: unexpected error: %v", err)
	}
	if tr.RetryFactor != backoff.DefaultFactor {
		t.Errorf("RetryFactor = %v, want backoff.DefaultFactor (%v)", tr.RetryFactor, backoff.DefaultFactor)
	}
}

func TestLoadTransport_RetryValuesParsed(t *testing.T) {
	t.Parallel()
	env := map[string]string{
		envTGRetryMaxAttempts: "5",
		envTGRetryBaseDelay:   "1s",
		envTGRetryMaxDelay:    "1m",
		envTGRetryFactor:      "1.5",
		envTGAttemptTimeout:   "10s",
	}
	tr, err := loadTransport(mapLookup(env))
	if err != nil {
		t.Fatalf("loadTransport: unexpected error: %v", err)
	}
	if tr.RetryMaxAttempts != 5 {
		t.Errorf("RetryMaxAttempts = %d, want 5", tr.RetryMaxAttempts)
	}
	if tr.RetryBaseDelay != time.Second {
		t.Errorf("RetryBaseDelay = %v, want 1s", tr.RetryBaseDelay)
	}
	if tr.RetryMaxDelay != time.Minute {
		t.Errorf("RetryMaxDelay = %v, want 1m", tr.RetryMaxDelay)
	}
	if tr.RetryFactor != 1.5 {
		t.Errorf("RetryFactor = %v, want 1.5", tr.RetryFactor)
	}
	if tr.AttemptTimeout != 10*time.Second {
		t.Errorf("AttemptTimeout = %v, want 10s", tr.AttemptTimeout)
	}
}

func TestLoadTransport_LimitValuesParsed(t *testing.T) {
	t.Parallel()
	env := map[string]string{
		envTGLimitMessageGlobal:   "60/1s",
		envTGLimitMessageChatRate: "2/1s",
		envTGLimitMessageChatCap:  "40/1m",
		envTGLimitEditGlobal:      "10/1s",
		envTGLimitOtherChatCap:    "off",
	}
	tr, err := loadTransport(mapLookup(env))
	if err != nil {
		t.Fatalf("loadTransport: unexpected error: %v", err)
	}
	want := Rate{Count: 60, Per: time.Second}
	if tr.Limits.Message.Global != want {
		t.Errorf("Message.Global = %+v, want %+v", tr.Limits.Message.Global, want)
	}
	want = Rate{Count: 2, Per: time.Second}
	if tr.Limits.Message.ChatRate != want {
		t.Errorf("Message.ChatRate = %+v, want %+v", tr.Limits.Message.ChatRate, want)
	}
	want = Rate{Count: 40, Per: time.Minute}
	if tr.Limits.Message.ChatCap != want {
		t.Errorf("Message.ChatCap = %+v, want %+v", tr.Limits.Message.ChatCap, want)
	}
	want = Rate{Count: 10, Per: time.Second}
	if tr.Limits.Edit.Global != want {
		t.Errorf("Edit.Global = %+v, want %+v", tr.Limits.Edit.Global, want)
	}
	if tr.Limits.Other.ChatCap != (Rate{}) {
		t.Errorf("Other.ChatCap = %+v, want zero value (off)", tr.Limits.Other.ChatCap)
	}
	// Untouched keys keep their compiled-in defaults.
	if tr.Limits.Edit.ChatRate != (Rate{}) {
		t.Errorf("Edit.ChatRate = %+v, want zero value (off, default untouched)", tr.Limits.Edit.ChatRate)
	}
}

func TestLoadTransport_Malformed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		key  string
		val  string
	}{
		{"retry max attempts zero", envTGRetryMaxAttempts, "0"},
		{"retry max attempts negative", envTGRetryMaxAttempts, "-1"},
		{"retry max attempts not a number", envTGRetryMaxAttempts, "three"},
		{"retry base delay not a duration", envTGRetryBaseDelay, "soon"},
		{"retry base delay zero", envTGRetryBaseDelay, "0s"},
		{"retry base delay negative", envTGRetryBaseDelay, "-1s"},
		{"retry max delay not a duration", envTGRetryMaxDelay, "later"},
		{"retry factor exactly one", envTGRetryFactor, "1"},
		{"retry factor below one", envTGRetryFactor, "0.5"},
		{"retry factor zero", envTGRetryFactor, "0"},
		{"retry factor negative", envTGRetryFactor, "-1"},
		{"retry factor NaN", envTGRetryFactor, "NaN"},
		{"retry factor positive infinity", envTGRetryFactor, "Inf"},
		{"retry factor infinity spelling", envTGRetryFactor, "infinity"},
		{"retry factor not a number", envTGRetryFactor, "many"},
		{"retry factor empty", envTGRetryFactor, ""},
		{"attempt timeout not a duration", envTGAttemptTimeout, "eventually"},
		{"limit missing slash", envTGLimitMessageGlobal, "30"},
		{"limit zero count", envTGLimitMessageGlobal, "0/1s"},
		{"limit negative count", envTGLimitMessageGlobal, "-1/1s"},
		{"limit count not a number", envTGLimitMessageGlobal, "many/1s"},
		{"limit duration not parseable", envTGLimitMessageGlobal, "30/soon"},
		{"limit zero duration", envTGLimitMessageGlobal, "30/0s"},
		{"limit negative duration", envTGLimitMessageGlobal, "30/-1s"},

		// Every remaining key gets its own malformed case (missing slash is
		// enough to drive lookupRate's error branch and, via assertKeyError,
		// confirm the *KeyError names THAT key specifically) — AC22's
		// malformed clause must be unverified for none of the 13 keys, not
		// just the one (envTGLimitMessageGlobal) the rows above already
		// drive. envTGLimitMessageChatRate/ChatCap additionally exercise
		// loadClass's second and third branches, which no case above
		// reaches at all (self-review round 6, R6-1).
		{"limit missing slash (message chat rate)", envTGLimitMessageChatRate, "30"},
		{"limit missing slash (message chat cap)", envTGLimitMessageChatCap, "30"},
		{"limit missing slash (edit global)", envTGLimitEditGlobal, "30"},
		{"limit missing slash (edit chat rate)", envTGLimitEditChatRate, "30"},
		{"limit missing slash (edit chat cap)", envTGLimitEditChatCap, "30"},
		{"limit missing slash (other global)", envTGLimitOtherGlobal, "30"},
		{"limit missing slash (other chat rate)", envTGLimitOtherChatRate, "30"},
		{"limit missing slash (other chat cap)", envTGLimitOtherChatCap, "30"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := loadTransport(mapLookup(map[string]string{tc.key: tc.val}))
			assertKeyError(t, err, ErrInvalidValue, tc.key)
		})
	}
}

func TestLoadTransport_QueriesEveryKeyUnconditionally(t *testing.T) {
	t.Parallel()
	lookup, recorded := recordingLookup(mapLookup(map[string]string{}))
	if _, err := loadTransport(lookup); err != nil {
		t.Fatalf("loadTransport: unexpected error: %v", err)
	}
	assertSameKeySet(t, "loadTransport-consulted keys", recorded(), "transportEnvKeys()", transportEnvKeys())
}

func TestLoadTransport_RateOffCaseSensitiveAndTrimmed(t *testing.T) {
	t.Parallel()
	tr, err := loadTransport(mapLookup(map[string]string{envTGLimitMessageGlobal: "  off  "}))
	if err != nil {
		t.Fatalf("loadTransport: unexpected error: %v", err)
	}
	if tr.Limits.Message.Global != (Rate{}) {
		t.Errorf("Message.Global = %+v, want zero value (off, trimmed)", tr.Limits.Message.Global)
	}

	_, err = loadTransport(mapLookup(map[string]string{envTGLimitMessageGlobal: "OFF"}))
	assertKeyError(t, err, ErrInvalidValue, envTGLimitMessageGlobal)
}
