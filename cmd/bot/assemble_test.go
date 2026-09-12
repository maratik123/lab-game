package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mymmrac/telego"

	"github.com/maratik123/lab-game/internal/config"
	"github.com/maratik123/lab-game/internal/ingest"
	"github.com/maratik123/lab-game/internal/leaktest"
	"github.com/maratik123/lab-game/internal/repotest"
	"github.com/maratik123/lab-game/internal/scheduler"
	"github.com/maratik123/lab-game/internal/store"
	"github.com/maratik123/lab-game/internal/testdb"
	"github.com/maratik123/lab-game/internal/tgtest"
)

func TestMain(m *testing.M) {
	os.Exit(leaktest.Main(m, testdb.Main))
}

// assembleTestEnv returns a fresh, fully valid environment for assemble:
// a fresh schema DSN, the fake Bot API's base URL, and a metrics address
// that binds an ephemeral loopback port.
func assembleTestEnv(t *testing.T) map[string]string {
	t.Helper()
	return map[string]string{
		"LAB_GAME_BOT_TOKEN":           tgtest.Token,
		"LAB_GAME_DSN":                 testdb.SchemaDSN(t),
		"LAB_GAME_BOT_API_BASE_URL":    tgtest.BaseURL,
		"LAB_GAME_ALLOWED_CHAT_IDS":    "-100123456789",
		"LAB_GAME_BALANCE_PATH":        repotest.RootPath(t, "config/balance.yaml"),
		"LAB_GAME_WORLD_PATH":          repotest.RootPath(t, "config/world"),
		"LAB_GAME_HEALTH_METRICS_ADDR": "127.0.0.1:0",
	}
}

func mapLookup(m map[string]string) config.Lookup {
	return func(key string) (string, bool) {
		v, ok := m[key]
		return v, ok
	}
}

// teardown walks a's closers from last to first, exactly as assemble's
// own unwind and drain's shutdown do, so a test never leaks a listener
// or a pool into the next.
func teardown(tb testing.TB, a *app) {
	tb.Helper()
	if a == nil {
		return
	}
	for i := len(a.closers) - 1; i >= 0; i-- {
		if err := a.closers[i].close(context.Background()); err != nil {
			tb.Errorf("teardown: closer %q: %v", a.closers[i].name, err)
		}
	}
}

