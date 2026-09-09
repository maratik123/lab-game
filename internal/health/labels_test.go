package health

import (
	"testing"

	"github.com/maratik123/lab-game/internal/ingest"
	"github.com/maratik123/lab-game/internal/scheduler"
)

func TestSchedulerOutcomeLabel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   scheduler.Outcome
		want string
	}{
		{"done", scheduler.OutcomeDone, "done"},
		{"noop", scheduler.OutcomeNoop, "noop"},
		{"failed", scheduler.OutcomeFailed, "failed"},
		{"out of range", scheduler.Outcome(99), unknownLabelValue},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := schedulerOutcomeLabel(tc.in); got != tc.want {
				t.Errorf("schedulerOutcomeLabel(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSchedulerFailureLabel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   scheduler.FailureKind
		want string
	}{
		{"none", scheduler.FailureNone, "none"},
		{"handler", scheduler.FailureHandler, "handler"},
		{"unregistered", scheduler.FailureUnregistered, "unregistered"},
		{"deadline", scheduler.FailureDeadline, "deadline"},
		{"rolled back", scheduler.FailureRolledBack, "rolled_back"},
		{"out of range", scheduler.FailureKind(99), unknownLabelValue},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := schedulerFailureLabel(tc.in); got != tc.want {
				t.Errorf("schedulerFailureLabel(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestIngestOutcomeLabel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   ingest.Outcome
		want string
	}{
		{"handled", ingest.OutcomeHandled, "handled"},
		{"duplicate", ingest.OutcomeDuplicate, "duplicate"},
		{"unrouted", ingest.OutcomeUnrouted, "unrouted"},
		{"failed", ingest.OutcomeFailed, "failed"},
		{"panic", ingest.OutcomePanic, "panic"},
		{"given up", ingest.OutcomeGivenUp, "given_up"},
		{"out of range", ingest.Outcome(99), unknownLabelValue},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ingestOutcomeLabel(tc.in); got != tc.want {
				t.Errorf("ingestOutcomeLabel(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestIngestKindLabel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   ingest.Kind
		want string
	}{
		{"message", ingest.KindMessage, "message"},
		{"callback query", ingest.KindCallbackQuery, "callback_query"},
		{"empty (unrouted)", ingest.Kind(""), unknownLabelValue},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ingestKindLabel(tc.in); got != tc.want {
				t.Errorf("ingestKindLabel(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
