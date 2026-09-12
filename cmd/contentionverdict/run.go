package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// exitInstrument is the status that says the run's INSTRUMENT failed
// rather than its subject: the classification names the shared server,
// or it could not be made at all. The caller distinguishes it from the
// classified gate's own 0 and 1, which pass through unaltered.
const exitInstrument = 2

// noStatus is the -status default. The flag is required — a run whose
// gate status was not passed in cannot be reported, and defaulting it to
// 0 would report a pass for it.
const noStatus = -1

// probeAttempts is how many times a failing liveness probe is retried. The
// probe runs after the load loop has already been killed, so a merely
// saturated server has had a moment to start shedding load; a bounded
// retry is what separates that from a server that is actually gone.
const probeAttempts = 3

// probeRetryDelay is the pause between liveness-probe attempts. The whole
// retry budget stays small beside the run it classifies.
const probeRetryDelay = 2 * time.Second

// probeTimeout bounds a single liveness probe so a server that accepts a
// connection but never answers cannot hang the classification itself.
const probeTimeout = 10 * time.Second

// signature is one literal that, found in a child log, names a class of
// instrument failure. The literal is matched and never parsed: these are
// the server's own message texts and the SQLSTATE codes as the driver
// prints them.
type signature struct {
	literal string
	class   string
}

// signatures is the vocabulary the child logs are scanned for. Every
// entry names a way the shared server itself can fail, never a way a
// test can fail.
var signatures = []signature{
	{literal: "sorry, too many clients already", class: "the server ran out of connections"},
	{literal: "SQLSTATE 53300", class: "the server ran out of connections"},
	{literal: "No space left on device", class: "the server ran out of disk"},
	{literal: "SQLSTATE 53100", class: "the server ran out of disk"},
	{literal: "SQLSTATE 57P03", class: "the server was in crash recovery or shutting down"},
	{literal: "the database system is in recovery mode", class: "the server was in crash recovery or shutting down"},
}

// prober reports whether the shared server named by dsn is answering.
type prober func(ctx context.Context, dsn string) error

// scanLogs returns the first signature that any of paths contains. A
// path it cannot read is an error rather than a clean result: a
// classification made over a log that was never read is a claim about
// the read.
func scanLogs(paths []string) (signature, bool, error) {
	for _, p := range paths {
		b, err := os.ReadFile(p) //nolint:gosec // paths are this command's own argument vector, the child logs it was invoked to classify, never external input
		if err != nil {
			return signature{}, false, fmt.Errorf("read %s: %w", p, err)
		}
		text := string(b)
		for _, s := range signatures {
			if strings.Contains(text, s.literal) {
				return s, true, nil
			}
		}
	}
	return signature{}, false, nil
}

// withRetry decorates probe so that a failing attempt is retried up to
// attempts times, waiting delay between tries: the first success wins and
// the last error is what a total failure reports. The retry is applied
// where the real probe is built rather than inside the classification, so
// a caller injecting its own probe pays no wall clock for a budget it is
// not exercising.
func withRetry(probe prober, attempts int, delay time.Duration) prober {
	return func(ctx context.Context, dsn string) error {
		var err error
		for attempt := 0; attempt < attempts; attempt++ {
			if attempt > 0 {
				time.Sleep(delay)
			}
			if err = probe(ctx, dsn); err == nil {
				return nil
			}
		}
		return err
	}
}

// run classifies one contention run. args is the argument vector without
// the command name: the flags, then the child logs to scan. probe is the
// liveness check run against the shared server named by -dsn.
func run(args []string, probe prober, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("contentionverdict", flag.ContinueOnError)
	fs.SetOutput(stderr)
	status := fs.Int("status", noStatus, "the classified gate's own exit status, returned unaltered when the instrument is sound")
	dsn := fs.String("dsn", "", "the shared server's DSN, probed for liveness after the run")
	if err := fs.Parse(args); err != nil {
		return exitInstrument
	}

	logs := fs.Args()
	if *status == noStatus || len(logs) == 0 {
		logf(stderr, "contentionverdict: usage: contentionverdict -status N -dsn DSN <log> [log...]\n")
		return exitInstrument
	}

	if *dsn == "" {
		logf(stdout, "test-contention: INSTRUMENT FAILURE — no server DSN was passed, so this run's instrument cannot be shown to have survived it\n")
		return exitInstrument
	}

	sig, found, err := scanLogs(logs)
	if err != nil {
		logf(stdout, "test-contention: INSTRUMENT FAILURE — the classification itself failed (%v), so a clean result would be a claim about the classification\n", err)
		return exitInstrument
	}
	if found {
		logf(stdout, "test-contention: INSTRUMENT FAILURE — %s, so this run says nothing about contention either way\n", sig.class)
		return exitInstrument
	}

	if err := probe(context.Background(), *dsn); err != nil {
		logf(stdout, "test-contention: INSTRUMENT FAILURE — the server was not answering after the run (%v), so this run says nothing about contention either way\n", err)
		return exitInstrument
	}

	logf(stdout, "test-contention: instrument sound — logs clean, server answering\n")
	return *status
}

// logf writes a report line, discarding the write error: a classifier
// that failed to print its verdict still has to return it.
func logf(w io.Writer, format string, a ...any) {
	_, _ = fmt.Fprintf(w, format, a...)
}
