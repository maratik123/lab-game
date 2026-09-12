// Package tgtest provides a shared, in-process fake Bot API server for
// this module's Telegram client tests, an ordering/rate-limit/429 test
// suite, and an eval harness — the threshold at which this project's own
// rules require a shared package rather than copy-paste in each consumer.
//
// The server answers over net.Pipe through a real *http.Server and a real
// *http.Transport, wired by a DialContext hook: there is no listener and
// no socket. That is what makes the whole test suite runnable inside a
// testing/synctest bubble — a pipe connection is durably
// blockable inside a bubble, a real socket is not — and it makes "no
// network egress" structural: BaseURL is under the reserved ".invalid" TLD
// (RFC 2606), so any dial this package's DialContext does not
// intercept can only fail.
//
// tgtest imports neither this module's Telegram client package nor
// telego — net, net/http and encoding/json are enough — so it carries no
// cycle and, imported only from test files, never links into the bot
// command (mirroring the same consequence for this module's database
// test helper package).
package tgtest

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

// BaseURL is the fake server's base URL production code is pointed at in
// tests, taken from the same configuration field production uses
// for the self-hosted instance and for api.telegram.org.
const BaseURL = "http://bot-api.invalid"

// Token is a syntactically valid fake bot token — a digit run, a colon,
// then 35 word/hyphen characters, matching telego's own tokenRegexp
// exactly — so telego.NewBot accepts it. The repeated "FAKE-" body makes
// it unmistakably not a credential.
const Token = "1:FAKE-FAKE-FAKE-FAKE-FAKE-FAKE-FAKE-" //nolint:gosec // G101: syntactically valid but fake bot token literal, used only by tests against this in-process fake server — not a credential.

// Handler answers one HTTP request against the fake server. It is an
// ordinary http.HandlerFunc-shaped value; the Success, TooManyRequests,
// ServerError and Delayed helpers below construct the Bot API response
// shapes this package's consumers need.
type Handler func(w http.ResponseWriter, r *http.Request)

// Server is an in-process fake Bot API server. The zero value
// is not usable; construct one with New.
type Server struct {
	tb testing.TB

	mu       sync.Mutex
	handler  Handler
	failDial bool

	listener *pipeListener
	httpSrv  *http.Server
}

// New starts a fake server that answers every request with handler until
// SetHandler replaces it. Every request's context derives from a base
// context of New's own — not the test's Context(), which is already
// cancelled by the time any cleanup runs — cancelled by tb.Cleanup before
// the server itself closes, so the fake server keeps answering normally
// while cleanups registered after New run, and a Delayed handler still
// waiting when the test ends is released there instead of outliving it,
// no matter which clock is active (the real one or a testing/synctest
// bubble's virtual one) and no matter whether the request's body was
// read.
func New(tb testing.TB, handler Handler) *Server {
	tb.Helper()
	baseCtx, cancelBase := context.WithCancel(context.Background()) //nolint:forbidigo // this is New's own base context: the tb.Cleanup below cancels it and closes the server and listener
	s := &Server{
		tb:       tb,
		handler:  handler,
		listener: newPipeListener(),
	}
	s.httpSrv = &http.Server{
		Handler:           http.HandlerFunc(s.serveHTTP),
		ReadHeaderTimeout: 0, // unbounded — tests control timing explicitly via context deadlines.
		BaseContext:       func(net.Listener) context.Context { return baseCtx },
	}
	go func() {
		// http.ErrServerClosed is the expected outcome of tb.Cleanup's
		// Close below; anything else is a genuine failure.
		if err := s.httpSrv.Serve(s.listener); err != nil && err != http.ErrServerClosed {
			tb.Errorf("tgtest: Serve: %v", err)
		}
	}()
	tb.Cleanup(func() {
		cancelBase()
		_ = s.httpSrv.Close()
		_ = s.listener.Close()
	})
	return s
}

// serveHTTP dispatches to the currently installed handler, under the
// server's own lock so SetHandler may be called concurrently with
// in-flight requests without a data race (go test -race is a required
// gate for this change).
func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	h := s.handler
	s.mu.Unlock()
	h(w, r)
}

// SetHandler replaces the handler that answers every subsequent request.
func (s *Server) SetHandler(h Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handler = h
}

// FailNextDial makes exactly the next DialContext call return an error
// instead of connecting, reproducing "a transport-level failure with no
// request written" — httptrace's
// WroteRequest hook never fires because no connection was ever obtained.
func (s *Server) FailNextDial() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failDial = true
}

