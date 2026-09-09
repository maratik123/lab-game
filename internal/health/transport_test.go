package health

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/maratik123/lab-game/internal/tg"
)

func newTransportObserverForTest(t *testing.T) *TransportObserver {
	t.Helper()
	o, err := NewTransportObserver(NewRegistry())
	if err != nil {
		t.Fatalf("NewTransportObserver: %v", err)
	}
	return o
}

func TestTransportObserver_SuccessfulCall(t *testing.T) {
	t.Parallel()
	o := newTransportObserverForTest(t)
	o.ObserveCall(tg.Observation{Method: "getMe", Latency: 100 * time.Millisecond, StatusCode: 200})

	if count := testutil.CollectAndCount(o.callDuration); count != 1 {
		t.Errorf("callDuration count = %d, want 1", count)
	}
	want := `
# HELP labgame_botapi_responses_total Outbound Bot API call responses, by method and HTTP status code (0 = no response received).
# TYPE labgame_botapi_responses_total counter
labgame_botapi_responses_total{code="200",method="getMe"} 1
`
	if err := testutil.CollectAndCompare(o.responses, strings.NewReader(want)); err != nil {
		t.Errorf("responses: %v", err)
	}
	if count := testutil.CollectAndCount(o.rateLimited); count != 0 {
		t.Errorf("rateLimited count = %d, want 0 (never touched on a non-rate-limited call)", count)
	}
	if count := testutil.CollectAndCount(o.retries); count != 0 {
		t.Errorf("retries count = %d, want 0 (never touched when Retries is 0)", count)
	}
}

func TestTransportObserver_RateLimitedCall(t *testing.T) {
	t.Parallel()
	o := newTransportObserverForTest(t)
	o.ObserveCall(tg.Observation{Method: "sendMessage", StatusCode: 429, RateLimited: true})

	want := `
# HELP labgame_botapi_rate_limited_total Outbound Bot API calls that received a 429 on any attempt, by method.
# TYPE labgame_botapi_rate_limited_total counter
labgame_botapi_rate_limited_total{method="sendMessage"} 1
`
	if err := testutil.CollectAndCompare(o.rateLimited, strings.NewReader(want)); err != nil {
		t.Errorf("rateLimited: %v", err)
	}
}

func TestTransportObserver_CallWithRetries(t *testing.T) {
	t.Parallel()
	o := newTransportObserverForTest(t)
	o.ObserveCall(tg.Observation{Method: "getMe", StatusCode: 200, Retries: 3})

	want := `
# HELP labgame_botapi_retries_total Attempts beyond the first an outbound Bot API call consumed, by method.
# TYPE labgame_botapi_retries_total counter
labgame_botapi_retries_total{method="getMe"} 3
`
	if err := testutil.CollectAndCompare(o.retries, strings.NewReader(want)); err != nil {
		t.Errorf("retries: %v", err)
	}
}

func TestTransportObserver_NoResponseRecordsCodeZero(t *testing.T) {
	t.Parallel()
	o := newTransportObserverForTest(t)
	o.ObserveCall(tg.Observation{Method: "getMe", StatusCode: 0})

	want := `
# HELP labgame_botapi_responses_total Outbound Bot API call responses, by method and HTTP status code (0 = no response received).
# TYPE labgame_botapi_responses_total counter
labgame_botapi_responses_total{code="0",method="getMe"} 1
`
	if err := testutil.CollectAndCompare(o.responses, strings.NewReader(want)); err != nil {
		t.Errorf("responses: %v", err)
	}
}

func TestTransportObserver_DifferentMethodsStayInSeparateSeries(t *testing.T) {
	t.Parallel()
	o := newTransportObserverForTest(t)
	o.ObserveCall(tg.Observation{Method: "getMe", StatusCode: 200})
	o.ObserveCall(tg.Observation{Method: "sendMessage", StatusCode: 200})

	want := `
# HELP labgame_botapi_responses_total Outbound Bot API call responses, by method and HTTP status code (0 = no response received).
# TYPE labgame_botapi_responses_total counter
labgame_botapi_responses_total{code="200",method="getMe"} 1
labgame_botapi_responses_total{code="200",method="sendMessage"} 1
`
	if err := testutil.CollectAndCompare(o.responses, strings.NewReader(want)); err != nil {
		t.Errorf("responses: %v", err)
	}
	if count := testutil.CollectAndCount(o.callDuration); count != 2 {
		t.Errorf("callDuration count = %d, want 2 (one per method)", count)
	}
}
