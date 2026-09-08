package tg

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/maratik123/lab-game/internal/config"
	"github.com/maratik123/lab-game/internal/tgtest"
)

// repoRootPath resolves rel against the repository root, regardless of the
// test binary's working directory — this package is exactly two
// directories below the root, same depth as this module's configuration
// package's own copy of this helper.
func repoRootPath(t *testing.T, rel string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("repoRootPath: runtime.Caller failed")
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
	return filepath.Join(root, filepath.FromSlash(rel))
}

// walkGoFiles calls fn for every non-test Go source file under root's
// command and package directories.
func walkGoFiles(t *testing.T, root string, fn func(path string, content []byte)) {
	t.Helper()
	for _, dir := range []string{"cmd", "internal"} {
		start := filepath.Join(root, dir)
		err := filepath.WalkDir(start, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			fn(path, content)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", start, err)
		}
	}
}

// TestGuard_NoFastHTTPOrGoJSONImport asserts that no non-test Go file
// under this module's command or package directories imports a fasthttp
// or go-json package — the client's
// transport is net/http and its codec is encoding/json throughout this
// project's own code (telego's own internal
// decode of its generated types is the one residue, which this test does
// not and cannot reach — it scans this project's imports, not the module
// graph).
func TestGuard_NoFastHTTPOrGoJSONImport(t *testing.T) {
	t.Parallel()
	forbidden := []string{`"github.com/valyala/fasthttp`, `"github.com/grbit/go-json`, `"github.com/valyala/fastjson`}
	root := repoRootPath(t, ".")
	walkGoFiles(t, root, func(path string, content []byte) {
		for _, f := range forbidden {
			if strings.Contains(string(content), f) {
				t.Errorf("%s imports %s — the net/http + encoding/json swap must be total in this project's own code", path, f)
			}
		}
	})
}

// TestGuard_NoMetricsRegistryImportInTG asserts that no non-test Go file
// in this package imports a metrics-registry package — a future
// integration owns registering this package's Observations against one.
func TestGuard_NoMetricsRegistryImportInTG(t *testing.T) {
	t.Parallel()
	root := repoRootPath(t, ".")
	dir := filepath.Join(root, "internal", "tg")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", e.Name(), err)
		}
		if strings.Contains(string(content), "prometheus/client_golang") {
			t.Errorf("%s imports a metrics registry — this package must expose only the Observer interface", e.Name())
		}
	}
}

// TestGuard_NoRetryOrRateLimitLiteralAtCallSite asserts that no literal
// retry or rate-limit numeric-duration value appears at a call site in
// this package's own production files — every such value must arrive
// from the Transport configuration. The pattern flags a numeric literal
// multiplied directly by a time unit (e.g. "500 * time.Millisecond");
// the retry_after-seconds-to-Duration conversion elsewhere in this
// package multiplies a runtime value (resp.Parameters.RetryAfter), not a
// literal, so it does not match.
func TestGuard_NoRetryOrRateLimitLiteralAtCallSite(t *testing.T) {
	t.Parallel()
	pattern := regexp.MustCompile(`[0-9]+\s*\*\s*time\.(Nanosecond|Microsecond|Millisecond|Second|Minute|Hour)`)
	root := repoRootPath(t, ".")
	dir := filepath.Join(root, "internal", "tg")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", e.Name(), err)
		}
		for _, m := range pattern.FindAllString(string(content), -1) {
			t.Errorf("%s contains a literal duration %q — retry/rate-limit values must come from the Transport configuration", e.Name(), m)
		}
	}
}

// TestGuard_NoTelegoBotConstructionOutsideTG asserts that no non-test file
// outside this package names telego.NewBot or any
// telego.With* option — the only reachable lever (bot.api is unexported)
// that could apply an option to a bot this package did not build with its
// own caller wired in. This module has no downstream importers, so the
// scan over its own tree is a complete cover.
func TestGuard_NoTelegoBotConstructionOutsideTG(t *testing.T) {
	t.Parallel()
	pattern := regexp.MustCompile(`telego\.(NewBot|With[A-Za-z]+)\(`)
	root := repoRootPath(t, ".")
	tgDir := filepath.Join(root, "internal", "tg") + string(filepath.Separator)
	walkGoFiles(t, root, func(path string, content []byte) {
		if strings.HasPrefix(path, tgDir) {
			return
		}
		if m := pattern.FindString(string(content)); m != "" {
			t.Errorf("%s references %s outside this package — only this package may construct or reconfigure a *telego.Bot", path, m)
		}
	})
}