// DialContext is the *http.Transport DialContext hook that reaches this
// server with no real network: wire it as
// &http.Transport{DialContext: srv.DialContext}. It consumes one pending
// FailNextDial call if present, otherwise hands the server a net.Pipe
// connection and returns the client side.
func (s *Server) DialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	s.mu.Lock()
	fail := s.failDial
	s.failDial = false
	s.mu.Unlock()
	if fail {
		return nil, fmt.Errorf("tgtest: dial refused (FailNextDial)")
	}

	client, srv := net.Pipe()
	select {
	case s.listener.conns <- srv:
		return client, nil
	case <-s.listener.closed:
		_ = client.Close()
		_ = srv.Close()
		return nil, net.ErrClosed
	case <-ctx.Done():
		_ = client.Close()
		_ = srv.Close()
		return nil, ctx.Err()
	}
}

// Client returns an *http.Client whose Transport reaches this server
// exclusively through DialContext — no other host is reachable through it
// — with keep-alives disabled, which is required for a leaked-connection
// goroutine never to survive past a synctest bubble's root returning.
func (s *Server) Client() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext:       s.DialContext,
			DisableKeepAlives: true,
		},
	}
}

// CloseWithoutResponse hijacks the connection and closes it with no bytes
// written back, reproducing "a transport-level failure after the request
// was written" — the ambiguous case the classifier table and its
// callers need. It reports (via s.tb) rather than panicking if the connection does
// not support hijacking, which does not happen over this package's own
// net.Pipe-backed listener.
func (s *Server) CloseWithoutResponse() Handler {
	return func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			s.tb.Errorf("tgtest: ResponseWriter does not support Hijack")
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			s.tb.Errorf("tgtest: hijack: %v", err)
			return
		}
		_ = conn.Close()
	}
}

// envelope is the Bot API's response shape, matching
// telegoapi.Response/telegoapi.Error/telegoapi.ResponseParameters closely
// enough for this package's tests, without importing telego (this package
// must not — see the package comment).
type envelope struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result,omitempty"`
	ErrorCode   int             `json:"error_code,omitempty"`
	Description string          `json:"description,omitempty"`
	Parameters  *parameters     `json:"parameters,omitempty"`
}

type parameters struct {
	RetryAfter int `json:"retry_after,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Success answers with HTTP 200 and {"ok":true,"result":result} — result
// must already be valid JSON (e.g. json.RawMessage or a marshalled
// struct); a nil result answers with no "result" field.
func Success(result json.RawMessage) Handler {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, envelope{OK: true, Result: result})
	}
}

// TooManyRequests answers with HTTP 429. When retryAfter is positive, the
// body carries parameters.retry_after; when it is zero or negative, the
// field is omitted entirely — a 429 need not carry
// parameters.retry_after.
func TooManyRequests(retryAfter int) Handler {
	return func(w http.ResponseWriter, _ *http.Request) {
		env := envelope{OK: false, ErrorCode: http.StatusTooManyRequests, Description: "Too Many Requests"}
		if retryAfter > 0 {
			env.Parameters = &parameters{RetryAfter: retryAfter}
		}
		writeJSON(w, http.StatusTooManyRequests, env)
	}
}

// ServerError answers with the given 5xx status and a minimal error body.
func ServerError(status int) Handler {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, status, envelope{OK: false, ErrorCode: status, Description: "Internal Server Error"})
	}
}

// Delayed waits for the given duration or for the request's context to
// end, whichever comes first — through whichever clock is active in the
// caller's goroutine tree, the real one or a testing/synctest bubble's
// virtual one — before invoking next. It returns without calling next
// when the request's context ends first, so a handler still waiting when
// its server's cleanup cancels the server's own base context does not
// outlive the test.
func Delayed(after time.Duration, next Handler) Handler {
	return func(w http.ResponseWriter, r *http.Request) {
		timer := time.NewTimer(after)
		defer timer.Stop()
		select {
		case <-timer.C:
			next(w, r)
		case <-r.Context().Done():
		}
	}
}

// pipeListener is a net.Listener with no real socket: Accept hands out
// net.Pipe server-side connections fed by DialContext. It exists solely so
// Server can drive a real *http.Server with no OS-level listener.
type pipeListener struct {
	conns  chan net.Conn
	closed chan struct{}
	once   sync.Once
}

func newPipeListener() *pipeListener {
	return &pipeListener{
		conns:  make(chan net.Conn),
		closed: make(chan struct{}),
	}
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *pipeListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (l *pipeListener) Addr() net.Addr { return pipeAddr{} }

// pipeAddr is the net.Addr pipeListener reports — there is no real address
// behind it.
type pipeAddr struct{}

func (pipeAddr) Network() string { return "pipe" }
func (pipeAddr) String() string  { return "tgtest-pipe" }
