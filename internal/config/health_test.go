package config

import (
	"net"
	"testing"
	"time"
)

func TestLoadHealth_AllAbsentYieldsDefaults(t *testing.T) {
	t.Parallel()
	h, err := loadHealth(mapLookup(map[string]string{}))
	if err != nil {
		t.Fatalf("loadHealth: unexpected error: %v", err)
	}
	want := defaultHealth()
	if *h != want {
		t.Errorf("loadHealth(empty) = %+v, want defaults %+v", *h, want)
	}
}

// TestLoadHealth_DefaultCloudTokenIsEmpty pins "absent means off": with
// the cloud token key absent, the loaded value is the empty secret, which
// is what a leg builder treats as "build no cloud client at all".
func TestLoadHealth_DefaultCloudTokenIsEmpty(t *testing.T) {
	t.Parallel()
	h, err := loadHealth(mapLookup(map[string]string{}))
	if err != nil {
		t.Fatalf("loadHealth: unexpected error: %v", err)
	}
	if h.CanaryCloudToken != "" {
		t.Errorf("CanaryCloudToken = %q, want empty", h.CanaryCloudToken.Reveal())
	}
}

// TestLoadHealth_DefaultMetricsAddrIsLoopback asserts that the
// compiled-in default binds the loopback interface only.
func TestLoadHealth_DefaultMetricsAddrIsLoopback(t *testing.T) {
	t.Parallel()
	h, err := loadHealth(mapLookup(map[string]string{}))
	if err != nil {
		t.Fatalf("loadHealth: unexpected error: %v", err)
	}
	host, _, err := net.SplitHostPort(h.MetricsAddr)
	if err != nil {
		t.Fatalf("net.SplitHostPort(%q): %v", h.MetricsAddr, err)
	}
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		t.Errorf("default MetricsAddr host = %q, want a loopback address", host)
	}
}

// TestLoadHealth_PresentButEmptyCloudTokenIsAbsent is the escape hatch:
// unlike every other key in this class, a present-but-empty
// LAB_GAME_HEALTH_CANARY_CLOUD_TOKEN is treated exactly as absent, not as
// a malformed value.
func TestLoadHealth_PresentButEmptyCloudTokenIsAbsent(t *testing.T) {
	t.Parallel()
	h, err := loadHealth(mapLookup(map[string]string{envHealthCanaryCloudToken: ""}))
	if err != nil {
		t.Fatalf("loadHealth: unexpected error: %v", err)
	}
	if h.CanaryCloudToken != "" {
		t.Errorf("CanaryCloudToken = %q, want empty", h.CanaryCloudToken.Reveal())
	}
}

func TestLoadHealth_ValuesParsed(t *testing.T) {
	t.Parallel()
	env := map[string]string{
		envHealthMetricsAddr:        "0.0.0.0:9999",
		envHealthCanaryInterval:     "30s",
		envHealthCanaryCloudToken:   "cloud-token",
		envHealthCanaryCloudBaseURL: "https://example.invalid",
	}
	h, err := loadHealth(mapLookup(env))
	if err != nil {
		t.Fatalf("loadHealth: unexpected error: %v", err)
	}
	if h.MetricsAddr != "0.0.0.0:9999" {
		t.Errorf("MetricsAddr = %q, want %q", h.MetricsAddr, "0.0.0.0:9999")
	}
	if h.CanaryInterval != 30*time.Second {
		t.Errorf("CanaryInterval = %v, want 30s", h.CanaryInterval)
	}
	if h.CanaryCloudToken.Reveal() != "cloud-token" {
		t.Errorf("CanaryCloudToken = %q, want %q", h.CanaryCloudToken.Reveal(), "cloud-token")
	}
	if h.CanaryCloudBaseURL.String() != "https://example.invalid" {
		t.Errorf("CanaryCloudBaseURL = %q, want %q", h.CanaryCloudBaseURL.String(), "https://example.invalid")
	}
}

