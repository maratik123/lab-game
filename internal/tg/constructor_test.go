package tg

import (
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"slices"
	"strings"
	"testing"

	ta "github.com/mymmrac/telego/telegoapi"
)

// namedReader is a minimal ta.NamedReader for TestMultipartRequest below.
type namedReader struct {
	io.Reader
	name string
}

func (n namedReader) Name() string { return n.name }

// errWriter is an io.Writer that always fails, for forcing
// writeMultipartBody's write-error paths (self-review round 5's coverage
// sweep — WriteField's and CreateFormFile's own error returns had zero
// coverage hits, since every prior fixture wrote to a real, succeeding
// pipe).
type errWriter struct{ err error }

func (w errWriter) Write([]byte) (int, error) { return 0, w.err }

// failReader is an io.Reader that always fails, for forcing io.Copy's
// error path inside writeMultipartBody's file loop (self-review round 5's
// coverage sweep).
type failReader struct{ err error }

func (r failReader) Read([]byte) (int, error) { return 0, r.err }

func (r failReader) Name() string { return "broken.bin" }

// TestWriteMultipartBody_FieldWriteErrorIsWrapped forces writer.WriteField
// to fail (constructor.go:70-72) by writing to an always-failing
// io.Writer, and asserts the error is wrapped with the field's name and
// the underlying cause, per writeMultipartBody's %w wrapping.
func TestWriteMultipartBody_FieldWriteErrorIsWrapped(t *testing.T) {
	t.Parallel()
	cause := errors.New("field write failed")
	writer := multipart.NewWriter(errWriter{err: cause})
	err := writeMultipartBody(writer, map[string]string{"key": "value"}, nil)
	if err == nil {
		t.Fatal("writeMultipartBody: expected an error from the failing writer")
	}
	if !errors.Is(err, cause) {
		t.Errorf("writeMultipartBody error = %v, want it to wrap %v", err, cause)
	}
	if !strings.Contains(err.Error(), `"key"`) {
		t.Errorf("writeMultipartBody error = %q, want it to name the failing field", err.Error())
	}
}

// TestWriteMultipartBody_CreateFormFileErrorIsWrapped forces
// writer.CreateFormFile to fail (constructor.go:83-85) the same way, with
// no parameters so the field loop cannot fail first.
func TestWriteMultipartBody_CreateFormFileErrorIsWrapped(t *testing.T) {
	t.Parallel()
	cause := errors.New("create form file failed")
	writer := multipart.NewWriter(errWriter{err: cause})
	err := writeMultipartBody(writer, nil, map[string]ta.NamedReader{"photo": namedReader{Reader: strings.NewReader("x"), name: "x.jpg"}})
	if err == nil {
		t.Fatal("writeMultipartBody: expected an error from the failing writer")
	}
	if !errors.Is(err, cause) {
		t.Errorf("writeMultipartBody error = %v, want it to wrap %v", err, cause)
	}
	if !strings.Contains(err.Error(), `"photo"`) {
		t.Errorf("writeMultipartBody error = %q, want it to name the failing file field", err.Error())
	}
}

// TestWriteMultipartBody_FileCopyErrorIsWrapped forces io.Copy(part, file)
// to fail (constructor.go:86-88) via a file whose Read always errors,
// against a real (succeeding) underlying writer so CreateFormFile itself
// succeeds first.
func TestWriteMultipartBody_FileCopyErrorIsWrapped(t *testing.T) {
	t.Parallel()
	cause := errors.New("read failed")
	var buf strings.Builder
	writer := multipart.NewWriter(&buf)
	err := writeMultipartBody(writer, nil, map[string]ta.NamedReader{"photo": failReader{err: cause}})
	if err == nil {
		t.Fatal("writeMultipartBody: expected an error from the failing reader")
	}
	if !errors.Is(err, cause) {
		t.Errorf("writeMultipartBody error = %v, want it to wrap %v", err, cause)
	}
	if !strings.Contains(err.Error(), `"photo"`) {
		t.Errorf("writeMultipartBody error = %q, want it to name the failing file field", err.Error())
	}
}

