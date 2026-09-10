package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"golang.org/x/sync/errgroup"
)

// runnerResult is one runner's terminal outcome, published as it
// returns so serve can select on it beside the registered signal
// channel and a late return never blocks a serve that has already
// moved on to draining.
type runnerResult struct {
	name string
	err  error
}

// serve starts every runner, waits for whichever comes first — a
// signal on a.signals, or the first runner to return on its own — and
// then drains. A runner returning before any signal is itself a
// shutdown trigger: it begins the same graceful shutdown a signal
// begins, reports its own name and error on stderr, and makes the exit
// code non-zero. Returns the process exit code.
func (a *app) serve(ctx context.Context, shutdownTimeout time.Duration, stderr io.Writer) int {
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	results := make(chan runnerResult, len(a.runners))
	var g errgroup.Group
	for _, r := range a.runners {
		g.Go(func() error {
			err := r.run(runCtx)
			results <- runnerResult{name: r.name, err: err}
			return err
		})
	}

	remaining := len(a.runners)
	var trigger *runnerResult
	select {
	case <-a.signals:
	case res := <-results:
		remaining--
		trigger = &res
	}

	return a.drain(ctx, cancelRun, shutdownTimeout, &g, results, remaining, trigger, stderr)
}

// drain runs the one written shutdown order: latch readiness.draining
// first — ahead of every runner.stop and every closer, so /readyz
// answers not-ready from the moment shutdown begins — call each
// runner's stop, join the group (cancelling the run context if the
// budget expires or a second signal arrives before the join
// completes), then walk the closer list from the last appended to the
// first. trigger, when non-nil, is the runner whose own unprompted
// return began this drain; remaining is how many more runner results
// are still owed on results once every runner has settled.
func (a *app) drain(
	ctx context.Context,
	cancelRun context.CancelFunc,
	budget time.Duration,
	g *errgroup.Group,
	results chan runnerResult,
	remaining int,
	trigger *runnerResult,
	stderr io.Writer,
) int {
	a.readiness.setDraining()

	for _, r := range a.runners {
		r.stop()
	}

	joined := make(chan struct{})
	go func() {
		_ = g.Wait()
		close(joined)
	}()

	abandoned := false
	timer := time.NewTimer(budget)
	defer timer.Stop()

	select {
	case <-joined:
	case <-timer.C:
		abandoned = true
		cancelRun()
		<-joined
	case <-a.signals:
		abandoned = true
		cancelRun()
		<-joined
	}

	exitCode := 0
	if abandoned {
		exitCode = 1
	}
	if trigger != nil {
		logStep(stderr, trigger.name, trigger.err)
		exitCode = 1
	}
	for range remaining {
		res := <-results
		if res.err != nil {
			logStep(stderr, res.name, res.err)
			exitCode = 1
		}
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	for i := len(a.closers) - 1; i >= 0; i-- {
		c := a.closers[i]
		if err := c.close(shutdownCtx); err != nil {
			// A closer's error is a diagnostic, never a second verdict: the
			// process is already leaving, and the exit code carries only
			// whether the drain itself completed (budget honoured, no second
			// signal). The liveness final write is the clearest instance —
			// the next start measures its gap from the last successful
			// heartbeat, so a lost final write is self-healing — but every
			// closer is the same case: reported by name here, on stderr, and
			// nothing more.
			logStep(stderr, c.name, err)
		}
	}
	return exitCode
}

// logStep writes one "lab-game bot: <name>: <cause>" diagnostic line,
// discarding the write error: a broken stderr leaves nothing more this
// process can do about it, and the exit code already reflects what
// happened.
func logStep(stderr io.Writer, name string, cause error) {
	_, _ = fmt.Fprintf(stderr, "lab-game bot: %s: %v\n", name, cause)
}
