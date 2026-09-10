package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/maratik123/lab-game/internal/config"
	"github.com/maratik123/lab-game/internal/repotest"
	"github.com/maratik123/lab-game/internal/testdb"
	"github.com/maratik123/lab-game/internal/tgtest"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Main(m))
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
	wantOrder := []string{"signal registration", "pool", "health listener", "liveness final write", "canary"}
	if len(a.closers) != len(wantOrder) {
		t.Fatalf("closers = %v, want names %v", closerNames(a), wantOrder)
	}
	for i, name := range wantOrder {
		if a.closers[i].name != name {
			t.Errorf("closers[%d].name = %q, want %q (full: %v)", i, a.closers[i].name, name, closerNames(a))
		}
	}
}

func closerNames(a *app) []string {
	names := make([]string, len(a.closers))
	for i, c := range a.closers {
		names[i] = c.name
	}
	return names
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