func TestLoadHealth_MalformedValuesRejected(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		key  string
		val  string
	}{
		{"malformed metrics addr", envHealthMetricsAddr, "not-a-host-port"},
		{"empty metrics addr", envHealthMetricsAddr, ""},
		{"malformed canary interval", envHealthCanaryInterval, "not-a-duration"},
		{"non-positive canary interval", envHealthCanaryInterval, "0s"},
		{"malformed cloud base URL", envHealthCanaryCloudBaseURL, "://not a url"},
		{"empty cloud base URL", envHealthCanaryCloudBaseURL, ""},
		{"cloud base URL with no scheme", envHealthCanaryCloudBaseURL, "example.invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := loadHealth(mapLookup(map[string]string{tc.key: tc.val}))
			assertKeyError(t, err, ErrInvalidValue, tc.key)
		})
	}
}

// TestLoadHealth_ExampleMatchesDefaults mirrors
// TestLoadIngest_ExampleMatchesDefaults, with one difference: CanaryCloudToken
// is excluded from the comparison, since the example file's placeholder
// value is deliberately non-empty while the compiled-in default that
// disables the cloud leg is deliberately empty — the two are not meant to
// agree on that one field.
func TestLoadHealth_ExampleMatchesDefaults(t *testing.T) {
	t.Parallel()
	example := readEnvExampleKeys(t)
	h, err := loadHealth(mapLookup(example))
	if err != nil {
		t.Fatalf("loadHealth(.env.example): unexpected error: %v", err)
	}
	got := *h
	got.CanaryCloudToken = ""
	want := defaultHealth()
	want.CanaryCloudToken = ""
	if got != want {
		t.Errorf("loadHealth(.env.example) (token zeroed) = %+v, want defaultHealth() (token zeroed) %+v — "+
			".env.example's LAB_GAME_HEALTH_* values and the compiled-in defaults have drifted apart",
			got, want)
	}
}

// TestLoadHealth_ExampleCloudTokenIsNonEmptyPlaceholder pins the other
// half of the example-file trap: the shipped placeholder value is
// non-empty (also asserted, package-wide, by
// TestEnvExample_ValuesAreNonEmpty) while the compiled-in default that
// disables the cloud leg is empty — an absent key and the example's own
// placeholder value are deliberately different starting points, and this
// test is what would catch the placeholder silently becoming empty (which
// TestEnvExample_ValuesAreNonEmpty would also catch, but for the wrong
// reason if it did).
func TestLoadHealth_ExampleCloudTokenIsNonEmptyPlaceholder(t *testing.T) {
	t.Parallel()
	example := readEnvExampleKeys(t)
	if example[envHealthCanaryCloudToken] == "" {
		t.Fatalf(".env.example's %s is empty, want a non-empty placeholder", envHealthCanaryCloudToken)
	}
}

// TestDefaultHealth_CloudBaseURLParses pins the literal
// defaultHealthCanaryCloudBaseURL as one this package's own base-URL
// parser accepts, which is what makes defaultHealth's error branch
// unreachable in practice.
func TestDefaultHealth_CloudBaseURLParses(t *testing.T) {
	t.Parallel()
	if _, err := parseBotAPIBaseURL(defaultHealthCanaryCloudBaseURL); err != nil {
		t.Fatalf("parseBotAPIBaseURL(defaultHealthCanaryCloudBaseURL): %v", err)
	}
}

func TestLoadHealth_QueriesEveryKeyUnconditionally(t *testing.T) {
	t.Parallel()
	lookup, recorded := recordingLookup(mapLookup(map[string]string{}))
	if _, err := loadHealth(lookup); err != nil {
		t.Fatalf("loadHealth: unexpected error: %v", err)
	}
	assertSameKeySet(t, "loadHealth-consulted keys", recorded(), "healthEnvKeys()", healthEnvKeys())
}
