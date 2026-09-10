package config

import (
	"errors"
	"time"
)

// Environment variable names for the composition root's own process
// tuning: the migration-apply policy, the whole-shutdown budget, the
// liveness heartbeat cadence and the restart-hygiene downtime threshold.
// Every one of these is optional: an absent value takes the compiled-in
// default named alongside it below — the same optional-with-default
// class Transport, Scheduler, Ingest and Health already established, and
// the fifth such class this package now carries.
const (
	envProcessMigrateOnStart   = "LAB_GAME_PROCESS_MIGRATE_ON_START"
	envProcessShutdownTimeout  = "LAB_GAME_PROCESS_SHUTDOWN_TIMEOUT"
	envProcessLivenessInterval = "LAB_GAME_PROCESS_LIVENESS_INTERVAL"
	envProcessDowntimeThresh   = "LAB_GAME_PROCESS_DOWNTIME_THRESHOLD"
)

// processEnvKeys returns the four process tuning variables, in
// declaration order (deterministic). It is appended to EnvKeys() only;
// envKeys() itself is untouched, mirroring transportEnvKeys',
// schedulerEnvKeys', ingestEnvKeys' and healthEnvKeys' rule.
func processEnvKeys() []string {
	return []string{
		envProcessMigrateOnStart,
		envProcessShutdownTimeout,
		envProcessLivenessInterval,
		envProcessDowntimeThresh,
	}
}

// Process holds the composition root's own tuning: whether pending
// migrations are applied at start-up, the whole-shutdown deadline, and
// the restart-hygiene liveness heartbeat's cadence and downtime
// threshold. Every field is optional-with-default — like Transport,
// Scheduler, Ingest and Health, an absent LAB_GAME_PROCESS_ variable
// never fails Load. These are operational tuning, never a balance
// number.
type Process struct {
	// MigrateOnStart controls whether the process applies pending
	// migrations at start-up (LAB_GAME_PROCESS_MIGRATE_ON_START, default
	// true — the decided policy is apply by default, with an opt-out for
	// an operator who applies migrations out of band).
	MigrateOnStart bool
	// ShutdownTimeout bounds the whole graceful-shutdown sequence — one
	// deadline for the drain as a whole, not a per-subsystem budget
	// (LAB_GAME_PROCESS_SHUTDOWN_TIMEOUT, default 30s).
	ShutdownTimeout time.Duration
	// LivenessInterval is how often the persisted liveness instant is
	// refreshed while the process runs
	// (LAB_GAME_PROCESS_LIVENESS_INTERVAL, default 30s) — it bounds the
	// recorded instant's staleness at one interval, far below
	// DowntimeThreshold.
	LivenessInterval time.Duration
	// DowntimeThreshold is the gap, measured against the persisted
	// liveness instant, above which the restart-hygiene shift moves
	// every overdue pending scheduler row's run_at forward by the
	// measured gap (LAB_GAME_PROCESS_DOWNTIME_THRESHOLD, default 5m) —
	// above any ordinary restart or deploy, below the power/internet
	// outage the shift exists to protect against.
	DowntimeThreshold time.Duration
}

// defaultProcess returns the compiled-in defaults every process key
// falls back to when its environment variable is absent.
func defaultProcess() Process {
	return Process{
		MigrateOnStart:    true,
		ShutdownTimeout:   30 * time.Second,
		LivenessInterval:  30 * time.Second,
		DowntimeThreshold: 5 * time.Minute,
	}
}

// loadProcess reads and validates every LAB_GAME_PROCESS_ variable
// through lookup, querying each one unconditionally regardless of any
// other key's presence (required so the disjointness test's recording
// lookup still records an absent optional key — mirrors loadTransport,
// loadScheduler, loadIngest and loadHealth). An absent variable takes
// its compiled-in default; a present, malformed value is reported as a
// *KeyError naming that variable. Every failure is collected and
// returned at once via errors.Join, in declaration order.
func loadProcess(lookup Lookup) (*Process, error) {
	var errs []error
	p := defaultProcess()

	if b, ok, err := lookupBool(lookup, envProcessMigrateOnStart); err != nil {
		errs = append(errs, err)
	} else if ok {
		p.MigrateOnStart = b
	}

	if d, ok, err := lookupPositiveDuration(lookup, envProcessShutdownTimeout); err != nil {
		errs = append(errs, err)
	} else if ok {
		p.ShutdownTimeout = d
	}

	if d, ok, err := lookupPositiveDuration(lookup, envProcessLivenessInterval); err != nil {
		errs = append(errs, err)
	} else if ok {
		p.LivenessInterval = d
	}

	if d, ok, err := lookupPositiveDuration(lookup, envProcessDowntimeThresh); err != nil {
		errs = append(errs, err)
	} else if ok {
		p.DowntimeThreshold = d
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return &p, nil
}
