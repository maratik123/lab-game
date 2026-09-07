package testdb

import (
	"context"
	"errors"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// stubContainer is a testcontainers.Container that only answers Terminate;
// retryRun calls nothing else on the container a failed attempt returns, so
// the embedded interface stays nil and any other call would panic loudly
// rather than pass silently.
type stubContainer struct {
	testcontainers.Container
	terminated bool
}

func (s *stubContainer) Terminate(_ context.Context, _ ...testcontainers.TerminateOption) error {
	s.terminated = true
	return nil
}

func TestRetryRun(t *testing.T) {
	t.Parallel()

	errStart := errors.New("start failed")

	tests := []struct {
		name      string
		attempts  int
		failures  int
		wantCalls int
		wantErr   error
	}{
		{name: "succeeds_on_the_first_attempt", attempts: 3, failures: 0, wantCalls: 1},
		{name: "succeeds_after_transient_failures", attempts: 3, failures: 2, wantCalls: 3},
		{name: "returns_the_last_error_when_every_attempt_fails", attempts: 3, failures: 3, wantCalls: 3, wantErr: errStart},
		{name: "single_attempt_never_retries", attempts: 1, failures: 1, wantCalls: 1, wantErr: errStart},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			calls := 0
			got, err := retryRun(tc.attempts, 0, func() (*postgres.PostgresContainer, error) {
				calls++
				if calls <= tc.failures {
					return nil, errStart
				}
				return nil, nil
			})

			if calls != tc.wantCalls {
				t.Errorf("run called %d times, want %d", calls, tc.wantCalls)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil && got != nil {
				t.Errorf("container = %v, want nil on failure", got)
			}
		})
	}
}

// TestRetryRun_terminatesTheContainerOfAFailedAttempt pins the cleanup a
// retry depends on: postgres.Run reports both a container and an error when
// the container was created but never became usable, and leaving it running
// would leak one container per retried attempt.
func TestRetryRun_terminatesTheContainerOfAFailedAttempt(t *testing.T) {
	t.Parallel()

	stub := &stubContainer{}
	failed := &postgres.PostgresContainer{Container: stub}

	_, err := retryRun(1, 0, func() (*postgres.PostgresContainer, error) {
		return failed, errors.New("start failed")
	})

	if err == nil {
		t.Fatal("retryRun returned no error, want the attempt's error")
	}
	if !stub.terminated {
		t.Error("the failed attempt's container was not terminated")
	}
}
