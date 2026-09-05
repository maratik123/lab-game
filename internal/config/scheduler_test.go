package config

import (
	"testing"
	"time"
)

func TestLoadScheduler_AllAbsentYieldsDefaults(t *testing.T) {
	t.Parallel()
	s, err := loadScheduler(mapLookup(map[string]string{}))
	if err != nil {
		t.Fatalf("loadScheduler: unexpected error: %v", err)
	}
	want := defaultScheduler()
	if *s != want {
		t.Errorf("loadScheduler(empty) = %+v, want defaults %+v", *s, want)
	}
}

// TestLoadScheduler_ExampleMatchesDefaults mirrors
// TestLoadTransport_ExampleMatchesDefaults: .env.example's own
// LAB_GAME_SCHEDULER_* values, run through loadScheduler, must equal
// defaultScheduler() (design D13, AC15).
func TestLoadScheduler_ExampleMatchesDefaults(t *testing.T) {
	t.Parallel()
	example := readEnvExampleKeys(t)
	s, err := loadScheduler(mapLookup(example))
	if err != nil {
		t.Fatalf("loadScheduler(.env.example): unexpected error: %v", err)
	}
	want := defaultScheduler()
	if *s != want {
		t.Errorf("loadScheduler(.env.example) = %+v, want defaultScheduler() %+v — "+
			".env.example's LAB_GAME_SCHEDULER_* values and the compiled-in defaults have drifted apart",
			*s, want)
	}
}

func TestLoadScheduler_ValuesParsed(t *testing.T) {
	t.Parallel()
	env := map[string]string{
		envSchedulerPollInterval:    "500ms",
		envSchedulerClaimLimit:      "10",
		envSchedulerRetryMaxAttempt: "3",
		envSchedulerRetryBaseDelay:  "2s",
		envSchedulerRetryMaxDelay:   "1m",
		envSchedulerTaskTimeout:     "15s",
	}
	s, err := loadScheduler(mapLookup(env))
	if err != nil {
		t.Fatalf("loadScheduler: unexpected error: %v", err)
	}
	if s.PollInterval != 500*time.Millisecond {
		t.Errorf("PollInterval = %v, want 500ms", s.PollInterval)
	}
	if s.ClaimLimit != 10 {
		t.Errorf("ClaimLimit = %d, want 10", s.ClaimLimit)
	}
	if s.RetryMaxAttempts != 3 {
		t.Errorf("RetryMaxAttempts = %d, want 3", s.RetryMaxAttempts)
	}
	if s.RetryBaseDelay != 2*time.Second {
		t.Errorf("RetryBaseDelay = %v, want 2s", s.RetryBaseDelay)
	}
	if s.RetryMaxDelay != time.Minute {
		t.Errorf("RetryMaxDelay = %v, want 1m", s.RetryMaxDelay)
	}
	if s.TaskTimeout != 15*time.Second {
		t.Errorf("TaskTimeout = %v, want 15s", s.TaskTimeout)
	}
}

func TestLoadScheduler_Malformed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		key  string
		val  string
	}{
		{"poll interval not a duration", envSchedulerPollInterval, "soon"},
		{"poll interval zero", envSchedulerPollInterval, "0s"},
		{"poll interval negative", envSchedulerPollInterval, "-1s"},
		{"claim limit not a number", envSchedulerClaimLimit, "many"},
		{"claim limit zero", envSchedulerClaimLimit, "0"},
		{"claim limit negative", envSchedulerClaimLimit, "-1"},
		{"retry max attempts not a number", envSchedulerRetryMaxAttempt, "many"},
		{"retry max attempts zero", envSchedulerRetryMaxAttempt, "0"},
		{"retry base delay not a duration", envSchedulerRetryBaseDelay, "soon"},
		{"retry base delay zero", envSchedulerRetryBaseDelay, "0s"},
		{"retry max delay not a duration", envSchedulerRetryMaxDelay, "later"},
		{"retry max delay zero", envSchedulerRetryMaxDelay, "0s"},
		{"task timeout not a duration", envSchedulerTaskTimeout, "eventually"},
		{"task timeout zero", envSchedulerTaskTimeout, "0s"},
		{"task timeout negative", envSchedulerTaskTimeout, "-1s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := loadScheduler(mapLookup(map[string]string{tc.key: tc.val}))
			assertKeyError(t, err, ErrInvalidValue, tc.key)
		})
	}
}

func TestLoadScheduler_QueriesEveryKeyUnconditionally(t *testing.T) {
	t.Parallel()
	lookup, recorded := recordingLookup(mapLookup(map[string]string{}))
	if _, err := loadScheduler(lookup); err != nil {
		t.Fatalf("loadScheduler: unexpected error: %v", err)
	}
	assertSameKeySet(t, "loadScheduler-consulted keys", recorded(), "schedulerEnvKeys()", schedulerEnvKeys())
}
