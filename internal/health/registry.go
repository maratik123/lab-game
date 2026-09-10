package health

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// namePrefix is prepended to every metric family this package registers,
// except the Go-runtime and process collectors RegisterRuntime installs,
// which keep the client library's own prefixes — the ones a Prometheus
// dashboard expects of them.
const namePrefix = "labgame_"

// Metric family names for the transport, scheduler and ingest adapters.
// Declared once here so the observation-field register below and each
// adapter's own registration cannot name the same family two different
// ways.
const (
	familyBotAPICallDuration = namePrefix + "botapi_call_duration_seconds"
	familyBotAPIResponses    = namePrefix + "botapi_responses_total"
	familyBotAPIRateLimited  = namePrefix + "botapi_rate_limited_total"
	familyBotAPIRetries      = namePrefix + "botapi_retries_total"

	familySchedulerTaskLag      = namePrefix + "scheduler_task_lag_seconds"
	familySchedulerTasks        = namePrefix + "scheduler_tasks_total"
	familySchedulerLoopDuration = namePrefix + "scheduler_loop_duration_seconds"
	familySchedulerClaimBatch   = namePrefix + "scheduler_claim_batch_size"
	familySchedulerLoopErrors   = namePrefix + "scheduler_loop_errors_total"

	familyIngestUpdateLag       = namePrefix + "ingest_update_lag_seconds"
	familyIngestHandlerDuration = namePrefix + "ingest_handler_duration_seconds"
	familyIngestUpdateOutcomes  = namePrefix + "ingest_update_outcomes_total"
	familyIngestUndecodable     = namePrefix + "ingest_undecodable_updates_total"
	familyIngestPollDuration    = namePrefix + "ingest_poll_duration_seconds"
	familyIngestPollBatch       = namePrefix + "ingest_poll_batch_size"
	familyIngestPollErrors      = namePrefix + "ingest_poll_errors_total"
)

// NewRegistry returns a vanilla, empty *prometheus.Registry — nothing is
// registered on it. Every other constructor in this package takes a
// prometheus.Registerer and registers its own family on whatever
// registerer it is handed, so an isolated registry per test case and one
// shared production registry both work identically.
func NewRegistry() *prometheus.Registry {
	return prometheus.NewRegistry()
}

// RegisterRuntime installs the client library's Go-runtime and process
// collectors on reg. Calling it twice on the same registerer returns an
// error — a duplicate-registration error from the second collector this
// call attempts — rather than panicking.
func RegisterRuntime(reg prometheus.Registerer) error {
	if err := reg.Register(collectors.NewGoCollector()); err != nil {
		return fmt.Errorf("health: register Go-runtime collector: %w", err)
	}
	if err := reg.Register(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{})); err != nil {
		return fmt.Errorf("health: register process collector: %w", err)
	}
	return nil
}

// Histogram bucket boundaries, one set per shape this package's families
// need — never a shared, one-size-fits-all set. Every slice is an
// explicit ascending literal: the library's bucket-builder helpers
// (LinearBuckets and friends) panic on a bad argument, which at package
// scope would be an init-time crash with no test between the mistake and
// the binary, so this package declares none of them.
var (
	// durationBuckets covers a transport call, a canary probe, or an
	// ingest handler call: milliseconds up to the transport's default
	// per-attempt timeout.
	durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30}

	// updateLagBuckets covers the ingest update-lag stall indicator: a
	// fraction of a second up to an hour, because a stall is measured in
	// minutes, not milliseconds.
	updateLagBuckets = []float64{0.5, 1, 2, 5, 10, 30, 60, 120, 300, 600, 900, 1800, 3600}

	// schedulerLagBuckets covers a scheduled task's execution lag: a
	// fraction of a second up to half an hour.
	schedulerLagBuckets = []float64{0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 120, 300, 600, 900, 1800}

	// pollDurationBuckets covers one ingest poll cycle: its upper reach
	// covers the default long-poll window.
	pollDurationBuckets = []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 15, 20, 25, 30}

	// schedulerLoopBuckets covers one scheduler RunOnce cycle: the
	// sub-second band.
	schedulerLoopBuckets = []float64{0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1}

	// ingestBatchSizeBuckets covers one poll cycle's update count, up to
	// the Bot API's own accepted ceiling.
	ingestBatchSizeBuckets = []float64{1, 2, 5, 10, 20, 30, 50, 75, 100}

	// schedulerBatchSizeBuckets covers one discovery cycle's claimed-task
	// count, up to the scheduler's own configured claim limit.
	schedulerBatchSizeBuckets = []float64{1, 2, 4, 8, 16, 24, 32}
)

// Observation struct field names that repeat across more than one entry
// below, named once so the repetition reads as intentional rather than as
// a copy-paste slip.
const (
	fieldBatchSize = "BatchSize"
	fieldDuration  = "Duration"
	fieldErr       = "Err"
)

// fieldEntry names one field of an observation struct together with its
// disposition: the metric family it feeds, or — when Family is empty —
// exempt, for the reason the package comment states in full. This table
// exists so a reflection-based guard can bind that prose to the structs
// mechanically, in both directions.
type fieldEntry struct {
	// Field is the observation struct's own field name.
	Field string
	// Family is the metric family this field feeds, or empty when the
	// field is exempt.
	Family string
}

// observationRegister maps each observation struct's own type name to its
// field register. A field of a struct listed here that is absent from its
// slice, or a slice entry naming a field the struct no longer has, both
// fail the guard this table backs.
var observationRegister = map[string][]fieldEntry{
	"tg.Observation": {
		{Field: "Method", Family: familyBotAPICallDuration},
		{Field: "Latency", Family: familyBotAPICallDuration},
		{Field: "StatusCode", Family: familyBotAPIResponses},
		{Field: "RateLimited", Family: familyBotAPIRateLimited},
		{Field: "Retries", Family: familyBotAPIRetries},
	},
	"scheduler.Observation": {
		{Field: "Type", Family: familySchedulerTaskLag},
		{Field: "Lag", Family: familySchedulerTaskLag},
		{Field: "Outcome", Family: familySchedulerTasks},
		{Field: "Failure", Family: familySchedulerTasks},
		{Field: fieldBatchSize, Family: ""},
		{Field: "ConsecutiveFailures", Family: ""},
	},
	"scheduler.LoopObservation": {
		{Field: fieldDuration, Family: familySchedulerLoopDuration},
		{Field: fieldBatchSize, Family: familySchedulerClaimBatch},
		{Field: fieldErr, Family: familySchedulerLoopErrors},
	},
	"ingest.Observation": {
		{Field: "Kind", Family: familyIngestUpdateLag},
		{Field: "Attempt", Family: ""},
		{Field: "Outcome", Family: familyIngestUpdateOutcomes},
		{Field: fieldDuration, Family: familyIngestHandlerDuration},
		{Field: "Lag", Family: familyIngestUpdateLag},
		{Field: "LagKnown", Family: familyIngestUpdateLag},
		{Field: fieldErr, Family: familyIngestUndecodable},
	},
	"ingest.LoopObservation": {
		{Field: fieldDuration, Family: familyIngestPollDuration},
		{Field: fieldBatchSize, Family: familyIngestPollBatch},
		{Field: fieldErr, Family: familyIngestPollErrors},
	},
}