func TestAssemble_HappyPath(t *testing.T) {
	t.Parallel()
	srv := tgtest.New(t, tgtest.Success(nil))

	var stderr bytes.Buffer
	a, err := assemble(context.Background(), assembleOptions{
		Lookup:     mapLookup(assembleTestEnv(t)),
		Stderr:     &stderr,
		Version:    "test-version",
		StartedAt:  time.Now(),
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("assemble: %v (stderr: %s)", err, stderr.String())
	}
	t.Cleanup(func() { teardown(t, a) })

	if err := a.readiness.Ready(context.Background()); err != nil {
		t.Errorf("readiness after assemble: %v, want nil", err)
	}
	if a.healthSrv.Addr() == "" {
		t.Error("healthSrv.Addr() empty, want a bound address")
	}
	if len(a.runners) != 3 {
		t.Errorf("len(runners) = %d, want 3", len(a.runners))
	}
	wantOrder := []string{"signal registration", "pool", "health listener", "liveness final write", "http client", "canary"}
	if len(a.closers) != len(wantOrder) {
		t.Fatalf("closers = %v, want names %v", closerNames(a), wantOrder)
	}
	for i, name := range wantOrder {
		if a.closers[i].name != name {
			t.Errorf("closers[%d].name = %q, want %q (full: %v)", i, a.closers[i].name, name, closerNames(a))
		}
	}

	// A message-delivering call to a chat outside the configured
	// allowlist is refused, whatever code path issues it — driven
	// through the assembled client itself, not a gate the test builds.
	const disallowedChatID = -999999999
	_, sendErr := a.client.API().SendMessage(context.Background(), &telego.SendMessageParams{
		ChatID: telego.ChatID{ID: disallowedChatID},
		Text:   "outside the allowlist",
	})
	if !errors.Is(sendErr, ingest.ErrChatRefused) {
		t.Errorf("SendMessage to a disallowed chat: err = %v, want ingest.ErrChatRefused in its chain", sendErr)
	}

	// The assembled loop's router carries no route, and the assembled
	// worker's registry no declaration — observed through what each
	// was constructed from, not by re-asserting what New was called
	// with.
	if kinds := a.router.Kinds(); len(kinds) != 0 {
		t.Errorf("router.Kinds() = %v, want none", kinds)
	}
	emptyRegistry, err := scheduler.NewRegistry()
	if err != nil {
		t.Fatalf("scheduler.NewRegistry() (reference empty registry): %v", err)
	}
	if !reflect.DeepEqual(a.taskRegistry, emptyRegistry) {
		t.Errorf("taskRegistry = %#v, want it deeply equal to an empty registry (no declarations)", a.taskRegistry)
	}
}

func closerNames(a *app) []string {
	names := make([]string, len(a.closers))
	for i, c := range a.closers {
		names[i] = c.name
	}
	return names
}

// TestAssemble_BuildsItsOwnHTTPClientWhenNoneSupplied proves that with
// no HTTPClient in assembleOptions, the process still holds a non-nil
// client of its own, and never the package-level http.DefaultClient —
// the shared global every other package in the binary could also
// mutate or close.
func TestAssemble_BuildsItsOwnHTTPClientWhenNoneSupplied(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	a, err := assemble(context.Background(), assembleOptions{
		Lookup:    mapLookup(assembleTestEnv(t)),
		Stderr:    &stderr,
		Version:   "test-version",
		StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("assemble: %v (stderr: %s)", err, stderr.String())
	}
	t.Cleanup(func() { teardown(t, a) })

	if a.httpClient == nil {
		t.Fatal("httpClient is nil, want a process client of its own")
	}
	if a.httpClient == http.DefaultClient {
		t.Error("httpClient == http.DefaultClient, want a client the process built and owns")
	}
}

// TestAssemble_UsesTheSuppliedHTTPClient proves that when a client is
// supplied, the process holds that exact one rather than building its
// own.
func TestAssemble_UsesTheSuppliedHTTPClient(t *testing.T) {
	t.Parallel()
	srv := tgtest.New(t, tgtest.Success(nil))
	supplied := srv.Client()

	var stderr bytes.Buffer
	a, err := assemble(context.Background(), assembleOptions{
		Lookup:     mapLookup(assembleTestEnv(t)),
		Stderr:     &stderr,
		Version:    "test-version",
		StartedAt:  time.Now(),
		HTTPClient: supplied,
	})
	if err != nil {
		t.Fatalf("assemble: %v (stderr: %s)", err, stderr.String())
	}
	t.Cleanup(func() { teardown(t, a) })

	if a.httpClient != supplied {
		t.Errorf("httpClient = %p, want the supplied client %p", a.httpClient, supplied)
	}
}

// TestAssemble_TransportCloneFailureReturnsStepErrorNotPanic drives the
// panic-surface risk row: when http.DefaultTransport is not the
// concrete *http.Transport this step's clone assumes, assemble must
// return its stepError naming the Telegram-client step rather than
// panicking on the failed type assertion.
func TestAssemble_TransportCloneFailureReturnsStepErrorNotPanic(t *testing.T) {
	prevDefaultTransport := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("unreachable: this transport is never invoked")
	})
	t.Cleanup(func() { http.DefaultTransport = prevDefaultTransport })

	var stderr bytes.Buffer
	a, err := assemble(context.Background(), assembleOptions{
		Lookup:    mapLookup(assembleTestEnv(t)),
		Stderr:    &stderr,
		Version:   "test-version",
		StartedAt: time.Now(),
	})
	t.Cleanup(func() { teardown(t, a) })

	if err == nil {
		t.Fatal("assemble: err = nil, want a stepError naming the telegram-client step")
	}
	var stepErr *stepError
	if !errors.As(err, &stepErr) {
		t.Fatalf("assemble: err = %v, want a *stepError", err)
	}
	if stepErr.step != "telegram client" {
		t.Errorf("stepError.step = %q, want %q", stepErr.step, "telegram client")
	}
}

