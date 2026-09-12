package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maratik123/lab-game/internal/config"
	"github.com/maratik123/lab-game/internal/health"
	"github.com/maratik123/lab-game/internal/ingest"
	"github.com/maratik123/lab-game/internal/scheduler"
	"github.com/maratik123/lab-game/internal/store"
	"github.com/maratik123/lab-game/internal/tg"
)

// pingTimeout bounds the start-up database step's Ping round trip. A
// named constant, not a configuration key: a database that has not
// answered a trivial round trip in this long is not one this process
// can serve against, and the all-fatal start-up rule wants the failure
// now rather than after an unbounded hang.
const pingTimeout = 5 * time.Second

// runner is one of the long-running subsystems serve starts and drain
// stops: the ingest loop, the scheduler worker, the liveness heartbeat.
// name is what a failure message and a drain timeout report.
type runner struct {
	name string
	run  func(ctx context.Context) error
	stop func()
}

// closer is the shutdown seam, and the one place the shutdown order is
// written down: assemble appends an entry the moment a step takes
// something that has to be given back, and both assemble's own unwind
// and drain walk the same list backwards.
type closer struct {
	name  string
	close func(ctx context.Context) error
}

// app is the assembled process: the pool, the logger, the registry,
// every constructed subsystem, the registered signal channel, the
// runner set and the closer list.
type app struct {
	logger    *slog.Logger
	pool      *pgxpool.Pool
	readiness *readiness
	healthSrv *health.Server
	cfg       *config.Config

	// client, router, taskRegistry and httpClient are the constructed
	// subsystems a test observes directly, independent of the
	// runner/closer seam: the Telegram client every outbound call passes
	// through, the router/registry the ingest loop and the scheduler
	// worker were each constructed from, and the process HTTP client
	// threaded into both the Telegram client and the canary legs — never
	// re-derived from what New was called with.
	client       *tg.Client
	router       *ingest.Router
	taskRegistry *scheduler.Registry
	httpClient   *http.Client

	signals chan os.Signal

	runners []runner
	closers []closer
}

// assembleOptions configures assemble.
type assembleOptions struct {
	// Lookup is the environment lookup the configuration step reads
	// through.
	Lookup config.Lookup
	// Stderr is the writer the process logger and the build identity
	// line are written to.
	Stderr io.Writer
	// Version is the build identity, exported as labgame_build_info's
	// version label value.
	Version string
	// StartedAt is the instant assemble began, exported as
	// labgame_start_time_seconds.
	StartedAt time.Time
	// HTTPClient, when non-nil, is threaded into the Telegram client and
	// both canary legs — the process-wide client, or a test's own fake
	// server client.
	HTTPClient *http.Client
}

// stepError names the start-up step that failed, so a message on
// stderr can report both without a caller re-deriving the step from
// the underlying error's text.
type stepError struct {
	step  string
	cause error
}

// Error renders "<step>: <cause>".
func (e *stepError) Error() string {
	return fmt.Sprintf("%s: %v", e.step, e.cause)
}

// Unwrap returns the underlying cause, so errors.Is/errors.As over a
// failed assemble's error still reach it.
func (e *stepError) Unwrap() error {
	return e.cause
}

// unwind walks a's closers from the last appended to the first. A
// failing closer is reported on stderr by name — the same
// logStep drain itself uses for a shutdown closer failure — but never
// stops the rest of the unwind: a listener or a pool that failed to
// release during start-up is exactly the kind of operational fact an
// operator needs on stderr, the same reasoning drain already applies to
// every one of its own closer failures. unwind returns the step/cause
// pair as a *stepError. Called once assemble has decided to fail.
func unwind(ctx context.Context, a *app, stderr io.Writer, step string, cause error) (*app, error) {
	for i := len(a.closers) - 1; i >= 0; i-- {
		c := a.closers[i]
		if err := c.close(ctx); err != nil {
			logStep(stderr, c.name, err)
		}
	}
	return nil, &stepError{step: step, cause: cause}
}

