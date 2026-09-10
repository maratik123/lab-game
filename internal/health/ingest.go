package health

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/maratik123/lab-game/internal/ingest"
)

// IngestObserver is the update-ingest loop's Observer implementation.
// Every field of an update observation and a loop observation reaches
// either a family below or a named package-level exemption (this
// package's own documentation carries the reasons in full).
type IngestObserver struct {
	updateLag       *prometheus.HistogramVec
	handlerDuration *prometheus.HistogramVec
	updateOutcomes  *prometheus.CounterVec
	undecodable     prometheus.Counter
	pollDuration    prometheus.Histogram
	pollBatch       prometheus.Histogram
	pollErrors      prometheus.Counter
}

// NewIngestObserver registers the ingest families on reg and returns the
// observer to install on the update-ingest loop.
func NewIngestObserver(reg prometheus.Registerer) (*IngestObserver, error) {
	o := &IngestObserver{
		updateLag: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    familyIngestUpdateLag,
			Help:    "An update's own date subtracted from the observation instant, in seconds, by kind — sampled only when the update's payload declares a date.",
			Buckets: updateLagBuckets,
		}, []string{labelKind}),
		handlerDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    familyIngestHandlerDuration,
			Help:    "One update observation's own unit of work, in seconds, by kind and outcome.",
			Buckets: durationBuckets,
		}, []string{labelKind, labelOutcome}),
		updateOutcomes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: familyIngestUpdateOutcomes,
			Help: "Update observations, by kind and outcome — one per handler attempt plus one per attempt-less settlement, so a retried update contributes several.",
		}, []string{labelKind, labelOutcome}),
		undecodable: prometheus.NewCounter(prometheus.CounterOpts{
			Name: familyIngestUndecodable,
			Help: "Updates whose raw payload could not be decoded.",
		}),
		pollDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    familyIngestPollDuration,
			Help:    "One ingest poll cycle's wall time, in seconds.",
			Buckets: pollDurationBuckets,
		}),
		pollBatch: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    familyIngestPollBatch,
			Help:    "One ingest poll cycle's returned update count.",
			Buckets: ingestBatchSizeBuckets,
		}),
		pollErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: familyIngestPollErrors,
			Help: "Ingest poll cycles that ended in a poll error.",
		}),
	}
	cols := []prometheus.Collector{
		o.updateLag, o.handlerDuration, o.updateOutcomes, o.undecodable,
		o.pollDuration, o.pollBatch, o.pollErrors,
	}
	for _, c := range cols {
		if err := reg.Register(c); err != nil {
			return nil, fmt.Errorf("health: register ingest family: %w", err)
		}
	}
	return o, nil
}

// ObserveUpdate implements the ingest loop's Observer interface. The lag
// sample is taken only when the observation's own LagKnown flag is true
// — an observation with no known lag contributes no sample at all,
// distinguishing "no sample" from "a zero-valued sample".
func (o *IngestObserver) ObserveUpdate(obs ingest.Observation) {
	kind := ingestKindLabel(obs.Kind)
	outcome := ingestOutcomeLabel(obs.Outcome)
	if obs.LagKnown {
		o.updateLag.WithLabelValues(kind).Observe(obs.Lag.Seconds())
	}
	o.handlerDuration.WithLabelValues(kind, outcome).Observe(obs.Duration.Seconds())
	o.updateOutcomes.WithLabelValues(kind, outcome).Inc()
	if obs.Err != nil {
		o.undecodable.Inc()
	}
}

// ObserveLoop implements the ingest loop's Observer interface.
func (o *IngestObserver) ObserveLoop(obs ingest.LoopObservation) {
	o.pollDuration.Observe(obs.Duration.Seconds())
	o.pollBatch.Observe(float64(obs.BatchSize))
	if obs.Err != nil {
		o.pollErrors.Inc()
	}
}
