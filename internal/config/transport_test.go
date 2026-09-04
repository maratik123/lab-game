package config

import (
	"testing"
	"time"
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

func TestLoadTransport_RetryValuesParsed(t *testing.T) {
	t.Parallel()
	env := map[string]string{
		envTGRetryMaxAttempts: "5",
		envTGRetryBaseDelay:   "1s",
		envTGRetryMaxDelay:    "1m",
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
		{"attempt timeout not a duration", envTGAttemptTimeout, "eventually"},
		{"limit missing slash", envTGLimitMessageGlobal, "30"},
		{"limit zero count", envTGLimitMessageGlobal, "0/1s"},
		{"limit negative count", envTGLimitMessageGlobal, "-1/1s"},
		{"limit count not a number", envTGLimitMessageGlobal, "many/1s"},
		{"limit duration not parseable", envTGLimitMessageGlobal, "30/soon"},
		{"limit zero duration", envTGLimitMessageGlobal, "30/0s"},
		{"limit negative duration", envTGLimitMessageGlobal, "30/-1s"},
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
