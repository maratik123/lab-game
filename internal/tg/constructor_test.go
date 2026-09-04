package tg

import (
	"errors"
	"testing"
)

// TestMultipartCloseErr asserts finding 13's fix: writer.Close()'s error
// (dropping the closing multipart boundary) must not be silently
// discarded — it must reach the pipe reader whenever the body itself
// wrote cleanly, while an earlier body error still wins over a Close
// error that is merely a symptom of the pipe having already gone bad.
func TestMultipartCloseErr(t *testing.T) {
	t.Parallel()
	bodyErr := errors.New("body write failed")
	closeErr := errors.New("close failed")

	if got := multipartCloseErr(nil, nil); got != nil {
		t.Errorf("multipartCloseErr(nil, nil) = %v, want nil", got)
	}
	if got := multipartCloseErr(nil, closeErr); !errors.Is(got, closeErr) {
		t.Errorf("multipartCloseErr(nil, closeErr) = %v, want closeErr", got)
	}
	if got := multipartCloseErr(bodyErr, nil); !errors.Is(got, bodyErr) {
		t.Errorf("multipartCloseErr(bodyErr, nil) = %v, want bodyErr", got)
	}
	if got := multipartCloseErr(bodyErr, closeErr); !errors.Is(got, bodyErr) {
		t.Errorf("multipartCloseErr(bodyErr, closeErr) = %v, want bodyErr (the earlier failure wins)", got)
	}
}
