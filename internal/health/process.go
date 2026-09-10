package health

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metric family names for the process-identity and restart-hygiene
// surface — declared here, beside the collector that owns them.
const (
	familyBuildInfo           = namePrefix + "build_info"
	familyStartTimeSeconds    = namePrefix + "start_time_seconds"
	familyReady               = namePrefix + "ready"
	familyRestartDowntime     = namePrefix + "restart_downtime_seconds"
	familyRestartShiftedTasks = namePrefix + "restart_shifted_tasks"
)

// readyGaugeTimeout bounds how long the labgame_ready gauge waits on
// Ready during a scrape, matching readyzTimeout — the two share one
// definition of readiness and neither may hang a caller. A named
// constant, not a configuration key.
const readyGaugeTimeout = readyzTimeout

// ProcessOptions configures NewProcess.
type ProcessOptions struct {
	// Version is the build version, exported as labgame_build_info's
	// version label value — never parsed or compared, never a metric
	// name component.
	Version string
	// StartedAt is the instant the composition root began assembling,
	// exported as labgame_start_time_seconds. Supplied by the caller
	// rather than read here, so this file makes no clock call of its
	// own.
	StartedAt time.Time
	// Ready answers labgame_ready — the same definition /readyz
	// answers, so a scrape and a probe agree.
	Ready ReadyFunc
}

// Process exports the process-identity and restart-hygiene metric
// families: the build version, the process start instant, the
// readiness state, and the restart-hygiene downtime shift's own
// measurement.
type Process struct {
	restartDowntime     prometheus.Gauge
	restartShiftedTasks prometheus.Gauge
}

// NewProcess registers the process-identity and restart-hygiene
// families on reg and returns the value ObserveDowntime is later called
// on, refusing a nil Ready with an *OptionError naming the field.
func NewProcess(reg prometheus.Registerer, opts ProcessOptions) (*Process, error) {
	if opts.Ready == nil {
		return nil, &OptionError{Field: "Ready", Reason: reasonMustNotBeNil}
	}

	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: familyBuildInfo,
		Help: "Always 1; the build version is carried as the version label value.",
	}, []string{labelVersion})
	buildInfo.WithLabelValues(opts.Version).Set(1)

	startTime := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: familyStartTimeSeconds,
		Help: "Unix time, in seconds, of the instant the composition root began assembling.",
	})
	startTime.Set(float64(opts.StartedAt.Unix()))

	readyGauge := prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: familyReady,
		Help: "1 when Ready reports no error, 0 otherwise.",
	}, func() float64 {
		ctx, cancel := context.WithTimeout(context.Background(), readyGaugeTimeout)
		defer cancel()
		if opts.Ready(ctx) != nil {
			return 0
		}
		return 1
	})

	p := &Process{
		restartDowntime: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: familyRestartDowntime,
			Help: "The gap, in seconds, the most recent restart-hygiene downtime shift measured.",
		}),
		restartShiftedTasks: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: familyRestartShiftedTasks,
			Help: "The number of scheduled_task rows the most recent restart-hygiene downtime shift moved.",
		}),
	}

	for _, c := range []prometheus.Collector{buildInfo, startTime, readyGauge, p.restartDowntime, p.restartShiftedTasks} {
		if err := reg.Register(c); err != nil {
			return nil, fmt.Errorf("health: register process family: %w", err)
		}
	}
	return p, nil
}

// ObserveDowntime records one restart-hygiene downtime shift's
// measurement: the gap (in seconds) and the number of rows it moved. A
// start-up step that silently rewrites scheduler rows is the shape of
// an incident nobody can reconstruct afterwards — this is what makes it
// reconstructable.
func (p *Process) ObserveDowntime(gap time.Duration, shifted int) {
	p.restartDowntime.Set(gap.Seconds())
	p.restartShiftedTasks.Set(float64(shifted))
}