// roundTripperFunc adapts a function to http.RoundTripper, for a
// fixture http.DefaultTransport that is deliberately not *http.Transport.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestAssemble_MissingConfiguration(t *testing.T) {
	t.Parallel()
	env := assembleTestEnv(t)
	delete(env, "LAB_GAME_BOT_TOKEN")

	var stderr bytes.Buffer
	a, err := assemble(context.Background(), assembleOptions{
		Lookup: mapLookup(env),
		Stderr: &stderr,
	})
	if err == nil {
		teardown(t, a)
		t.Fatal("assemble: expected an error")
	}
	var se *stepError
	if !errors.As(err, &se) {
		t.Fatalf("assemble error = %v, want a *stepError", err)
	}
	if se.step != "configuration" {
		t.Errorf("step = %q, want %q", se.step, "configuration")
	}
	if !strings.Contains(err.Error(), "LAB_GAME_BOT_TOKEN") {
		t.Errorf("error = %q, want it to name LAB_GAME_BOT_TOKEN", err.Error())
	}
}

func TestAssemble_UnreachableDatabase(t *testing.T) {
	t.Parallel()
	env := assembleTestEnv(t)
	env["LAB_GAME_DSN"] = "postgres://user:pass@127.0.0.1:1/nosuchdb"

	var stderr bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a, err := assemble(ctx, assembleOptions{
		Lookup: mapLookup(env),
		Stderr: &stderr,
	})
	if err == nil {
		teardown(t, a)
		t.Fatal("assemble: expected an error")
	}
	var se *stepError
	if !errors.As(err, &se) {
		t.Fatalf("assemble error = %v, want a *stepError", err)
	}
	if se.step != "database" {
		t.Errorf("step = %q, want %q", se.step, "database")
	}
}

func TestAssemble_AutoApplyDisabledWithPendingMigration(t *testing.T) {
	t.Parallel()
	env := assembleTestEnv(t)
	env["LAB_GAME_PROCESS_MIGRATE_ON_START"] = "false"

	var stderr bytes.Buffer
	a, err := assemble(context.Background(), assembleOptions{
		Lookup: mapLookup(env),
		Stderr: &stderr,
	})
	if err == nil {
		teardown(t, a)
		t.Fatal("assemble: expected an error")
	}
	var se *stepError
	if !errors.As(err, &se) {
		t.Fatalf("assemble error = %v, want a *stepError", err)
	}
	if se.step != "migrations" {
		t.Errorf("step = %q, want %q", se.step, "migrations")
	}
	if !strings.Contains(err.Error(), "LAB_GAME_PROCESS_MIGRATE_ON_START") {
		t.Errorf("error = %q, want it to name the disabling key", err.Error())
	}
}

