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
