package tg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"path"
	"sync/atomic"
	"time"

	ta "github.com/mymmrac/telego/telegoapi"
)

// caller implements telego's telegoapi.Caller — the one seam every
// outbound Bot API call in this project passes through (design D2). It
// derives the method name and ChatRef from the request (design D4),
// consults the gate (design D12), runs the attempt loop with the limiter
// charged once per attempt (design D2), classifies each attempt's
// evidence (design D5), honours retry_after exactly (design D7), backs
// off between attempts (design D6), and reports exactly one Observation
// per call (design D11).
type caller struct {
	client *Client
}

var _ ta.Caller = (*caller)(nil)

// Call is the one method telego's generated API surface reaches this
// package's whole policy through.
func (c *caller) Call(ctx context.Context, rawURL string, data *ta.RequestData) (*ta.Response, error) {
	start := time.Now()
	method := methodFromURL(rawURL)
	call := Call{
		Method: method,
		Class:  classifyMethod(method),
		Chat:   chatRefFromData(data),
	}
	// A multipart (BodyStream) request streams its body through an
	// io.Pipe that is already drained after the first attempt, so a
	// second attempt would re-read nothing and send a malformed request —
	// design D5's "a multipart request is never retried".
	multipart := data.BodyRaw == nil && data.BodyStream != nil

	if c.client.gate != nil {
		if err := c.client.gate.AllowCall(ctx, call); err != nil {
			tgErr := c.client.giveUpError(method, 0, "", 0, 0, false, fmt.Errorf("gate: %w", err))
			c.client.observe(method, time.Since(start), 0, false, 0)
			return nil, tgErr
		}
	}

	var (
		lastStatus      int
		lastDescription string
		lastRetryAfter  time.Duration
		lastAmbiguous   bool
		lastCause       error
		rateLimited     bool
		attempts        int
	)

	for {
		deadline, hasDeadline := ctx.Deadline()
		t, ok, acquireErr := c.client.limiter.acquire(call, time.Now(), deadline, hasDeadline)
		if acquireErr != nil {
			tgErr := c.client.giveUpError(method, lastStatus, lastDescription, lastRetryAfter, attempts, lastAmbiguous,
				fmt.Errorf("limiter: %w", acquireErr))
			c.client.observe(method, time.Since(start), lastStatus, rateLimited, observedRetries(attempts))
			return nil, tgErr
		}
		if !ok {
			tgErr := c.client.giveUpError(method, lastStatus, lastDescription, lastRetryAfter, attempts, lastAmbiguous,
				errors.New("limiter: required wait ends after the context deadline"))
			c.client.observe(method, time.Since(start), lastStatus, rateLimited, observedRetries(attempts))
			return nil, tgErr
		}
		if err := waitUntil(ctx, t); err != nil {
			tgErr := c.client.giveUpError(method, lastStatus, lastDescription, lastRetryAfter, attempts, lastAmbiguous, err)
			c.client.observe(method, time.Since(start), lastStatus, rateLimited, observedRetries(attempts))
			return nil, tgErr
		}

		resp, httpStatus, wrote, attemptErr := c.doAttempt(ctx, rawURL, data)
		attempts++

		if attemptErr == nil && resp != nil && resp.Ok {
			c.client.observe(method, time.Since(start), httpStatus, rateLimited, observedRetries(attempts))
			return resp, nil
		}

		lastStatus = httpStatus
		if resp != nil && resp.Error != nil {
			lastDescription = resp.Description
		}
		if httpStatus == http.StatusTooManyRequests {
			rateLimited = true
		}
		switch {
		case attemptErr != nil:
			lastCause = attemptErr
		case resp != nil && resp.Error != nil:
			lastCause = errors.New(resp.Error.Error())
		default:
			lastCause = errors.New("tg: attempt failed with no further detail")
		}

		out := classifyAttempt(httpStatus, resp, wrote)
		if multipart {
			// design D5: exactly one attempt, whatever the classifier
			// otherwise concluded.
			out.retryable = false
		}
		lastAmbiguous = out.ambiguous
		lastRetryAfter = out.retryAfter

		if !out.retryable || attempts >= c.client.transport.RetryMaxAttempts {
			tgErr := c.client.giveUpError(method, lastStatus, lastDescription, lastRetryAfter, attempts, lastAmbiguous, lastCause)
			c.client.observe(method, time.Since(start), lastStatus, rateLimited, observedRetries(attempts))
			return nil, tgErr
		}

		// The wait before the next attempt: retry_after is honoured
		// exactly and never shortened or replaced by backoff (design D7);
		// otherwise equal-jitter backoff (design D6). Either way it costs
		// exactly one attempt, never a time budget (design D7).
		var wait time.Duration
		if out.retryAfter > 0 {
			wait = out.retryAfter
		} else {
			wait = backoffDelay(c.client.transport.RetryBaseDelay, c.client.transport.RetryMaxDelay, attempts-1, c.client.jitter)
		}
		waitUntilTime := time.Now().Add(wait)

		if deadline, hasDeadline := ctx.Deadline(); hasDeadline && waitUntilTime.After(deadline) {
			tgErr := c.client.giveUpError(method, lastStatus, lastDescription, lastRetryAfter, attempts, lastAmbiguous, lastCause)
			c.client.observe(method, time.Since(start), lastStatus, rateLimited, observedRetries(attempts))
			return nil, tgErr
		}
		if err := waitUntil(ctx, waitUntilTime); err != nil {
			tgErr := c.client.giveUpError(method, lastStatus, lastDescription, lastRetryAfter, attempts, lastAmbiguous, err)
			c.client.observe(method, time.Since(start), lastStatus, rateLimited, observedRetries(attempts))
			return nil, tgErr
		}
	}
}