// TestAssemble_AutoApplyDisabledWithNoPendingMigrationSucceeds asserts
// the case distinct from the one above: auto-apply disabled AND no
// migration pending starts the process normally. The schema is
// pre-migrated directly, before assemble itself ever runs — exactly the
// state an operator who applies migrations out of band leaves behind —
// with LAB_GAME_PROCESS_MIGRATE_ON_START=false.
func TestAssemble_AutoApplyDisabledWithNoPendingMigrationSucceeds(t *testing.T) {
	t.Parallel()
	dsn := testdb.SchemaDSN(t)

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse DSN: %v", err)
	}
	pool, err := store.NewPool(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	if err := store.Migrate(context.Background(), pool, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("pre-migrate: %v", err)
	}
	pool.Close()

	srv := tgtest.New(t, tgtest.Success(nil))
	env := assembleTestEnv(t)
	env["LAB_GAME_DSN"] = dsn
	env["LAB_GAME_PROCESS_MIGRATE_ON_START"] = "false"

	var stderr bytes.Buffer
	a, err := assemble(context.Background(), assembleOptions{
		Lookup:     mapLookup(env),
		Stderr:     &stderr,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("assemble: %v (stderr: %s)", err, stderr.String())
	}
	t.Cleanup(func() { teardown(t, a) })

	if err := a.readiness.Ready(context.Background()); err != nil {
		t.Errorf("readiness after assemble: %v, want nil", err)
	}
}

func TestAssemble_TelegramClientStepFailsOnPlaceholderToken(t *testing.T) {
	t.Parallel()
	env := assembleTestEnv(t)
	env["LAB_GAME_BOT_TOKEN"] = "changeme"

	var stderr bytes.Buffer
	a, err := assemble(context.Background(), assembleOptions{
		Lookup: mapLookup(env),
		Stderr: &stderr,
	})
	if err == nil {
		teardown(t, a)
		t.Fatal("assemble: expected an error")
	}
	var se *stepError
	if !errors.As(err, &se) {
		t.Fatalf("assemble error = %v, want a *stepError", err)
	}
	if se.step != "telegram client" {
		t.Errorf("step = %q, want %q (full: %v)", se.step, "telegram client", err)
	}
}

// TestAssemble_UnwindReleasesTheHealthListenerOnLateFailure asserts the
// real clause a late start-up failure must satisfy: a failure at a step
// after the health listener has bound leaves the listener's port free
// again, not merely a *stepError naming the step. The address is
// reserved and released first so it can be passed to assemble as a
// fixed (non-ephemeral) health metrics address — an ephemeral ":0"
// address never exposes the bound port back to the caller on a failed
// assemble, since a is nil.
func TestAssemble_UnwindReleasesTheHealthListenerOnLateFailure(t *testing.T) {
	t.Parallel()
	var lc net.ListenConfig
	l, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve a port: %v", err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatalf("release the reserved port: %v", err)
	}

	env := assembleTestEnv(t)
	env["LAB_GAME_HEALTH_METRICS_ADDR"] = addr
	// A placeholder token fails at the telegram client step — Step 11,
	// well after the health listener (Step 7) has already bound addr.
	env["LAB_GAME_BOT_TOKEN"] = "changeme"

	var stderr bytes.Buffer
	a, err := assemble(context.Background(), assembleOptions{
		Lookup: mapLookup(env),
		Stderr: &stderr,
	})
	if err == nil {
		teardown(t, a)
		t.Fatal("assemble: expected an error")
	}
	var se *stepError
	if !errors.As(err, &se) {
		t.Fatalf("assemble error = %v, want a *stepError", err)
	}
	if se.step != "telegram client" {
		t.Fatalf("step = %q, want %q (full: %v) — this test needs the failure to land after the health listener step", se.step, "telegram client", err)
	}

	l2, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		t.Fatalf("port %s is still bound after unwind: %v — the health listener was not released", addr, err)
	}
	_ = l2.Close()
}

func TestAssemble_CanaryStepFailsOnMalformedCloudToken(t *testing.T) {
	t.Parallel()
	srv := tgtest.New(t, tgtest.Success(nil))
	env := assembleTestEnv(t)
	// A non-empty value that telego's own token format check rejects —
	// the leg builder builds the cloud leg exactly when this key is
	// non-empty, so the canary step is the one that fails here, after
	// every earlier step (including the telegram client step, which
	// validates only the own-instance token) has already succeeded.
	env["LAB_GAME_HEALTH_CANARY_CLOUD_TOKEN"] = "not-a-valid-token"

	var stderr bytes.Buffer
	a, err := assemble(context.Background(), assembleOptions{
		Lookup:     mapLookup(env),
		Stderr:     &stderr,
		HTTPClient: srv.Client(),
	})
	if err == nil {
		teardown(t, a)
		t.Fatal("assemble: expected an error")
	}
	var se *stepError
	if !errors.As(err, &se) {
		t.Fatalf("assemble error = %v, want a *stepError", err)
	}
	if se.step != "canary" {
		t.Errorf("step = %q, want %q (full: %v)", se.step, "canary", err)
	}
	if !errors.Is(err, se.cause) {
		t.Errorf("errors.Is(err, se.cause) = false, want true — Unwrap should surface the underlying cause")
	}
}

func TestStepError_Error(t *testing.T) {
	t.Parallel()
	cause := errors.New("boom")
	se := &stepError{step: "database", cause: cause}

	if got, want := se.Error(), "database: boom"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if !errors.Is(se, cause) {
		t.Error("errors.Is(se, cause) = false, want true")
	}
}
