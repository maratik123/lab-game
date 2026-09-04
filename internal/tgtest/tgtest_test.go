package tgtest

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
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