// TestGuard_BaseURLOnlyInConstructor asserts the source-level half of a
// wider property: BaseURL appears in this package's own production files
// only in the client constructor, as a value handed to
// telego.WithAPIServer — never branched on. (The behavioural half —
// identical limiter behaviour under two different base URLs — is
// TestLimiter_IdenticalBehaviourAcrossBaseURLs elsewhere in this suite,
// since the Limiter never takes a URL parameter at all.)
func TestGuard_BaseURLOnlyInConstructor(t *testing.T) {
	t.Parallel()
	root := repoRootPath(t, ".")
	dir := filepath.Join(root, "internal", "tg")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		if e.Name() == "client.go" {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", e.Name(), err)
		}
		if strings.Contains(string(content), "BaseURL") {
			t.Errorf("%s references BaseURL — only the client constructor should", e.Name())
		}
	}
}

// TestGuard_TokenExposureSitesAreTheAcceptedOnes discharges the
// "accepted in-module exposure" note: telego.Bot.Token() and
// FileDownloadURL are the only two telego methods that expose the token
// by design, and this project's own use of them (if any) is confined to
// this test file's own knowledge — the sweep exists so a future reader
// does not mistake a legitimate hit on one of these two methods for a
// leak (the token-leak obligation is about error messages, observations
// and fixtures, not about telego's own exported accessors).
func TestGuard_TokenExposureSitesAreTheAcceptedOnes(t *testing.T) {
	t.Parallel()
	root := repoRootPath(t, ".")
	pattern := regexp.MustCompile(`\.Token\(\)|\.FileDownloadURL\(`)
	walkGoFiles(t, root, func(path string, content []byte) {
		for _, m := range pattern.FindAllString(string(content), -1) {
			t.Logf("%s: accepted token-exposure site %s", path, m)
		}
	})
	// No assertion beyond "this compiles and runs" — the test's value is
	// the log line a future reader can grep for; a real leak is caught by
	// TestRetry_TokenAbsentFromRenderedError elsewhere in this suite and by
	// the fixture/error-rendering assertions throughout this package's own
	// tests, none of which render the token.
}

// TestGuard_RefusingGateBlocksTheAccessor asserts a refusal's effect
// end-to-end through Client.API() itself — the only accessor this package
// exposes.
func TestGuard_RefusingGateBlocksTheAccessor(t *testing.T) {
	t.Parallel()
	srv := tgtest.New(t, tgtest.Success(json.RawMessage(`{"id":1}`)))
	refusing := gateFunc(func(context.Context, Call) error { return errors.New("refused") })
	obs := &recordingObserver{}
	c := newTestClient(t, srv, func(o *Options) { o.Gate = refusing; o.Observer = obs })

	if _, err := c.API().GetMe(context.Background()); err == nil {
		t.Fatal("GetMe: expected the gate to refuse the call")
	}

	// The observation point fires exactly once per outbound
	// call, including a gate refusal — a future health dashboard must see
	// refusals too.
	all := obs.all()
	if len(all) != 1 {
		t.Fatalf("Observer received %d observations, want exactly 1", len(all))
	}
	if all[0].Method != "getMe" {
		t.Errorf("Observation.Method = %q, want %q", all[0].Method, "getMe")
	}
	if all[0].StatusCode != 0 {
		t.Errorf("Observation.StatusCode = %d, want 0 (no attempt was ever made)", all[0].StatusCode)
	}
	if all[0].Retries != 0 {
		t.Errorf("Observation.Retries = %d, want 0", all[0].Retries)
	}
}

// TestGuard_EndToEndViaConfigLoadProducedBaseURL asserts a call through
// Client.API() built from a Load-produced Config.BotAPIBaseURL — proving
// the whole chain from the configuration layer through the transport to
// a fake server.
func TestGuard_EndToEndViaConfigLoadProducedBaseURL(t *testing.T) {
	t.Parallel()
	srv := tgtest.New(t, tgtest.Success(json.RawMessage(`{"id":1}`)))

	env := map[string]string{
		"LAB_GAME_BOT_TOKEN":        "test-token",
		"LAB_GAME_DSN":              "postgres://user:pass@localhost/db",
		"LAB_GAME_BOT_API_BASE_URL": tgtest.BaseURL,
		"LAB_GAME_ALLOWED_CHAT_IDS": "-100123456789",
		"LAB_GAME_BALANCE_PATH":     repoRootPath(t, "config/balance.yaml"),
		"LAB_GAME_WORLD_PATH":       repoRootPath(t, "config/world"),
	}
	lookup := func(key string) (string, bool) { v, ok := env[key]; return v, ok }

	cfg, err := config.Load(lookup)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	c, err := New(Options{
		BaseURL:    cfg.BotAPIBaseURL.String(),
		Token:      tgtest.Token,
		Transport:  cfg.Transport,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	me, err := c.API().GetMe(context.Background())
	if err != nil {
		t.Fatalf("GetMe: %v", err)
	}
	if me.ID != 1 {
		t.Errorf("me.ID = %d, want 1", me.ID)
	}
}
