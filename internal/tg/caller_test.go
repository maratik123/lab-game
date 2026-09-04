package tg

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/mymmrac/telego"
	ta "github.com/mymmrac/telego/telegoapi"

	"github.com/maratik123/lab-game/internal/config"
	"github.com/maratik123/lab-game/internal/tgtest"
)

// newTestClient builds a Client wired to srv, proving the whole chain —
// telego.WithAPICaller/WithRequestConstructor, the D4 derivation, the D9
// limiter and the D12 gate seam — is actually connected end to end
// (design D2, subtask 4's "minimal caller its end-to-end scenarios
// require").
func newTestClient(t *testing.T, srv *tgtest.Server, opts func(*Options)) *Client {
	t.Helper()
	o := Options{
		BaseURL:    tgtest.BaseURL,
		Token:      tgtest.Token,
		Transport:  validTransport(),
		HTTPClient: srv.Client(),
	}
	if opts != nil {
		opts(&o)
	}
	c, err := New(o)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestCaller_EndToEndSuccess(t *testing.T) {
	t.Parallel()
	srv := tgtest.New(t, tgtest.Success(json.RawMessage(`{"id":1,"is_bot":true,"first_name":"x"}`)))
	c := newTestClient(t, srv, nil)

	me, err := c.API().GetMe(context.Background())
	if err != nil {
		t.Fatalf("GetMe: %v", err)
	}
	if me.ID != 1 {
		t.Errorf("me.ID = %d, want 1", me.ID)
	}
}

// TestCaller_DerivesChatKnownFromSendMessage drives a real
// (*telego.Bot).SendMessage call through the whole chain and asserts the
// Call the gate observes: Method "sendMessage", ClassMessage, and a
// ChatKnown target carrying the numeric chat id's raw JSON token (design
// D4).
func TestCaller_DerivesChatKnownFromSendMessage(t *testing.T) {
	t.Parallel()
	srv := tgtest.New(t, tgtest.Success(json.RawMessage(`{"message_id":1,"date":0,"chat":{"id":-1001234567890,"type":"group"}}`)))
	var seen Call
	recording := gateFunc(func(_ context.Context, call Call) error {
		seen = call
		return nil
	})
	c := newTestClient(t, srv, func(o *Options) { o.Gate = recording })

	_, err := c.API().SendMessage(context.Background(), &telego.SendMessageParams{
		ChatID: telego.ChatID{ID: -1001234567890},
		Text:   "hi",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if seen.Method != "sendMessage" {
		t.Errorf("Method = %q, want %q", seen.Method, "sendMessage")
	}
	if seen.Class != ClassMessage {
		t.Errorf("Class = %v, want ClassMessage", seen.Class)
	}
	if seen.Chat.Target != ChatKnown {
		t.Errorf("Chat.Target = %v, want ChatKnown", seen.Chat.Target)
	}
	if seen.Chat.Key != "-1001234567890" {
		t.Errorf("Chat.Key = %q, want %q", seen.Chat.Key, "-1001234567890")
	}
}

// TestCaller_RequestCarriesContentTypeHeader is doAttempt's request-
// construction half of design D2: the outbound HTTP request must carry
// the Content-Type header the request constructor set on RequestData
// (ta.ContentTypeJSON for an ordinary JSON call), or a real Bot API
// server would reject it — a claim tgtest's other handlers cannot check
// since none of them read the request at all.
func TestCaller_RequestCarriesContentTypeHeader(t *testing.T) {
	t.Parallel()
	var gotContentType string
	srv := tgtest.New(t, func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get(ta.ContentTypeHeader)
		tgtest.Success(json.RawMessage(`{"id":1,"is_bot":true,"first_name":"x"}`))(w, r)
	})
	c := newTestClient(t, srv, nil)

	if _, err := c.API().GetMe(context.Background()); err != nil {
		t.Fatalf("GetMe: %v", err)
	}
	if gotContentType != ta.ContentTypeJSON {
		t.Errorf("request Content-Type = %q, want %q", gotContentType, ta.ContentTypeJSON)
	}
}

func TestCaller_GateRefusalReturnsTypedErrorWithNoAttempt(t *testing.T) {
	t.Parallel()
	srv := tgtest.New(t, tgtest.Success(nil))
	refusing := gateFunc(func(context.Context, Call) error { return errors.New("refused by test gate") })
	c := newTestClient(t, srv, func(o *Options) { o.Gate = refusing })

	_, err := c.API().GetMe(context.Background())
	if err == nil {
		t.Fatal("GetMe: expected an error from the refusing gate")
	}
	var tgErr *Error
	if !errors.As(err, &tgErr) {
		t.Fatalf("GetMe: error %v is not a *tg.Error", err)
	}
	if tgErr.Attempts != 0 {
		t.Errorf("Attempts = %d, want 0 (a gate refusal must cost no attempt)", tgErr.Attempts)
	}
}

func TestCaller_LimiterDelaysAndHonoursDeadline(t *testing.T) {
	t.Parallel()
	srv := tgtest.New(t, tgtest.Success(nil))
	// GetMe addresses no chat (ChatNone), so only the class-GLOBAL
	// schedule applies to it — a ChatRate bound would never be reached.
	limits := config.TransportLimits{Other: config.ClassLimits{Global: rate(1, 50*time.Millisecond)}}
	tr := validTransport()
	tr.Limits = limits
	c := newTestClient(t, srv, func(o *Options) { o.Transport = tr })

	// First call goes through immediately.
	if _, err := c.API().GetMe(context.Background()); err != nil {
		t.Fatalf("GetMe (1st): %v", err)
	}

	// A second call, with a deadline shorter than the limiter's required
	// wait, must return promptly with a context error rather than block
	// for the full wait (AC18).
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.API().GetMe(ctx)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("GetMe (2nd): expected an error (deadline too short for the limiter wait)")
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("GetMe (2nd) took %v, want well under the limiter's 50ms wait (deadline must cut it short)", elapsed)
	}
}

// gateFunc adapts a plain function to the Gate interface.
type gateFunc func(ctx context.Context, call Call) error

func (f gateFunc) AllowCall(ctx context.Context, call Call) error { return f(ctx, call) }

// TestCaller_MultipartRequestNeverRetried asserts design D5 directly
// (finding 1 — the retry loop's exits never consulted data.BodyStream,
// so a multipart request against a permanent 500 made 3 attempts,
// re-reading an already-drained io.Pipe on attempts 2-3). A BodyStream
// request must make exactly one attempt, however retryable the response
// looks.
func TestCaller_MultipartRequestNeverRetried(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var calls int32
		srv := tgtest.New(t, func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&calls, 1)
			tgtest.ServerError(http.StatusInternalServerError)(w, r)
		})
		c := newTestClient(t, srv, nil)
		cal := &caller{client: c}

		data := &ta.RequestData{
			ContentType: "multipart/form-data",
			BodyStream:  strings.NewReader("fake multipart body"),
		}
		_, err := cal.Call(context.Background(), tgtest.BaseURL+"/bot"+tgtest.Token+"/sendPhoto", data)
		if err == nil {
			t.Fatal("Call: expected an error from the permanent 500")
		}
		if got := atomic.LoadInt32(&calls); got != 1 {
			t.Errorf("handler invoked %d times, want 1 (a multipart request must never be retried)", got)
		}
	})
}

