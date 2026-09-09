package health

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// Metric family names and label values for the pgx pool collector —
// declared here, beside the collector that owns them, since no other
// file in this package registers a pool family.
const (
	familyPoolConns               = namePrefix + "pgxpool_conns"
	familyPoolTotalConns          = namePrefix + "pgxpool_total_conns"
	familyPoolMaxConns            = namePrefix + "pgxpool_max_conns"
	familyPoolAcquires            = namePrefix + "pgxpool_acquires_total"
	familyPoolAcquireDuration     = namePrefix + "pgxpool_acquire_duration_seconds_total"
	familyPoolEmptyAcquires       = namePrefix + "pgxpool_empty_acquires_total"
	familyPoolEmptyAcquireWait    = namePrefix + "pgxpool_empty_acquire_wait_seconds_total"
	familyPoolCanceledAcquires    = namePrefix + "pgxpool_canceled_acquires_total"
	familyPoolNewConns            = namePrefix + "pgxpool_new_conns_total"
	familyPoolMaxLifetimeDestroys = namePrefix + "pgxpool_max_lifetime_destroys_total"
	familyPoolMaxIdleDestroys     = namePrefix + "pgxpool_max_idle_destroys_total"
)

// Pool connection state label values.
const (
	poolStateIdle         = "idle"
	poolStateAcquired     = "acquired"
	poolStateConstructing = "constructing"
)

// Pool metric descriptors, declared once so every scrape builds each
// metric from the same *prometheus.Desc.
var (
	descPoolConns               = prometheus.NewDesc(familyPoolConns, "Pool connections, by state.", []string{labelState}, nil)
	descPoolTotalConns          = prometheus.NewDesc(familyPoolTotalConns, "Total pool connections.", nil, nil)
	descPoolMaxConns            = prometheus.NewDesc(familyPoolMaxConns, "The pool's configured maximum connection count.", nil, nil)
	descPoolAcquires            = prometheus.NewDesc(familyPoolAcquires, "Successful connection acquisitions from the pool.", nil, nil)
	descPoolAcquireDuration     = prometheus.NewDesc(familyPoolAcquireDuration, "Cumulative time spent acquiring a connection from the pool, in seconds.", nil, nil)
	descPoolEmptyAcquires       = prometheus.NewDesc(familyPoolEmptyAcquires, "Acquisitions that had to wait for a resource to be constructed or become idle.", nil, nil)
	descPoolEmptyAcquireWait    = prometheus.NewDesc(familyPoolEmptyAcquireWait, "Cumulative time spent waiting for an empty-acquire acquisition, in seconds.", nil, nil)
	descPoolCanceledAcquires    = prometheus.NewDesc(familyPoolCanceledAcquires, "Acquisitions canceled by their own context before completion.", nil, nil)
	descPoolNewConns            = prometheus.NewDesc(familyPoolNewConns, "New connections established by the pool.", nil, nil)
	descPoolMaxLifetimeDestroys = prometheus.NewDesc(familyPoolMaxLifetimeDestroys, "Connections destroyed for exceeding their maximum lifetime.", nil, nil)
	descPoolMaxIdleDestroys     = prometheus.NewDesc(familyPoolMaxIdleDestroys, "Connections destroyed for exceeding their maximum idle time.", nil, nil)
)

// PoolCollector exports a pgx connection pool's Stat as Prometheus
// metrics, calling its accessor once per scrape rather than caching a
// snapshot.
type PoolCollector struct {
	stat func() *pgxpool.Stat
}

// NewPoolCollector registers a PoolCollector on reg. stat is called
// exactly once per scrape and its return value is never cached; a nil
// return (e.g. a closed pool) yields a scrape emitting none of this
// collector's families.
func NewPoolCollector(reg prometheus.Registerer, stat func() *pgxpool.Stat) (*PoolCollector, error) {
	c := &PoolCollector{stat: stat}
	if err := reg.Register(c); err != nil {
		return nil, fmt.Errorf("health: register pool collector: %w", err)
	}
	return c, nil
}

// Describe sends no descriptor, marking this collector unchecked — its
// emitted metric set depends on the accessor's return value at scrape
// time, which the registry cannot know in advance.
func (c *PoolCollector) Describe(_ chan<- *prometheus.Desc) {}

// Collect implements prometheus.Collector: it calls the accessor once
// and emits nothing when the accessor returns nil. A construction
// failure from prometheus.NewConstMetric is reported to the registry via
// prometheus.NewInvalidMetric rather than swallowed or panicked.
func (c *PoolCollector) Collect(ch chan<- prometheus.Metric) {
	stat := c.stat()
	if stat == nil {
		return
	}

	emit := func(desc *prometheus.Desc, valueType prometheus.ValueType, value float64, labelValues ...string) {
		m, err := prometheus.NewConstMetric(desc, valueType, value, labelValues...)
		if err != nil {
			ch <- prometheus.NewInvalidMetric(desc, err)
			return
		}
		ch <- m
	}

	emit(descPoolConns, prometheus.GaugeValue, float64(stat.IdleConns()), poolStateIdle)
	emit(descPoolConns, prometheus.GaugeValue, float64(stat.AcquiredConns()), poolStateAcquired)
	emit(descPoolConns, prometheus.GaugeValue, float64(stat.ConstructingConns()), poolStateConstructing)
	emit(descPoolTotalConns, prometheus.GaugeValue, float64(stat.TotalConns()))
	emit(descPoolMaxConns, prometheus.GaugeValue, float64(stat.MaxConns()))
	emit(descPoolAcquires, prometheus.CounterValue, float64(stat.AcquireCount()))
	emit(descPoolAcquireDuration, prometheus.CounterValue, stat.AcquireDuration().Seconds())
	emit(descPoolEmptyAcquires, prometheus.CounterValue, float64(stat.EmptyAcquireCount()))
	emit(descPoolEmptyAcquireWait, prometheus.CounterValue, stat.EmptyAcquireWaitTime().Seconds())
	emit(descPoolCanceledAcquires, prometheus.CounterValue, float64(stat.CanceledAcquireCount()))
	emit(descPoolNewConns, prometheus.CounterValue, float64(stat.NewConnsCount()))
	emit(descPoolMaxLifetimeDestroys, prometheus.CounterValue, float64(stat.MaxLifetimeDestroyCount()))
	emit(descPoolMaxIdleDestroys, prometheus.CounterValue, float64(stat.MaxIdleDestroyCount()))
}
