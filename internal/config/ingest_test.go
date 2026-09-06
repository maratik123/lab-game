package config

import (
	"testing"
	"time"
)

func TestLoadIngest_AllAbsentYieldsDefaults(t *testing.T) {
	t.Parallel()
	i, err := loadIngest(mapLookup(map[string]string{}))
	if err != nil {
		t.Fatalf("loadIngest: unexpected error: %v", err)
	}
	want := defaultIngest()
	if *i != want {
		t.Errorf("loadIngest(empty) = %+v, want defaults %+v", *i, want)
	}
}

// TestLoadIngest_ExampleMatchesDefaults mirrors
// TestLoadScheduler_ExampleMatchesDefaults: .env.example's own
// LAB_GAME_INGEST_* values, run through loadIngest, must equal
// defaultIngest() (design D15).
func TestLoadIngest_ExampleMatchesDefaults(t *testing.T) {
	t.Parallel()
	example := readEnvExampleKeys(t)
	i, err := loadIngest(mapLookup(example))
	if err != nil {
		t.Fatalf("loadIngest(.env.example): unexpected error: %v", err)
	}
	want := defaultIngest()
	if *i != want {
		t.Errorf("loadIngest(.env.example) = %+v, want defaultIngest() %+v — "+
			".env.example's LAB_GAME_INGEST_* values and the compiled-in defaults have drifted apart",
			*i, want)
	}
}

func TestLoadIngest_ValuesParsed(t *testing.T) {
	t.Parallel()
	env := map[string]string{
		envIngestPollInterval:     "500ms",
		envIngestLongPollTimeout:  "10s",
		envIngestBatchLimit:       "50",
		envIngestRetryMaxAttempts: "3",
		envIngestRetryBaseDelay:   "2s",
		envIngestRetryMaxDelay:    "1m",
	}
	i, err := loadIngest(mapLookup(env))
	if err != nil {
		t.Fatalf("loadIngest: unexpected error: %v", err)
	}
	if i.PollInterval != 500*time.Millisecond {
		t.Errorf("PollInterval = %v, want 500ms", i.PollInterval)
	}
	if i.LongPollTimeout != 10*time.Second {
		t.Errorf("LongPollTimeout = %v, want 10s", i.LongPollTimeout)
	}
	if i.BatchLimit != 50 {
		t.Errorf("BatchLimit = %d, want 50", i.BatchLimit)
	}
	if i.RetryMaxAttempts != 3 {
		t.Errorf("RetryMaxAttempts = %d, want 3", i.RetryMaxAttempts)
	}
	if i.RetryBaseDelay != 2*time.Second {
		t.Errorf("RetryBaseDelay = %v, want 2s", i.RetryBaseDelay)
	}
	if i.RetryMaxDelay != time.Minute {
		t.Errorf("RetryMaxDelay = %v, want 1m", i.RetryMaxDelay)
	}
}

func TestLoadIngest_Malformed(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		key  string
		val  string
	}{
		{"poll interval not a duration", envIngestPollInterval, "soon"},
		{"poll interval zero", envIngestPollInterval, "0s"},
		{"poll interval negative", envIngestPollInterval, "-1s"},
		{"long poll timeout not a duration", envIngestLongPollTimeout, "soon"},
		{"long poll timeout zero", envIngestLongPollTimeout, "0s"},
		{"long poll timeout negative", envIngestLongPollTimeout, "-1s"},
		{"batch limit not a number", envIngestBatchLimit, "many"},
		{"batch limit zero", envIngestBatchLimit, "0"},
		{"batch limit negative", envIngestBatchLimit, "-1"},
		{"batch limit above the Bot API's accepted range", envIngestBatchLimit, "101"},
		{"retry max attempts not a number", envIngestRetryMaxAttempts, "many"},
		{"retry max attempts zero", envIngestRetryMaxAttempts, "0"},
		{"retry base delay not a duration", envIngestRetryBaseDelay, "soon"},
		{"retry base delay zero", envIngestRetryBaseDelay, "0s"},
		{"retry max delay not a duration", envIngestRetryMaxDelay, "later"},
		{"retry max delay zero", envIngestRetryMaxDelay, "0s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := loadIngest(mapLookup(map[string]string{tc.key: tc.val}))
			assertKeyError(t, err, ErrInvalidValue, tc.key)
		})
	}
}

