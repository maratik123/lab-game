package config

import (
	"errors"
	"net"
	"net/url"
	"time"
)

// Environment variable names for the health-metrics and canary surface's
// tuning: the /metrics listen address, the canary tick cadence, and the
// cloud reference leg's credential and endpoint. Every one of these is
// optional: an absent value takes the compiled-in default named alongside
// it below — the same optional-with-default class Transport, Scheduler and
// Ingest already established.
const (
	envHealthMetricsAddr        = "LAB_GAME_HEALTH_METRICS_ADDR"
	envHealthCanaryInterval     = "LAB_GAME_HEALTH_CANARY_INTERVAL"
	envHealthCanaryCloudToken   = "LAB_GAME_HEALTH_CANARY_CLOUD_TOKEN" //nolint:gosec // G101: this is an environment-variable NAME, not a credential value
	envHealthCanaryCloudBaseURL = "LAB_GAME_HEALTH_CANARY_CLOUD_BASE_URL"
)

// defaultHealthMetricsAddr is the /metrics endpoint's compiled-in default
// listen address: the loopback interface only, on a port chosen to
// collide with neither Prometheus's own well-known ports nor this
// module's other documented ports.
const defaultHealthMetricsAddr = "127.0.0.1:9095"

// defaultHealthCanaryCloudBaseURL is the cloud reference leg's compiled-in
// default endpoint — the Bot API's own cloud origin, used as the canary's
// reference target when no operator-configured value overrides it.
const defaultHealthCanaryCloudBaseURL = "https://api.telegram.org"

// healthEnvKeys returns the four health tuning variables, in declaration
// order (deterministic). It is appended to EnvKeys() only; envKeys()
// itself is untouched, mirroring transportEnvKeys', schedulerEnvKeys' and
// ingestEnvKeys' rule.
func healthEnvKeys() []string {
	return []string{
		envHealthMetricsAddr,
		envHealthCanaryInterval,
		envHealthCanaryCloudToken,
		envHealthCanaryCloudBaseURL,
	}
}

// Health holds the health-metrics endpoint's listen address and the
// canary probes' cadence and cloud-leg configuration. Every field is
// optional-with-default — like Transport, Scheduler and Ingest, an absent
// LAB_GAME_HEALTH_ variable never fails Load.
type Health struct {
	// MetricsAddr is the /metrics endpoint's listen address
	// (LAB_GAME_HEALTH_METRICS_ADDR). The compiled-in default is the
	// loopback interface only, on a port chosen to collide with neither
	// Prometheus's own well-known ports nor this module's other
	// documented ports.
	MetricsAddr string
	// CanaryInterval is the tick cadence driving both canary legs
	// (LAB_GAME_HEALTH_CANARY_INTERVAL, default one minute — the design's
	// once-a-minute cadence).
	CanaryInterval time.Duration
	// CanaryCloudToken is the cloud reference leg's bot token
	// (LAB_GAME_HEALTH_CANARY_CLOUD_TOKEN, default empty). An absent or
	// present-but-empty value disables the cloud leg entirely — unlike
	// every other key in this class, present-but-empty is treated as
	// absent rather than as a malformed value, so an operator who copies
	// the example file's placeholder and wants the leg off can blank the
	// value without deleting the line.
	CanaryCloudToken Secret
	// CanaryCloudBaseURL is the cloud reference leg's Bot API base URL
	// (LAB_GAME_HEALTH_CANARY_CLOUD_BASE_URL). Defaults to the Bot API's
	// own cloud origin, validated by the same parser that validates every
	// other configured Bot API base URL in this package.
	CanaryCloudBaseURL url.URL
}

// defaultHealth returns the compiled-in defaults every health key falls
// back to when its environment variable is absent.
//
// The compiled-in cloud base URL literal is a value this package's own
// parser accepts, so the error branch below is unreachable in practice;
// it is handled rather than ignored so a future edit to the literal
// fails as an empty URL instead of silently swallowing the parse error.
func defaultHealth() Health {
	var cloudBaseURL url.URL
	if u, err := parseBotAPIBaseURL(defaultHealthCanaryCloudBaseURL); err == nil {
		cloudBaseURL = *u
	}
	return Health{
		MetricsAddr:        defaultHealthMetricsAddr,
		CanaryInterval:     time.Minute,
		CanaryCloudToken:   "",
		CanaryCloudBaseURL: cloudBaseURL,
	}
}

// loadHealth reads and validates every LAB_GAME_HEALTH_ variable through
// lookup, querying each one unconditionally regardless of any other key's
// presence (required so the disjointness test's recording lookup still
// records an absent optional key — mirrors loadTransport, loadScheduler
// and loadIngest). An absent variable takes its compiled-in default; a
// present, malformed value is reported as a *KeyError naming that
// variable — except CanaryCloudToken, whose present-but-empty value is
// treated as absent rather than malformed, so an operator wanting the
// cloud leg off may blank the value without deleting the line. Every
// failure is collected and returned at once via errors.Join, in
// declaration order.
func loadHealth(lookup Lookup) (*Health, error) {
	var errs []error
	h := defaultHealth()

	if s, ok := lookup(envHealthMetricsAddr); ok {
		if s == "" {
			errs = append(errs, keyErrorf(envHealthMetricsAddr, ErrInvalidValue, "must not be empty"))
		} else if _, _, err := net.SplitHostPort(s); err != nil {
			errs = append(errs, keyErrorf(envHealthMetricsAddr, ErrInvalidValue, "%s", err))
		} else {
			h.MetricsAddr = s
		}
	}

	if d, ok, err := lookupPositiveDuration(lookup, envHealthCanaryInterval); err != nil {
		errs = append(errs, err)
	} else if ok {
		h.CanaryInterval = d
	}

	if s, ok := lookup(envHealthCanaryCloudToken); ok && s != "" {
		h.CanaryCloudToken = Secret(s)
	}

	if s, ok := lookup(envHealthCanaryCloudBaseURL); ok {
		if s == "" {
			errs = append(errs, keyErrorf(envHealthCanaryCloudBaseURL, ErrInvalidValue, "must not be empty"))
		} else if u, err := parseBotAPIBaseURL(s); err != nil {
			errs = append(errs, keyErrorf(envHealthCanaryCloudBaseURL, ErrInvalidValue, "%s", err))
		} else {
			h.CanaryCloudBaseURL = *u
		}
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return &h, nil
}
