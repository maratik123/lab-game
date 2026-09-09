package health

import (
	"fmt"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/maratik123/lab-game/internal/tg"
)

// TransportObserver is the Telegram transport's Observer implementation,
// exporting every field of a transport observation: this package carries
// no transport exemption.
type TransportObserver struct {
	callDuration *prometheus.HistogramVec
	responses    *prometheus.CounterVec
	rateLimited  *prometheus.CounterVec
	retries      *prometheus.CounterVec
}

// NewTransportObserver registers the transport families on reg and
// returns the observer to install as a Telegram client's Observer.
func NewTransportObserver(reg prometheus.Registerer) (*TransportObserver, error) {
	o := &TransportObserver{
		callDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    familyBotAPICallDuration,
			Help:    "Whole outbound Bot API call latency in seconds, limiter and backoff waits included.",
			Buckets: durationBuckets,
		}, []string{labelMethod}),
		responses: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: familyBotAPIResponses,
			Help: "Outbound Bot API call responses, by method and HTTP status code (0 = no response received).",
		}, []string{labelMethod, labelCode}),
		rateLimited: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: familyBotAPIRateLimited,
			Help: "Outbound Bot API calls that received a 429 on any attempt, by method.",
		}, []string{labelMethod}),
		retries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: familyBotAPIRetries,
			Help: "Attempts beyond the first an outbound Bot API call consumed, by method.",
		}, []string{labelMethod}),
	}
	for _, c := range []prometheus.Collector{o.callDuration, o.responses, o.rateLimited, o.retries} {
		if err := reg.Register(c); err != nil {
			return nil, fmt.Errorf("health: register transport family: %w", err)
		}
	}
	return o, nil
}

// ObserveCall implements the transport's Observer interface.
func (o *TransportObserver) ObserveCall(obs tg.Observation) {
	o.callDuration.WithLabelValues(obs.Method).Observe(obs.Latency.Seconds())
	o.responses.WithLabelValues(obs.Method, strconv.Itoa(obs.StatusCode)).Inc()
	if obs.RateLimited {
		o.rateLimited.WithLabelValues(obs.Method).Inc()
	}
	if obs.Retries > 0 {
		o.retries.WithLabelValues(obs.Method).Add(float64(obs.Retries))
	}
}
