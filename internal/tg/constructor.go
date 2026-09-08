package tg

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"sort"

	ta "github.com/mymmrac/telego/telegoapi"
)

// jsonConstructor implements telegoapi.RequestConstructor with
// encoding/json — the request-marshalling half of the net/http +
// encoding/json swap. It is stateless.
type jsonConstructor struct{}

var _ ta.RequestConstructor = jsonConstructor{}

// JSONRequest marshals parameters with encoding/json into a raw-body
// RequestData.
func (jsonConstructor) JSONRequest(parameters any) (*ta.RequestData, error) {
	body, err := json.Marshal(parameters)
	if err != nil {
		return nil, fmt.Errorf("tg: marshal request: %w", err)
	}
	return &ta.RequestData{ContentType: ta.ContentTypeJSON, BodyRaw: body}, nil
}

// MultipartRequest streams parameters and filesParameters as a multipart
// form body. Not exercised by the MVP — no media mechanic exists yet —
// but implemented so the request constructor's contract is total. Field
// order is sorted for determinism, never map-iteration order.
func (jsonConstructor) MultipartRequest(parameters map[string]string, filesParameters map[string]ta.NamedReader) (*ta.RequestData, error) {
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	go func() {
		bodyErr := writeMultipartBody(writer, parameters, filesParameters)
		closeErr := writer.Close()
		_ = pw.CloseWithError(multipartCloseErr(bodyErr, closeErr))
	}()

	return &ta.RequestData{ContentType: writer.FormDataContentType(), BodyStream: pr}, nil
}

// multipartCloseErr folds writer.Close()'s error (which writes the
// closing multipart boundary and can itself fail) into the error the
// pipe reader ultimately observes, so a truncated body is never reported
// as a clean close. bodyErr, the earlier failure, wins when both are
// non-nil — closeErr is then just a symptom of the pipe having already
// gone bad.
func multipartCloseErr(bodyErr, closeErr error) error {
	if bodyErr != nil {
		return bodyErr
	}
	return closeErr
}

// writeMultipartBody writes every field then every file into writer, in a
// deterministic (sorted-key) order, stopping at the first error.
func writeMultipartBody(writer *multipart.Writer, parameters map[string]string, filesParameters map[string]ta.NamedReader) error {
	fieldNames := make([]string, 0, len(parameters))
	for k := range parameters {
		fieldNames = append(fieldNames, k)
	}
	sort.Strings(fieldNames)
	for _, k := range fieldNames {
		if err := writer.WriteField(k, parameters[k]); err != nil {
			return fmt.Errorf("tg: write multipart field %q: %w", k, err)
		}
	}

	fileNames := make([]string, 0, len(filesParameters))
	for k := range filesParameters {
		fileNames = append(fileNames, k)
	}
	sort.Strings(fileNames)
	for _, k := range fileNames {
		file := filesParameters[k]
		part, err := writer.CreateFormFile(k, file.Name())
		if err != nil {
			return fmt.Errorf("tg: create multipart file %q: %w", k, err)
		}
		if _, err := io.Copy(part, file); err != nil {
			return fmt.Errorf("tg: write multipart file %q: %w", k, err)
		}
	}
	return nil
}