// observedRetries converts a raw count of attempts made so far into the
// Retries value Observation documents — "the number of attempts beyond
// the first" — clamped at 0 for the case no attempt has been made yet
// (a gate refusal or a limiter bail-out on the very first pass).
func observedRetries(attempts int) int {
	if attempts <= 0 {
		return 0
	}
	return attempts - 1
}

// doAttempt performs exactly one HTTP round trip and decodes its Bot API
// envelope, bounded by the client's configured AttemptTimeout on top of
// ctx (design D10's AttemptTimeout). It always reports the real HTTP
// status code received (0 when none) and whether httptrace observed the
// request being fully written — design D5's classifier evidence — even
// when it also returns an error.
func (c *caller) doAttempt(ctx context.Context, rawURL string, data *ta.RequestData) (resp *ta.Response, httpStatus int, wrote bool, err error) {
	attemptCtx := ctx
	if to := c.client.transport.AttemptTimeout; to > 0 {
		var cancel context.CancelFunc
		attemptCtx, cancel = context.WithTimeout(ctx, to)
		defer cancel()
	}

	var wroteFlag atomic.Bool
	attemptCtx = httptrace.WithClientTrace(attemptCtx, &httptrace.ClientTrace{
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err == nil {
				wroteFlag.Store(true)
			}
		},
	})

	var body io.Reader
	switch {
	case data.BodyRaw != nil:
		body = bytes.NewReader(data.BodyRaw)
	case data.BodyStream != nil:
		body = data.BodyStream
	default:
		return nil, 0, false, errors.New("tg: request has no body")
	}

	req, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, rawURL, body)
	if err != nil {
		return nil, 0, false, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set(ta.ContentTypeHeader, data.ContentType)

	httpResp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, 0, wroteFlag.Load(), fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()
	httpStatus = httpResp.StatusCode
	wrote = wroteFlag.Load()

	raw, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, httpStatus, wrote, fmt.Errorf("read body: %w", err)
	}

	env := &ta.Response{}
	if err := json.Unmarshal(raw, env); err != nil {
		return nil, httpStatus, wrote, fmt.Errorf("decode json: %w", err)
	}
	return env, httpStatus, wrote, nil
}

// httpClient returns the client's configured *http.Client, or
// http.DefaultClient when none was supplied (design D2's
// Options.HTTPClient).
func (c *caller) httpClient() *http.Client {
	if c.client.httpClient != nil {
		return c.client.httpClient
	}
	return http.DefaultClient
}

// methodFromURL returns the Bot API method name from rawURL — the last
// path segment, whether or not telego's test-server path inserts a
// "/test/" segment before it (design D4).
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
		// No decodable body at all — a multipart request (BodyStream) or
		// no body whatsoever. Either way the destination is unreadable,
		// which is ChatUnknown, not ChatNone (design D4/D12): #22's
		// allowlist gate must refuse an unverifiable destination, not
		// let it through exempt from every per-chat window.
		return ChatRef{Target: ChatUnknown}
	}
	var probe chatIDProbe
	if err := json.Unmarshal(data.BodyRaw, &probe); err != nil {
		// The body is present but not decodable — the same "destination
		// unreadable" failure mode as no body at all (design D4).
		return ChatRef{Target: ChatUnknown}
	}
	if len(probe.ChatID) == 0 {
		// The body decodes cleanly and simply carries no chat_id —
		// getMe, getUpdates, an inline-message edit. There is no
		// destination to check (design D4).
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
