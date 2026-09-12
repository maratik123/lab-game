package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// alwaysAlive is a prober that always reports the server alive.
func alwaysAlive(context.Context, string) error { return nil }

// alwaysDead is a prober that always reports the same probe error.
func alwaysDead(err error) prober {
	return func(context.Context, string) error { return err }
}

// writeLog writes body to a file named name under the test's own
// directory and returns its path.
func writeLog(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

const cleanLog = "ok  \tgithub.com/maratik123/lab-game/internal/store\t8.7s\n"

// TestRun_classifiesTheInstrumentBeforeTheGateStatus locks the property
// the classifier exists for: a run whose shared server died is neither a
// pass nor a finding, whatever killed it, and is reported apart from the
// classified gate's own verdict.
func TestRun_classifiesTheInstrumentBeforeTheGateStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// race and load are the two child logs, verbatim.
		race string
		load string
		// status is the classified gate's own exit status.
		status int
		// want is the status the classifier must return, and wantClass a
		// substring its report must carry so that one class of death can
		// never be reported as another.
		want      int
		wantClass string
	}{
		{
			name:   "returns_the_gate_status_when_both_logs_are_clean_and_the_gate_passed",
			race:   cleanLog,
			load:   cleanLog,
			status: 0,
			want:   0,
		},
		{
			name:   "returns_the_gate_status_when_the_gate_failed_under_a_sound_instrument",
			race:   "--- FAIL: TestSomethingRacy (0.42s)\n",
			load:   cleanLog,
			status: 1,
			want:   1,
		},
		{
			name:      "reports_instrument_failure_when_the_server_ran_out_of_connections",
			race:      cleanLog,
			load:      "failed to connect: FATAL: sorry, too many clients already (SQLSTATE 53300)\n",
			status:    1,
			want:      exitInstrument,
			wantClass: "ran out of connections",
		},
		{
			name:      "reports_instrument_failure_when_the_connection_ceiling_is_named_by_sqlstate_alone",
			race:      "pq: SQLSTATE 53300\n",
			load:      cleanLog,
			status:    1,
			want:      exitInstrument,
			wantClass: "ran out of connections",
		},
		{
			name:      "reports_instrument_failure_when_the_server_ran_out_of_disk",
			race:      cleanLog,
			load:      `could not extend file "base/16384/1249": No space left on device (SQLSTATE 53100)` + "\n",
			status:    1,
			want:      exitInstrument,
			wantClass: "ran out of disk",
		},
		{
			name:      "reports_instrument_failure_when_a_write_hit_no_space_left_on_device",
			race:      "PANIC: could not write to file \"pg_wal/xlogtemp.42\": No space left on device\n",
			load:      cleanLog,
			status:    1,
			want:      exitInstrument,
			wantClass: "ran out of disk",
		},
		{
			name:      "reports_instrument_failure_when_the_server_was_in_crash_recovery",
			race:      "failed to connect: FATAL: SQLSTATE 57P03\n",
			load:      cleanLog,
			status:    1,
			want:      exitInstrument,
			wantClass: "crash recovery",
		},
		{
			name:      "reports_instrument_failure_when_the_recovery_mode_message_appears",
			race:      cleanLog,
			load:      "FATAL: the database system is in recovery mode\n",
			status:    1,
			want:      exitInstrument,
			wantClass: "crash recovery",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			race := writeLog(t, "race.log", tc.race)
			load := writeLog(t, "load.log", tc.load)

			var stdout, stderr bytes.Buffer
			got := run([]string{"-status", strconv.Itoa(tc.status), "-dsn", "postgres://irrelevant", race, load}, alwaysAlive, &stdout, &stderr)

			if got != tc.want {
				t.Errorf("run = %d, want %d; stdout: %s stderr: %s", got, tc.want, stdout.String(), stderr.String())
			}
			if tc.wantClass != "" && !strings.Contains(stdout.String(), tc.wantClass) {
				t.Errorf("report = %q, want it to name the class %q — a death reported as the wrong class sends the reader at innocent code", stdout.String(), tc.wantClass)
			}
		})
	}
}

// TestRun_unreadableLog_isAnInstrumentFailure keeps the scan from
// reporting clean over a log it never read.
func TestRun_unreadableLog_isAnInstrumentFailure(t *testing.T) {
	t.Parallel()

	race := writeLog(t, "race.log", cleanLog)
	missing := filepath.Join(t.TempDir(), "load.log")

	var stdout, stderr bytes.Buffer
	if got := run([]string{"-status", "0", "-dsn", "postgres://irrelevant", race, missing}, alwaysAlive, &stdout, &stderr); got != exitInstrument {
		t.Errorf("run = %d, want %d; stdout: %s stderr: %s", got, exitInstrument, stdout.String(), stderr.String())
	}
}

