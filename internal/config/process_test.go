package config

import (
	"testing"
	"time"
)

func TestLoadProcess_AllAbsentYieldsDefaults(t *testing.T) {
	t.Parallel()
	p, err := loadProcess(mapLookup(map[string]string{}))
	if err != nil {
		t.Fatalf("loadProcess: unexpected error: %v", err)
	}
	want := defaultProcess()
	if *p != want {
		t.Errorf("loadProcess(empty) = %+v, want defaults %+v", *p, want)
	}
}

// TestLoadProcess_DefaultMigrateOnStartIsTrue pins the decided policy:
// apply migrations at start-up by default, with an opt-out.
func TestLoadProcess_DefaultMigrateOnStartIsTrue(t *testing.T) {
	t.Parallel()
	p, err := loadProcess(mapLookup(map[string]string{}))
	if err != nil {
		t.Fatalf("loadProcess: unexpected error: %v", err)
	}
	if !p.MigrateOnStart {
		t.Errorf("MigrateOnStart = %v, want true", p.MigrateOnStart)
	}
}

func TestLoadProcess_ValuesParsed(t *testing.T) {
	t.Parallel()
	env := map[string]string{
		envProcessMigrateOnStart:   "false",
		envProcessShutdownTimeout:  "45s",
		envProcessLivenessInterval: "10s",
		envProcessDowntimeThresh:   "10m",
	}
	p, err := loadProcess(mapLookup(env))
	if err != nil {
		t.Fatalf("loadProcess: unexpected error: %v", err)
	}
	if p.MigrateOnStart {
		t.Errorf("MigrateOnStart = %v, want false", p.MigrateOnStart)
	}
	if p.ShutdownTimeout != 45*time.Second {
		t.Errorf("ShutdownTimeout = %v, want 45s", p.ShutdownTimeout)
	}
	if p.LivenessInterval != 10*time.Second {
		t.Errorf("LivenessInterval = %v, want 10s", p.LivenessInterval)
	}
	if p.DowntimeThreshold != 10*time.Minute {
		t.Errorf("DowntimeThreshold = %v, want 10m", p.DowntimeThreshold)
	}
}

func TestLoadProcess_MigrateOnStartAcceptsEveryParseBoolSpelling(t *testing.T) {
	t.Parallel()
	trueSpellings := []string{"1", "t", "T", "TRUE", "true", "True"}
	for _, v := range trueSpellings {
		p, err := loadProcess(mapLookup(map[string]string{envProcessMigrateOnStart: v}))
		if err != nil {
			t.Fatalf("loadProcess(%q): unexpected error: %v", v, err)
		}
		if !p.MigrateOnStart {
			t.Errorf("loadProcess(%q).MigrateOnStart = false, want true", v)
		}
	}
	falseSpellings := []string{"0", "f", "F", "FALSE", "false", "False"}
	for _, v := range falseSpellings {
		p, err := loadProcess(mapLookup(map[string]string{envProcessMigrateOnStart: v}))
		if err != nil {
			t.Fatalf("loadProcess(%q): unexpected error: %v", v, err)
		}
		if p.MigrateOnStart {
			t.Errorf("loadProcess(%q).MigrateOnStart = true, want false", v)
		}
	}
}

func TestLoadProcess_MalformedValuesRejected(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		key  string
		val  string
	}{
		{"malformed migrate-on-start", envProcessMigrateOnStart, "not-a-bool"},
		{"malformed shutdown timeout", envProcessShutdownTimeout, "not-a-duration"},
		{"non-positive shutdown timeout", envProcessShutdownTimeout, "0s"},
		{"malformed liveness interval", envProcessLivenessInterval, "not-a-duration"},
		{"non-positive liveness interval", envProcessLivenessInterval, "-1s"},
		{"malformed downtime threshold", envProcessDowntimeThresh, "not-a-duration"},
		{"non-positive downtime threshold", envProcessDowntimeThresh, "0s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := loadProcess(mapLookup(map[string]string{tc.key: tc.val}))
			assertKeyError(t, err, ErrInvalidValue, tc.key)
		})
	}
}

// TestLoadProcess_ExampleMatchesDefaults mirrors
// TestLoadIngest_ExampleMatchesDefaults: the shipped example file's
// values must equal the compiled-in defaults for this class.
func TestLoadProcess_ExampleMatchesDefaults(t *testing.T) {
	t.Parallel()
	example := readEnvExampleKeys(t)
	p, err := loadProcess(mapLookup(example))
	if err != nil {
		t.Fatalf("loadProcess(.env.example): unexpected error: %v", err)
	}
	want := defaultProcess()
	if *p != want {
		t.Errorf("loadProcess(.env.example) = %+v, want defaultProcess() %+v — "+
			".env.example's LAB_GAME_PROCESS_* values and the compiled-in defaults have drifted apart",
			*p, want)
	}
}

func TestLoadProcess_QueriesEveryKeyUnconditionally(t *testing.T) {
	t.Parallel()
	lookup, recorded := recordingLookup(mapLookup(map[string]string{}))
	if _, err := loadProcess(lookup); err != nil {
		t.Fatalf("loadProcess: unexpected error: %v", err)
	}
	assertSameKeySet(t, "loadProcess-consulted keys", recorded(), "processEnvKeys()", processEnvKeys())
}
