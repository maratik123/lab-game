package tgtest

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func doGet(t *testing.T, ctx context.Context, client *http.Client) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, BaseURL+"/bot"+Token+"/getMe", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	return client.Do(req)
}

func TestServer_Success(t *testing.T) {
	t.Parallel()
	srv := New(t, Success(json.RawMessage(`{"id":1}`)))
	resp, err := doGet(t, context.Background(), srv.Client())
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !env.OK {
		t.Errorf("OK = false, want true")
	}
	if string(env.Result) != `{"id":1}` {
		t.Errorf("Result = %s", env.Result)
	}
}

func TestServer_TooManyRequestsWithRetryAfter(t *testing.T) {
	t.Parallel()
	srv := New(t, TooManyRequests(7))
	resp, err := doGet(t, context.Background(), srv.Client())
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", resp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if env.Parameters == nil || env.Parameters.RetryAfter != 7 {
		t.Errorf("Parameters = %+v, want RetryAfter 7", env.Parameters)
	}
}

func TestServer_TooManyRequestsWithoutRetryAfter(t *testing.T) {
	t.Parallel()
	srv := New(t, TooManyRequests(0))
	resp, err := doGet(t, context.Background(), srv.Client())
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", resp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if env.Parameters != nil {
		t.Errorf("Parameters = %+v, want nil (field omitted)", env.Parameters)
	}
}

func TestServer_ServerError(t *testing.T) {
	t.Parallel()
	srv := New(t, ServerError(http.StatusBadGateway))
	resp, err := doGet(t, context.Background(), srv.Client())
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}
}

func TestServer_FailNextDial(t *testing.T) {
	t.Parallel()
	srv := New(t, Success(nil))
	srv.FailNextDial()
	failResp, err := doGet(t, context.Background(), srv.Client())
	if failResp != nil {
		_ = failResp.Body.Close()
	}
	if err == nil {
		t.Fatal("Do: expected an error from the failed dial, got nil")
	}
	// The failure is consumed: the next call reaches the real handler.
	resp, err := doGet(t, context.Background(), srv.Client())
	if err != nil {
		t.Fatalf("Do (second call): %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 (dial failure must not persist)", resp.StatusCode)
	}
}

func TestServer_CloseWithoutResponse(t *testing.T) {
	t.Parallel()
	srv := New(t, nil)
	srv.SetHandler(srv.CloseWithoutResponse())
	resp, err := doGet(t, context.Background(), srv.Client())
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err == nil {
		t.Fatal("Do: expected an error from the closed connection, got nil")
	}
}

func TestServer_Delayed(t *testing.T) {
	t.Parallel()
	srv := New(t, Delayed(20*time.Millisecond, Success(nil)))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	resp, err := doGet(t, ctx, srv.Client())
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err == nil {
		t.Fatal("Do: expected a context-deadline error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want context.DeadlineExceeded in the chain", err)
	}
}

func TestServer_SetHandlerAppliesToSubsequentRequests(t *testing.T) {
	t.Parallel()
	srv := New(t, Success(json.RawMessage(`1`)))
	srv.SetHandler(Success(json.RawMessage(`2`)))
	resp, err := doGet(t, context.Background(), srv.Client())
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if string(env.Result) != "2" {
		t.Errorf("Result = %s, want 2 (SetHandler must apply)", env.Result)
	}
}

// TestServer_DelayedHandlerReleasedByCleanup covers a Delayed handler
// still waiting when its server's cleanup runs: it returns without waiting
// out its delay, and it is the cleanup — not the client giving up — that
// releases it. The request carries a JSON body (the shape every real
// consumer sends) that the handler never reads, so a client cancel alone
// cannot end the request's context: net/http starts watching for the
// peer's disconnect only once the body has been consumed.
func TestServer_DelayedHandlerReleasedByCleanup(t *testing.T) {
	started := make(chan struct{})
	returned := make(chan struct{})
	wrapped := func(w http.ResponseWriter, r *http.Request) {
		close(started)
		Delayed(time.Hour, Success(nil))(w, r)
		close(returned)
	}

	t.Run("subtest", func(t *testing.T) {
		srv := New(t, wrapped)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			<-started
			cancel()
		}()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, BaseURL+"/bot"+Token+"/getMe", strings.NewReader("null"))
		if err != nil {
			t.Fatalf("NewRequestWithContext: %v", err)
		}
		resp, err := srv.Client().Do(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
		if err == nil {
			t.Fatal("Do: expected an error — the handler must not have answered inside this subtest, " +
				"since a client cancel alone does not end a request whose body is unread")
		}
	})

	// The subtest's cleanups (New's Cleanup among them) have already run
	// by the time t.Run returns.
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not return within the patience budget after the server's cleanup ran")
	}
}

// TestServer_AnswersNormallyWhileLaterCleanupsRun covers New's choice of
// its own base context over the test's Context(): the fake server keeps
// answering normally while cleanups registered after New run, because
// New's own cleanup — which cancels that base context — runs after them
// (cleanups run last-registered-first).
func TestServer_AnswersNormallyWhileLaterCleanupsRun(t *testing.T) {
	var got []byte
	t.Run("subtest", func(t *testing.T) {
		srv := New(t, Delayed(10*time.Millisecond, Success(json.RawMessage(`{"reached":true}`))))
		t.Cleanup(func() {
			resp, err := doGet(t, context.Background(), srv.Client())
			if err != nil {
				t.Fatalf("Do: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			got = body
		})
	})
	const want = `{"ok":true,"result":{"reached":true}}` + "\n"
	if string(got) != want {
		t.Errorf("got = %q, want %q", got, want)
	}
}

// TestServer_DelayedInsideSynctestBubble covers a Delayed handler's wait
// as one a testing/synctest bubble sees as durably blocked, so a request
// whose context outlives the delay succeeds, and the virtual time elapsed
// across the call equals the delay exactly.
func TestServer_DelayedInsideSynctestBubble(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const delay = 3 * time.Second
		srv := New(t, Delayed(delay, Success(json.RawMessage(`7`))))
		ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
		defer cancel()
		start := time.Now()
		resp, err := doGet(t, ctx, srv.Client())
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		elapsed := time.Since(start)
		if elapsed != delay {
			t.Errorf("elapsed = %v, want exactly %v", elapsed, delay)
		}
	})
}