// TestRun_missingStatus_isAnInstrumentFailure refuses a run whose gate
// status was never passed in: returning 0 for it would report a pass.
func TestRun_missingStatus_isAnInstrumentFailure(t *testing.T) {
	t.Parallel()

	race := writeLog(t, "race.log", cleanLog)

	var stdout, stderr bytes.Buffer
	if got := run([]string{"-dsn", "postgres://irrelevant", race}, alwaysAlive, &stdout, &stderr); got != exitInstrument {
		t.Errorf("run = %d, want %d; stdout: %s stderr: %s", got, exitInstrument, stdout.String(), stderr.String())
	}
}

// TestRun_missingDSN_isAnInstrumentFailure refuses a run whose server DSN
// was never passed in: a run whose liveness cannot be established says
// nothing about contention, and returning the gate status for it would
// report a pass.
func TestRun_missingDSN_isAnInstrumentFailure(t *testing.T) {
	t.Parallel()

	race := writeLog(t, "race.log", cleanLog)
	load := writeLog(t, "load.log", cleanLog)

	var stdout, stderr bytes.Buffer
	if got := run([]string{"-status", "0", race, load}, alwaysAlive, &stdout, &stderr); got != exitInstrument {
		t.Errorf("run = %d, want %d; stdout: %s stderr: %s", got, exitInstrument, stdout.String(), stderr.String())
	}
}

// TestRun_deadServer_isAnInstrumentFailure_regardlessOfGateStatus checks
// that a server that stopped answering after the run is reported as an
// instrument failure even when the classified gate itself reported a
// pass: a "passed" run against a dead server is not a pass.
func TestRun_deadServer_isAnInstrumentFailure_regardlessOfGateStatus(t *testing.T) {
	t.Parallel()

	probeErr := errors.New("dial tcp: connection refused")

	for _, status := range []int{0, 1} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			t.Parallel()

			race := writeLog(t, "race.log", cleanLog)
			load := writeLog(t, "load.log", cleanLog)

			var stdout, stderr bytes.Buffer
			got := run([]string{"-status", strconv.Itoa(status), "-dsn", "postgres://irrelevant", race, load}, alwaysDead(probeErr), &stdout, &stderr)

			if got != exitInstrument {
				t.Errorf("run = %d, want %d; stdout: %s stderr: %s", got, exitInstrument, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), "not answering") {
				t.Errorf("report = %q, want it to say the server was not answering", stdout.String())
			}
		})
	}
}

// TestRun_liveServer_passesTheGateStatusThrough confirms clean logs and a
// live server let the classified gate's own status through unaltered.
func TestRun_liveServer_passesTheGateStatusThrough(t *testing.T) {
	t.Parallel()

	race := writeLog(t, "race.log", cleanLog)
	load := writeLog(t, "load.log", cleanLog)

	var stdout, stderr bytes.Buffer
	if got := run([]string{"-status", "1", "-dsn", "postgres://irrelevant", race, load}, alwaysAlive, &stdout, &stderr); got != 1 {
		t.Errorf("run = %d, want 1; stdout: %s stderr: %s", got, stdout.String(), stderr.String())
	}
}

// TestWithRetry_retriesUntilSuccess confirms a probe that fails a bounded
// number of times before succeeding is still reported alive.
func TestWithRetry_retriesUntilSuccess(t *testing.T) {
	t.Parallel()

	var calls int
	probe := func(context.Context, string) error {
		calls++
		if calls < 3 {
			return errors.New("not yet")
		}
		return nil
	}

	if err := withRetry(probe, 3, time.Microsecond)(context.Background(), "postgres://irrelevant"); err != nil {
		t.Errorf("withRetry()() = %v, want nil", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

// TestWithRetry_returnsTheLastErrorWhenEveryAttemptFails confirms the
// retry gives up after the stated number of attempts and reports the
// final attempt's own error.
func TestWithRetry_returnsTheLastErrorWhenEveryAttemptFails(t *testing.T) {
	t.Parallel()

	var calls int
	wantErr := errors.New("attempt failure")
	probe := func(context.Context, string) error {
		calls++
		return fmt.Errorf("call %d: %w", calls, wantErr)
	}

	err := withRetry(probe, 3, time.Microsecond)(context.Background(), "postgres://irrelevant")
	if !errors.Is(err, wantErr) {
		t.Errorf("withRetry()() = %v, want it to wrap %v", err, wantErr)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}