// TestLoadIngest_BatchLimitAcceptsBotAPIRange pins the boundary of the
// Bot API's stated 1-100 range (design D15).
func TestLoadIngest_BatchLimitAcceptsBotAPIRange(t *testing.T) {
	t.Parallel()
	cases := []struct {
		val  string
		want int
	}{
		{"1", 1},
		{"100", 100},
	}
	for _, tc := range cases {
		i, err := loadIngest(mapLookup(map[string]string{envIngestBatchLimit: tc.val}))
		if err != nil {
			t.Fatalf("loadIngest(%s=%s): unexpected error: %v", envIngestBatchLimit, tc.val, err)
		}
		if i.BatchLimit != tc.want {
			t.Errorf("loadIngest(%s=%s).BatchLimit = %d, want %d", envIngestBatchLimit, tc.val, i.BatchLimit, tc.want)
		}
	}
}

func TestLoadIngest_QueriesEveryKeyUnconditionally(t *testing.T) {
	t.Parallel()
	lookup, recorded := recordingLookup(mapLookup(map[string]string{}))
	if _, err := loadIngest(lookup); err != nil {
		t.Fatalf("loadIngest: unexpected error: %v", err)
	}
	assertSameKeySet(t, "loadIngest-consulted keys", recorded(), "ingestEnvKeys()", ingestEnvKeys())
}

// TestLoad_IngestLongPollTimeoutBelowAttemptTimeout is design D16's first
// cross-check: a long-poll timeout at or above Transport.AttemptTimeout is
// rejected naming LAB_GAME_INGEST_LONG_POLL_TIMEOUT, and one strictly
// below it is accepted.
func TestLoad_IngestLongPollTimeoutBelowAttemptTimeout(t *testing.T) {
	t.Parallel()

	example := readEnvExampleKeys(t)
	baseEnv := rewriteExamplePaths(t, example)

	t.Run("equal to AttemptTimeout is rejected", func(t *testing.T) {
		t.Parallel()
		env := cloneEnv(baseEnv)
		env[envTGAttemptTimeout] = "25s"
		env[envIngestLongPollTimeout] = "25s"
		_, err := Load(mapLookup(env))
		assertKeyError(t, err, ErrInvalidValue, envIngestLongPollTimeout)
	})

	t.Run("above AttemptTimeout is rejected", func(t *testing.T) {
		t.Parallel()
		env := cloneEnv(baseEnv)
		env[envTGAttemptTimeout] = "20s"
		env[envIngestLongPollTimeout] = "25s"
		_, err := Load(mapLookup(env))
		assertKeyError(t, err, ErrInvalidValue, envIngestLongPollTimeout)
	})

	t.Run("strictly below AttemptTimeout is accepted", func(t *testing.T) {
		t.Parallel()
		env := cloneEnv(baseEnv)
		env[envTGAttemptTimeout] = "30s"
		env[envIngestLongPollTimeout] = "25s"
		if _, err := Load(mapLookup(env)); err != nil {
			t.Fatalf("Load: unexpected error: %v", err)
		}
	})
}

// TestLoad_IngestLongPollTimeoutWholeSeconds is design D16's second
// cross-check: a long-poll timeout that is not a whole number of seconds
// is rejected naming LAB_GAME_INGEST_LONG_POLL_TIMEOUT even though it is
// well below any plausible AttemptTimeout — the neighbouring check must
// not be the reason the case fails.
func TestLoad_IngestLongPollTimeoutWholeSeconds(t *testing.T) {
	t.Parallel()

	example := readEnvExampleKeys(t)
	baseEnv := rewriteExamplePaths(t, example)

	t.Run("25500ms is rejected", func(t *testing.T) {
		t.Parallel()
		env := cloneEnv(baseEnv)
		env[envIngestLongPollTimeout] = "25500ms"
		_, err := Load(mapLookup(env))
		assertKeyError(t, err, ErrInvalidValue, envIngestLongPollTimeout)
	})

	t.Run("1500ms is rejected even though it is far below AttemptTimeout", func(t *testing.T) {
		t.Parallel()
		env := cloneEnv(baseEnv)
		env[envIngestLongPollTimeout] = "1500ms"
		_, err := Load(mapLookup(env))
		assertKeyError(t, err, ErrInvalidValue, envIngestLongPollTimeout)
	})

	t.Run("25s is accepted", func(t *testing.T) {
		t.Parallel()
		env := cloneEnv(baseEnv)
		env[envIngestLongPollTimeout] = "25s"
		if _, err := Load(mapLookup(env)); err != nil {
			t.Fatalf("Load: unexpected error: %v", err)
		}
	})
}

// cloneEnv returns a shallow copy of env, so a subtest's mutation never
// leaks into a sibling t.Parallel() subtest sharing the same base map.
func cloneEnv(env map[string]string) map[string]string {
	out := make(map[string]string, len(env))
	for k, v := range env {
		out[k] = v
	}
	return out
}
