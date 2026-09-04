package tg

import "time"

// Observation is what this package reports to an Observer exactly once
// per outbound call — after the last attempt returns, including a gate
// refusal — carrying the four things §13.2's health dashboard asks of the
// transport (design D11).
type Observation struct {
	// Method is the Bot API method name.
	Method string
	// Latency is the whole call as the caller experienced it: limiter
	// waits and backoff waits included.
	Latency time.Duration
	// StatusCode is the last HTTP status code received, or 0 when no
	// response was ever received (including a gate refusal).
	StatusCode int
	// RateLimited is true when any attempt of this call received a 429 —
	// not only when the final response was one, since a 429-then-success
	// call is exactly the event §13.2's 429 counter wants counted.
	RateLimited bool
	// Retries is the number of attempts beyond the first this call
	// consumed.
	Retries int
}

// Observer receives one Observation per outbound call. Implementations
// must not block the caller for long — ObserveCall runs on the calling
// goroutine, after the call's own outcome is already decided.
type Observer interface {
	// ObserveCall reports one completed outbound call.
	ObserveCall(Observation)
}
