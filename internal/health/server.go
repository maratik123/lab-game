package health

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// metricsPath is the health-metrics endpoint's path — a contract with
// the scraper, not a configuration key. Every other path answers the
// mux's own 404.
const metricsPath = "/metrics"

// readyzPath is the readiness endpoint's path, beside metricsPath on the
// same listener — a contract with the probe, not a configuration key.
// This module gains no second listen address and no second
// configuration key for it.
const readyzPath = "/readyz"

// readHeaderTimeout bounds how long the server waits to read a request's
// headers, closing the slow-request attack surface an unbounded server
// leaves open.
const readHeaderTimeout = 5 * time.Second

// readyzTimeout bounds how long the /readyz handler waits on Ready, so a
// probe that blocks server-side never hangs a request — a named
// constant, not a configuration key.
const readyzTimeout = 5 * time.Second

// readyzReadyBody and readyzNotReadyBody are the /readyz response
// bodies. Fixed text, never the underlying error: a secret or any other
// diagnostic detail must never reach a probe response.
const (
	readyzReadyBody    = "ready\n"
	readyzNotReadyBody = "not ready\n"
)

// reasonMustNotBeNil is the rejection reason for every required nil
// ServerOptions field.
const reasonMustNotBeNil = "must not be nil"

// OptionError is returned by NewServer when an Options field is
// invalid.
type OptionError struct {
	// Field names the invalid ServerOptions field.
	Field string
	// Reason describes why the value was rejected.
	Reason string
}

// Error renders "health: <field>: <reason>".
func (e *OptionError) Error() string {
	return fmt.Sprintf("health: %s: %s", e.Field, e.Reason)
}

// ReadyFunc reports whether the process is ready to serve: nil means
// ready, and a non-nil error names why not (never rendered to a probe
// response — see readyzNotReadyBody). The /readyz handler and the
// labgame_ready gauge share this one definition, each calling it with
// an internally-bounded context so neither a scrape nor a probe can
// hang on it.
type ReadyFunc func(ctx context.Context) error

// Server serves the metrics endpoint over one *http.Server, with an
// explicit Start/Shutdown pair the caller owns.
type Server struct {
	httpSrv *http.Server

	mu           sync.Mutex
	started      bool
	listener     net.Listener
	serveErr     chan error
	shutdownOnce sync.Once
	shutdownDone chan struct{}
	shutdownErr  error
}

// ServerOptions configures NewServer.
type ServerOptions struct {
	// Addr is the listen address Start binds — "host:port", or
	// "host:0" to let the kernel choose a port (read back afterwards
	// through Addr()).
	Addr string
	// Gatherer is scraped at the metrics path. Must not be nil.
	Gatherer prometheus.Gatherer
	// Ready answers both /readyz and the labgame_ready gauge. Must not
	// be nil.
	Ready ReadyFunc
}

// NewServer builds a Server that gathers from opts.Gatherer at the
// fixed metrics path and answers opts.Ready at the fixed readiness
// path, refusing a nil Gatherer or Ready with an *OptionError naming
// the field.
func NewServer(opts ServerOptions) (*Server, error) {
	if opts.Gatherer == nil {
		return nil, &OptionError{Field: "Gatherer", Reason: reasonMustNotBeNil}
	}
	if opts.Ready == nil {
		return nil, &OptionError{Field: "Ready", Reason: reasonMustNotBeNil}
	}

	mux := http.NewServeMux()
	mux.Handle(metricsPath, promhttp.HandlerFor(opts.Gatherer, promhttp.HandlerOpts{}))
	mux.HandleFunc(readyzPath, readyzHandler(opts.Ready))
	return &Server{
		httpSrv: &http.Server{
			Addr:              opts.Addr,
			Handler:           mux,
			ReadHeaderTimeout: readHeaderTimeout,
		},
	}, nil
}

// readyzHandler answers 200 with readyzReadyBody when ready returns nil,
// and 503 with readyzNotReadyBody otherwise — never the underlying
// error, so a probe response can never carry a secret or any other
// diagnostic detail. ready is called with a context bounded by
// readyzTimeout, derived from the request's own context, so a blocking
// Ready can never hang the handler past that bound.
func readyzHandler(ready ReadyFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), readyzTimeout)
		defer cancel()

		if err := ready(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(readyzNotReadyBody))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(readyzReadyBody))
	}
}

// Start binds the listener synchronously, returning a bind error before
// spawning the serve goroutine — so a bind failure is always reported
// through a returned error, never discovered later on a background
// goroutine. Returns an error rather than panicking on a second call.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return errors.New("health: server already started")
	}

	ln, err := new(net.ListenConfig).Listen(context.Background(), "tcp", s.httpSrv.Addr) //nolint:forbidigo // this is the listener bind: owned by Start itself, complete before Start returns
	if err != nil {
		return fmt.Errorf("health: listen: %w", err)
	}
	s.listener = ln
	s.started = true
	s.serveErr = make(chan error, 1)
	s.shutdownDone = make(chan struct{})
	go func() {
		s.serveErr <- s.httpSrv.Serve(ln)
	}()
	return nil
}

// Addr reports the bound listen address — the operator-visible way to
// discover the actual port when Start was called with a ":0" address.
// Empty before Start succeeds.
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// Shutdown gracefully stops the server, joining the shutdown error with
// the serve goroutine's terminal error so neither is dropped;
// http.ErrServerClosed is the expected terminal value of a graceful stop
// and is not itself reported as a failure. Returns an error rather than
// panicking when called on a server never started. Idempotent: a second
// (or later) call never repeats the graceful stop. sync.Once only ever
// starts the graceful stop on a goroutine — it never waits inside
// Do — so every caller, the one that starts the stop included, waits on
// the shared completion channel racing its own ctx, and every caller's
// wait is bounded by the ctx it passed, whether or not the graceful stop
// has finished yet. All callers see the same latched result once ready.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	started := s.started
	serveErr := s.serveErr
	done := s.shutdownDone
	s.mu.Unlock()
	if !started {
		return errors.New("health: server not started")
	}

	s.shutdownOnce.Do(func() {
		go func() {
			shutdownErr := s.httpSrv.Shutdown(ctx)
			err := <-serveErr
			if errors.Is(err, http.ErrServerClosed) {
				err = nil
			}
			s.shutdownErr = errors.Join(shutdownErr, err)
			close(done)
		}()
	})

	select {
	case <-done:
		return s.shutdownErr
	case <-ctx.Done():
		return ctx.Err()
	}
}