// assemble builds the whole process in one fixed, all-fatal, unwinding
// order: signal registration, configuration, logger, database,
// readiness, metrics registry, health listener, migrations, restart
// hygiene, scheduler, Telegram client, ingest loop, canary, runners. On
// any failure it walks the closers it has appended so far backwards,
// then returns the error naming the step. ctx bounds every step;
// assembly itself is not interruptible by a signal — the registration
// exists to buffer one for serve, not to
// cancel assembly.
func assemble(ctx context.Context, opts assembleOptions) (*app, error) {
	a := &app{}

	// Step 1: signal registration — first, ahead of everything
	// observable, so SIGINT/SIGTERM are buffered rather than fatal for
	// the rest of start-up.
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	a.signals = sigCh
	a.closers = append(a.closers, closer{
		name: "signal registration",
		close: func(context.Context) error {
			signal.Stop(sigCh)
			return nil
		},
	})

	// Step 2: configuration.
	cfg, err := config.Load(opts.Lookup)
	if err != nil {
		return unwind(ctx, a, opts.Stderr, "configuration", err)
	}
	a.cfg = cfg

	// Step 3: logger.
	logger := slog.New(slog.NewTextHandler(opts.Stderr, nil))
	a.logger = logger

	// Step 4: database.
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN.Reveal())
	if err != nil {
		return unwind(ctx, a, opts.Stderr, "database", err)
	}
	pool, err := store.NewPool(ctx, poolCfg)
	if err != nil {
		return unwind(ctx, a, opts.Stderr, "database", err)
	}
	a.pool = pool
	a.closers = append(a.closers, closer{
		name: "pool",
		close: func(context.Context) error {
			pool.Close()
			return nil
		},
	})
	pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
	err = pool.Ping(pingCtx)
	cancel()
	if err != nil {
		return unwind(ctx, a, opts.Stderr, "database", err)
	}

	// Step 5: readiness — before the registry, since NewProcess takes
	// the Ready func the labgame_ready gauge calls.
	ready := newReadiness(pool)
	a.readiness = ready

	// Step 6: metrics registry.
	reg := health.NewRegistry()
	if err := health.RegisterRuntime(reg); err != nil {
		return unwind(ctx, a, opts.Stderr, "metrics registry", err)
	}
	transportObs, err := health.NewTransportObserver(reg)
	if err != nil {
		return unwind(ctx, a, opts.Stderr, "metrics registry", err)
	}
	schedulerObs, err := health.NewSchedulerObserver(reg)
	if err != nil {
		return unwind(ctx, a, opts.Stderr, "metrics registry", err)
	}
	ingestObs, err := health.NewIngestObserver(reg)
	if err != nil {
		return unwind(ctx, a, opts.Stderr, "metrics registry", err)
	}
	if _, err := health.NewPoolCollector(reg, pool.Stat); err != nil {
		return unwind(ctx, a, opts.Stderr, "metrics registry", err)
	}
	process, err := health.NewProcess(reg, health.ProcessOptions{ //nolint:contextcheck // labgame_ready's GaugeFunc derives its own bounded context per scrape, by design — it takes no ctx here
		Version:   opts.Version,
		StartedAt: opts.StartedAt,
		Ready:     ready.Ready,
	})
	if err != nil {
		return unwind(ctx, a, opts.Stderr, "metrics registry", err)
	}

	// Step 7: health listener.
	healthSrv, err := health.NewServer(health.ServerOptions{
		Addr:     cfg.Health.MetricsAddr,
		Gatherer: reg,
		Ready:    ready.Ready,
	})
	if err != nil {
		return unwind(ctx, a, opts.Stderr, "health listener", err)
	}
	if err := healthSrv.Start(); err != nil { //nolint:contextcheck // Start binds a long-lived listener and takes no ctx by design — it outlives the assemble call that starts it
		return unwind(ctx, a, opts.Stderr, "health listener", err)
	}
	a.healthSrv = healthSrv
	a.closers = append(a.closers, closer{name: "health listener", close: healthSrv.Shutdown})

	// Step 8: migrations.
	if cfg.Process.MigrateOnStart {
		if err := store.Migrate(ctx, pool, logger, store.WithAdvisoryLock(store.ProcessLockID)); err != nil {
			return unwind(ctx, a, opts.Stderr, "migrations", err)
		}
	} else {
		pending, err := store.HasPendingMigrations(ctx, pool, logger)
		if err != nil {
			return unwind(ctx, a, opts.Stderr, "migrations", err)
		}
		if pending {
			return unwind(ctx, a, opts.Stderr, "migrations", fmt.Errorf("%s is disabled and migrations are pending", config.EnvProcessMigrateOnStart))
		}
	}
	ready.setMigrated()

	// Step 9: restart hygiene.
	liveness, err := scheduler.NewLiveness(scheduler.LivenessOptions{
		Pool:              pool,
		Interval:          cfg.Process.LivenessInterval,
		DowntimeThreshold: cfg.Process.DowntimeThreshold,
	})
	if err != nil {
		return unwind(ctx, a, opts.Stderr, "restart hygiene", err)
	}
	downtime, err := liveness.AbsorbDowntime(ctx)
	if err != nil {
		return unwind(ctx, a, opts.Stderr, "restart hygiene", err)
	}
	process.ObserveDowntime(downtime.Gap, downtime.Shifted)
	a.closers = append(a.closers, closer{
		name: "liveness final write",
		close: func(ctx context.Context) error {
			return liveness.Refresh(ctx)
		},
	})

	// Step 10: scheduler — Reconcile runs here, once, during assembly:
	// a reconcile failure is a start-up failure under the all-fatal
	// rule, rather than an early return from Worker.Run mid-life.
	taskRegistry, err := scheduler.NewRegistry()
	if err != nil {
		return unwind(ctx, a, opts.Stderr, "scheduler", err)
	}
	a.taskRegistry = taskRegistry
	worker, err := scheduler.New(scheduler.Options{
		Pool:     pool,
		Registry: taskRegistry,
		Config:   cfg.Scheduler,
		Observer: schedulerObs,
		Logger:   logger,
	})
	if err != nil {
		return unwind(ctx, a, opts.Stderr, "scheduler", err)
	}
	if err := worker.Reconcile(ctx); err != nil {
		return unwind(ctx, a, opts.Stderr, "scheduler", err)
	}

	// Step 11: Telegram client — the process's own HTTP client is built
	// here too, folded into this step rather than added as one of its
	// own: the caller's client is used when supplied, and otherwise a
	// clone of the default transport becomes "the process HTTP client",
	// threaded into both this client and the canary legs (step 13) and
	// released on the way out by its own closer.
	httpClient := opts.HTTPClient
	if httpClient == nil {
		defaultTransport, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			return unwind(ctx, a, opts.Stderr, "telegram client", fmt.Errorf("http.DefaultTransport is not *http.Transport, got %T", http.DefaultTransport))
		}
		httpClient = &http.Client{Transport: defaultTransport.Clone()}
	}
	a.httpClient = httpClient
	a.closers = append(a.closers, closer{
		name: "http client",
		close: func(context.Context) error {
			httpClient.CloseIdleConnections()
			return nil
		},
	})

	gate := ingest.NewPoolGate(cfg.AllowedChatIDs, pool)
	client, err := tg.New(tg.Options{
		BaseURL:    cfg.BotAPIBaseURL.String(),
		Token:      cfg.BotToken.Reveal(),
		Transport:  cfg.Transport,
		Gate:       gate,
		Observer:   transportObs,
		HTTPClient: httpClient,
	})
	if err != nil {
		return unwind(ctx, a, opts.Stderr, "telegram client", err)
	}
	a.client = client

	// Step 12: ingest loop — the router is wired empty; this task
	// declares that legal and adds no placeholder route.
	router, err := ingest.NewRouter()
	if err != nil {
		return unwind(ctx, a, opts.Stderr, "ingest loop", err)
	}
	a.router = router
	loop, err := ingest.New(ingest.Options{
		Client:   client,
		Pool:     pool,
		Router:   router,
		Config:   cfg.Ingest,
		Observer: ingestObs,
		Logger:   logger,
	})
	if err != nil {
		return unwind(ctx, a, opts.Stderr, "ingest loop", err)
	}

	// Step 13: canary.
	legs, err := health.NewLegs(health.LegsOptions{
		OwnToken:     cfg.BotToken,
		OwnBaseURL:   cfg.BotAPIBaseURL.String(),
		CloudToken:   cfg.Health.CanaryCloudToken,
		CloudBaseURL: cfg.Health.CanaryCloudBaseURL.String(),
		Transport:    cfg.Transport,
		HTTPClient:   httpClient,
	})
	if err != nil {
		return unwind(ctx, a, opts.Stderr, "canary", err)
	}
	canary, err := health.NewCanary(reg, health.CanaryOptions{
		Legs:     legs,
		Interval: cfg.Health.CanaryInterval,
	})
	if err != nil {
		return unwind(ctx, a, opts.Stderr, "canary", err)
	}
	if err := canary.Start(); err != nil { //nolint:contextcheck // Start begins a long-lived tick loop and takes no ctx by design — it outlives the assemble call that starts it
		return unwind(ctx, a, opts.Stderr, "canary", err)
	}
	a.closers = append(a.closers, closer{name: "canary", close: canary.Shutdown})

	// Step 14: runners — constructed here, started by serve.
	a.runners = []runner{
		{name: "ingest loop", run: loop.Run, stop: loop.Stop},
		{name: "scheduler worker", run: worker.Run, stop: worker.Stop},
		{name: "liveness heartbeat", run: liveness.Run, stop: liveness.Stop},
	}

	return a, nil
}