// TestChatRefFromData is a table test over chatRefFromData's whole
// branch set (findings 2 and 3 — the derivation had zero coverage, and
// two of its four branches fell open to ChatNone instead of
// ChatUnknown). Design D4/D12's table: ChatUnknown when there is no
// decodable body at all (BodyRaw nil, whether or not BodyStream is set,
// and BodyRaw present but undecodable), ChatNone when the body decodes
// cleanly with no chat_id field, ChatKnown otherwise.
func TestChatRefFromData(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data *ta.RequestData
		want ChatRef
	}{
		{
			name: "no body at all",
			data: &ta.RequestData{},
			want: ChatRef{Target: ChatUnknown},
		},
		{
			name: "BodyStream only (a real multipart request)",
			data: &ta.RequestData{BodyStream: strings.NewReader("x")},
			want: ChatRef{Target: ChatUnknown},
		},
		{
			name: "BodyRaw present but not valid JSON",
			data: &ta.RequestData{BodyRaw: []byte("not json")},
			want: ChatRef{Target: ChatUnknown},
		},
		{
			name: "BodyRaw decodes but is not a JSON object (chat_id unreadable)",
			data: &ta.RequestData{BodyRaw: []byte("[1,2,3]")},
			want: ChatRef{Target: ChatUnknown},
		},
		{
			name: "BodyRaw decodes with no chat_id field",
			data: &ta.RequestData{BodyRaw: []byte(`{}`)},
			want: ChatRef{Target: ChatNone},
		},
		{
			name: "BodyRaw carries a numeric chat_id",
			data: &ta.RequestData{BodyRaw: []byte(`{"chat_id":-100123}`)},
			want: ChatRef{Key: "-100123", Target: ChatKnown},
		},
		{
			name: "BodyRaw carries a string chat_id (@channelusername)",
			data: &ta.RequestData{BodyRaw: []byte(`{"chat_id":"@mychannel"}`)},
			want: ChatRef{Key: `"@mychannel"`, Target: ChatKnown},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := chatRefFromData(tt.data); got != tt.want {
				t.Errorf("chatRefFromData(%+v) = %+v, want %+v", tt.data, got, tt.want)
			}
		})
	}
}