// TestJSONRequest_MarshalErrorIsWrapped forces json.Marshal to fail
// (constructor.go:24-26) with a value encoding/json cannot represent (a
// bare channel), and asserts the error is wrapped rather than dropped.
func TestJSONRequest_MarshalErrorIsWrapped(t *testing.T) {
	t.Parallel()
	_, err := (jsonConstructor{}).JSONRequest(make(chan int))
	if err == nil {
		t.Fatal("JSONRequest: expected an error marshalling an unsupported type")
	}
	if !strings.Contains(err.Error(), "tg: marshal request") {
		t.Errorf("JSONRequest error = %q, want it to carry the \"tg: marshal request\" prefix", err.Error())
	}
}

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

// TestMultipartRequest_FieldOrderAndParts drives MultipartRequest itself
// (never exercised by any prior test — TestCaller_MultipartRequestNeverRetried
// builds its RequestData by hand instead) and parses the resulting body
// back with mime/multipart, asserting both the parsed field/file values
// and — the load-bearing part — that fields and files each arrive in
// sorted order, per writeMultipartBody's own "sorted for determinism"
// comment.
func TestMultipartRequest_FieldOrderAndParts(t *testing.T) {
	t.Parallel()
	params := map[string]string{
		"zeta":  "z-value",
		"kilo":  "k-value",
		"alpha": "a-value",
		"mu":    "m-value",
		"echo":  "e-value",
	}
	files := map[string]ta.NamedReader{
		"photo":    namedReader{Reader: strings.NewReader("photo-bytes"), name: "photo.jpg"},
		"document": namedReader{Reader: strings.NewReader("doc-bytes"), name: "doc.txt"},
		"audio":    namedReader{Reader: strings.NewReader("audio-bytes"), name: "audio.ogg"},
	}

	data, err := (jsonConstructor{}).MultipartRequest(params, files)
	if err != nil {
		t.Fatalf("MultipartRequest: %v", err)
	}
	if data.BodyStream == nil {
		t.Fatal("BodyStream is nil")
	}

	_, mediaParams, err := mime.ParseMediaType(data.ContentType)
	if err != nil {
		t.Fatalf("ParseMediaType(%q): %v", data.ContentType, err)
	}
	mr := multipart.NewReader(data.BodyStream, mediaParams["boundary"])

	var fieldOrder, fileOrder []string
	fieldValues := map[string]string{}
	fileNames := map[string]string{}
	fileContents := map[string]string{}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextPart: %v", err)
		}
		content, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("read part %q: %v", part.FormName(), err)
		}
		if part.FileName() != "" {
			fileOrder = append(fileOrder, part.FormName())
			fileNames[part.FormName()] = part.FileName()
			fileContents[part.FormName()] = string(content)
		} else {
			fieldOrder = append(fieldOrder, part.FormName())
			fieldValues[part.FormName()] = string(content)
		}
	}

	wantFieldOrder := []string{"alpha", "echo", "kilo", "mu", "zeta"}
	if !slices.Equal(fieldOrder, wantFieldOrder) {
		t.Errorf("field order = %v, want %v (sorted for determinism)", fieldOrder, wantFieldOrder)
	}
	for k, v := range params {
		if fieldValues[k] != v {
			t.Errorf("field %q = %q, want %q", k, fieldValues[k], v)
		}
	}

	wantFileOrder := []string{"audio", "document", "photo"}
	if !slices.Equal(fileOrder, wantFileOrder) {
		t.Errorf("file order = %v, want %v (sorted for determinism)", fileOrder, wantFileOrder)
	}
	if fileNames["photo"] != "photo.jpg" {
		t.Errorf("photo filename = %q, want %q", fileNames["photo"], "photo.jpg")
	}
	if fileContents["photo"] != "photo-bytes" {
		t.Errorf("photo content = %q, want %q", fileContents["photo"], "photo-bytes")
	}
}
