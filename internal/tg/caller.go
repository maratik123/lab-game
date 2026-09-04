package tg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"time"

	ta "github.com/mymmrac/telego/telegoapi"
)

// caller implements telego's telegoapi.Caller — the one seam every
// outbound Bot API call in this project passes through (design D2).
// Subtask 4 wires the gate, the limiter and a single HTTP attempt;
// subtask 5 extends Call into a full attempt loop with backoff,
// retry_after and observation.
type caller struct {
	client *Client
}

var _ ta.Caller = (*caller)(nil)

// Call derives the method name and ChatRef from url and data (design D4),
// consults the gate (design D12), acquires from the limiter (design D9)
// and waits until the granted instant — honoring ctx the whole way — then
// performs one HTTP attempt and decodes the Bot API envelope.
func (c *caller) Call(ctx context.Context, rawURL string, data *ta.RequestData) (*ta.Response, error) {
	method := methodFromURL(rawURL)
	call := Call{
		Method: method,
		Class:  classifyMethod(method),
		Chat:   chatRefFromData(data),
	}

	if c.client.gate != nil {
		if err := c.client.gate.AllowCall(ctx, call); err != nil {
			return nil, &Error{Method: method, Err: fmt.Errorf("gate: %w", err)}
		}
	}

	deadline, hasDeadline := ctx.Deadline()
	t, ok := c.client.limiter.acquire(call, time.Now(), deadline, hasDeadline)
	if !ok {
		return nil, &Error{Method: method, Err: errors.New("limiter: required wait ends after the context deadline")}
	}

	if err := waitUntil(ctx, t); err != nil {
		return nil, &Error{Method: method, Err: err}
	}

	return c.attempt(ctx, rawURL, data, method)
}

// attempt performs exactly one HTTP round trip and decodes its Bot API
// envelope, bounded by the client's configured AttemptTimeout on top of
// ctx (design D10's AttemptTimeout).
func (c *caller) attempt(ctx context.Context, rawURL string, data *ta.RequestData, method string) (*ta.Response, error) {
	attemptCtx := ctx
	if to := c.client.transport.AttemptTimeout; to > 0 {
		var cancel context.CancelFunc
		attemptCtx, cancel = context.WithTimeout(ctx, to)
		defer cancel()
	}

	var body io.Reader
	switch {
	case data.BodyRaw != nil:
		body = bytes.NewReader(data.BodyRaw)
	case data.BodyStream != nil:
		body = data.BodyStream
	default:
		return nil, &Error{Method: method, Err: errors.New("tg: request has no body")}
	}

	req, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, rawURL, body)
	if err != nil {
		return nil, &Error{Method: method, Err: fmt.Errorf("build request: %w", err)}
	}
	req.Header.Set(ta.ContentTypeHeader, data.ContentType)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, &Error{Method: method, Err: fmt.Errorf("do request: %w", err)}
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &Error{Method: method, StatusCode: resp.StatusCode, Err: fmt.Errorf("read body: %w", err)}
	}

	env := &ta.Response{}
	if err := json.Unmarshal(raw, env); err != nil {
		return nil, &Error{Method: method, StatusCode: resp.StatusCode, Err: fmt.Errorf("decode json: %w", err)}
	}
	return env, nil
}

// httpClient returns the client's configured *http.Client, or a fresh
// default one when none was supplied (design D2's Options.HTTPClient).
func (c *caller) httpClient() *http.Client {
	if c.client.httpClient != nil {
		return c.client.httpClient
	}
	return http.DefaultClient
}

// methodFromURL returns the Bot API method name from u — the last path
// segment, whether or not telego's test-server path inserts a "/test/"
// segment before it (design D4).
func methodFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return path.Base(u.Path)
}

// chatIDProbe decodes only the chat_id field of a request body, as raw
// JSON — a chat_id may be a @channelusername string rather than a number,
// so the key is the raw token, not an int64 (design D4).
type chatIDProbe struct {
	ChatID json.RawMessage `json:"chat_id"`
}

// chatRefFromData derives a Call's ChatRef from data, per design D4's two
// distinct "no chat id" branches: ChatUnknown when there is no decodable
// body at all (a multipart request, BodyRaw nil), ChatNone when the body
// decodes but carries no chat_id field, ChatKnown otherwise.
func chatRefFromData(data *ta.RequestData) ChatRef {
	if data.BodyRaw == nil {
		if data.BodyStream != nil {
			return ChatRef{Target: ChatUnknown}
		}
		return ChatRef{Target: ChatNone}
	}
	var probe chatIDProbe
	if err := json.Unmarshal(data.BodyRaw, &probe); err != nil || len(probe.ChatID) == 0 {
		return ChatRef{Target: ChatNone}
	}
	return ChatRef{Key: string(probe.ChatID), Target: ChatKnown}
}

// waitUntil blocks until t or until ctx is done, whichever comes first
// (design D9's cancellation obligation, AC18).
func waitUntil(ctx context.Context, t time.Time) error {
	d := time.Until(t)
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
