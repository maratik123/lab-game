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

// readHeaderTimeout bounds how long the server waits to read a request's
// headers, closing the slow-request attack surface an unbounded server
// leaves open.
const readHeaderTimeout = 5 * time.Second

// Server serves the metrics endpoint over one *http.Server, with an
// explicit Start/Shutdown pair the caller owns.
type Server struct {
	httpSrv *http.Server

	mu       sync.Mutex
	started  bool
	listener net.Listener
	serveErr chan error
}

// NewServer builds a Server that gathers from gatherer at the fixed
// metrics path.
func NewServer(addr string, gatherer prometheus.Gatherer) *Server {
	mux := http.NewServeMux()
	mux.Handle(metricsPath, promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{}))
	return &Server{
		httpSrv: &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadHeaderTimeout: readHeaderTimeout,
		},
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

	ln, err := new(net.ListenConfig).Listen(context.Background(), "tcp", s.httpSrv.Addr)
	if err != nil {
		return fmt.Errorf("health: listen: %w", err)
	}
	s.listener = ln
	s.started = true
	s.serveErr = make(chan error, 1)
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
// panicking when called on a server never started.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	started := s.started
	serveErr := s.serveErr
	s.mu.Unlock()
	if !started {
		return errors.New("health: server not started")
	}

	shutdownErr := s.httpSrv.Shutdown(ctx)
	err := <-serveErr
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	return errors.Join(shutdownErr, err)
}
