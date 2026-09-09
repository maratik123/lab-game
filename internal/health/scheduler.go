package health

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/maratik123/lab-game/internal/scheduler"
)

// SchedulerObserver is the task scheduler's Observer implementation.
// Every field of a task observation and a loop observation reaches
// either a family below or a named package-level exemption (this
// package's own documentation carries the reasons in full).
type SchedulerObserver struct {
	taskLag      *prometheus.HistogramVec
	tasks        *prometheus.CounterVec
	loopDuration prometheus.Histogram
	claimBatch   prometheus.Histogram
	loopErrors   prometheus.Counter
}

// NewSchedulerObserver registers the scheduler families on reg and
// returns the observer to install on the scheduler worker.
func NewSchedulerObserver(reg prometheus.Registerer) (*SchedulerObserver, error) {
	o := &SchedulerObserver{
		taskLag: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    familySchedulerTaskLag,
			Help:    "A scheduled task's execution instant minus its due instant, in seconds, by task type.",
			Buckets: schedulerLagBuckets,
		}, []string{labelType}),
		tasks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: familySchedulerTasks,
			Help: "Executed scheduler tasks, by type, outcome and failure classification.",
		}, []string{labelType, labelOutcome, labelFailure}),
		loopDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    familySchedulerLoopDuration,
			Help:    "One scheduler discovery-and-execution cycle's wall time, in seconds.",
			Buckets: schedulerLoopBuckets,
		}),
		claimBatch: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    familySchedulerClaimBatch,
			Help:    "One scheduler discovery cycle's claimed-task count.",
			Buckets: schedulerBatchSizeBuckets,
		}),
		loopErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Name: familySchedulerLoopErrors,
			Help: "Scheduler discovery cycles that ended in a transient error.",
		}),
	}
	for _, c := range []prometheus.Collector{o.taskLag, o.tasks, o.loopDuration, o.claimBatch, o.loopErrors} {
		if err := reg.Register(c); err != nil {
			return nil, fmt.Errorf("health: register scheduler family: %w", err)
		}
	}
	return o, nil
}

// ObserveTask implements the scheduler's Observer interface.
func (o *SchedulerObserver) ObserveTask(obs scheduler.Observation) {
	o.taskLag.WithLabelValues(string(obs.Type)).Observe(obs.Lag.Seconds())
	o.tasks.WithLabelValues(string(obs.Type), schedulerOutcomeLabel(obs.Outcome), schedulerFailureLabel(obs.Failure)).Inc()
}

// ObserveLoop implements the scheduler's Observer interface.
func (o *SchedulerObserver) ObserveLoop(obs scheduler.LoopObservation) {
	o.loopDuration.Observe(obs.Duration.Seconds())
	o.claimBatch.Observe(float64(obs.BatchSize))
	if obs.Err != nil {
		o.loopErrors.Inc()
	}
}
