package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/andreswebs/dn-tool/internal/terr"
)

// decodeEnvelope decodes the single JSON object EmitError wrote, failing the test
// if the bytes are not exactly one newline-terminated object.
func decodeEnvelope(t *testing.T, out string) map[string]any {
	t.Helper()
	if !strings.HasSuffix(out, "\n") {
		t.Fatalf("envelope must end with a newline, got %q", out)
	}
	if strings.Count(out, "\n") != 1 {
		t.Fatalf("envelope must be exactly one line, got %q", out)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("envelope is not valid JSON: %v (%q)", err, out)
	}
	return got
}

// errorObject extracts the nested error object, failing the test when absent.
func errorObject(t *testing.T, env map[string]any) map[string]any {
	t.Helper()
	obj, ok := env["error"].(map[string]any)
	if !ok {
		t.Fatalf("envelope has no error object: %v", env)
	}
	return obj
}

// The contract shape (ADR 0005): the head, then the error object with the code,
// message, hint and details the typed error carries.
func TestEmitError_FullShape(t *testing.T) {
	var buf bytes.Buffer
	err := terr.Newf("dnclient_failed", CodeIO, "check the dnclient logs", "dnclient enroll failed").
		WithDetails(map[string]any{"exit_status": 3})

	EmitError(&buf, err)

	env := decodeEnvelope(t, buf.String())
	if got := env["schema_version"]; got != float64(SchemaVersion) {
		t.Errorf("schema_version = %v, want %d", got, SchemaVersion)
	}
	if got := env["ok"]; got != false {
		t.Errorf("ok = %v, want false", got)
	}
	if len(env) != 3 {
		t.Errorf("envelope keys = %v, want exactly schema_version, ok, error", env)
	}

	obj := errorObject(t, env)
	if got := obj["code"]; got != "dnclient_failed" {
		t.Errorf("error.code = %v, want %q", got, "dnclient_failed")
	}
	if got := obj["message"]; got != "dnclient enroll failed" {
		t.Errorf("error.message = %v, want %q", got, "dnclient enroll failed")
	}
	if got := obj["hint"]; got != "check the dnclient logs" {
		t.Errorf("error.hint = %v, want %q", got, "check the dnclient logs")
	}
	details, ok := obj["details"].(map[string]any)
	if !ok {
		t.Fatalf("error.details = %v, want an object", obj["details"])
	}
	if got := details["exit_status"]; got != float64(3) {
		t.Errorf("error.details.exit_status = %v, want 3", got)
	}
}

// The head is written first, so a consumer reading the raw bytes sees the
// version before anything version-dependent.
func TestEmitError_HeadComesFirst(t *testing.T) {
	var buf bytes.Buffer
	EmitError(&buf, errors.New("boom"))

	const want = `{"schema_version":1,"ok":false,`
	if !strings.HasPrefix(buf.String(), want) {
		t.Errorf("envelope = %q, want it to open with %q", buf.String(), want)
	}
}

// A hint naming a command placeholder stays readable on a terminal: the envelope
// is JSON for machines, never HTML, so <, > and & are not escaped.
func TestEmitError_DoesNotEscapeHTML(t *testing.T) {
	var buf bytes.Buffer
	EmitError(&buf, terr.Newf("missing_argument", CodeUsage,
		"see `dn-tool <command> --help`", "missing required argument"))

	out := buf.String()
	if !strings.Contains(out, "<command>") {
		t.Errorf("envelope = %q, want the hint's <command> placeholder unescaped", out)
	}
	if hint := errorObject(t, decodeEnvelope(t, out))["hint"]; hint != "see `dn-tool <command> --help`" {
		t.Errorf("error.hint = %v, want the placeholder verbatim", hint)
	}
}

// An unclassified error is a missing classification, which must stay visible: it
// renders as internal_error rather than being labeled as something it is not.
func TestEmitError_UnclassifiedIsInternalError(t *testing.T) {
	var buf bytes.Buffer
	EmitError(&buf, errors.New("boom"))

	obj := errorObject(t, decodeEnvelope(t, buf.String()))
	if got := obj["code"]; got != "internal_error" {
		t.Errorf("error.code = %v, want %q", got, "internal_error")
	}
	if got := obj["message"]; got != "boom" {
		t.Errorf("error.message = %v, want %q", got, "boom")
	}
}

// Absent optional fields are absent, not null: a jq pipeline never has to
// distinguish "no hint" from "hint: null".
func TestEmitError_OmitsEmptyOptionalFields(t *testing.T) {
	var buf bytes.Buffer
	EmitError(&buf, terr.Newf("io_failure", CodeIO, "", "local write failed"))

	obj := errorObject(t, decodeEnvelope(t, buf.String()))
	if _, ok := obj["hint"]; ok {
		t.Errorf("hint must be omitted when empty, got %v", obj)
	}
	if _, ok := obj["details"]; ok {
		t.Errorf("details must be omitted when absent, got %v", obj)
	}
	if len(obj) != 2 {
		t.Errorf("error keys = %v, want exactly code and message", obj)
	}
}

// The classification is found through a wrapper, so context added on the way out
// does not lose the code, hint or details.
func TestEmitError_ClassificationSurvivesWrapping(t *testing.T) {
	var buf bytes.Buffer
	coded := terr.Newf("api_rejected", CodeData, "check the request", "api rejected the request").
		WithDetails(map[string]any{"status": 422})
	EmitError(&buf, errors.New("outer: "+coded.Error()))
	if code := errorObject(t, decodeEnvelope(t, buf.String()))["code"]; code != "internal_error" {
		t.Fatalf("a plain outer error must not inherit a code by message: got %v", code)
	}

	buf.Reset()
	EmitError(&buf, wrapped{coded})

	obj := errorObject(t, decodeEnvelope(t, buf.String()))
	if got := obj["code"]; got != "api_rejected" {
		t.Errorf("error.code = %v, want %q", got, "api_rejected")
	}
	if _, ok := obj["details"].(map[string]any); !ok {
		t.Errorf("error.details = %v, want the wrapped error's payload", obj["details"])
	}
}

// wrapped adds context around a coded error while keeping the chain walkable.
type wrapped struct{ error }

func (w wrapped) Error() string { return "resolving configuration: " + w.error.Error() }
func (w wrapped) Unwrap() error { return w.error }

// A payload that cannot marshal degrades to the envelope without details: a
// reporting bug must not swallow the failure being reported.
func TestEmitError_UnmarshalableDetailsDegrades(t *testing.T) {
	var buf bytes.Buffer
	err := terr.Newf("io_failure", CodeIO, "", "local write failed").
		WithDetails(map[string]any{"fn": func() {}})

	EmitError(&buf, err)

	obj := errorObject(t, decodeEnvelope(t, buf.String()))
	if got := obj["code"]; got != "io_failure" {
		t.Errorf("error.code = %v, want %q", got, "io_failure")
	}
	if _, ok := obj["details"]; ok {
		t.Errorf("details must be dropped when unmarshalable, got %v", obj)
	}
}

// A nil error is not a failure, so nothing is written.
func TestEmitError_NilErrorWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	EmitError(&buf, nil)
	if buf.Len() != 0 {
		t.Errorf("EmitError(nil) wrote %q, want nothing", buf.String())
	}
}

// The write is best-effort: a stderr that cannot be written to is unrecoverable
// and must not escalate over the error it describes.
// The test passes by returning: EmitError swallows the write failure instead of
// panicking or propagating it.
func TestEmitError_FailingWriterIsTolerated(_ *testing.T) {
	EmitError(failingWriter{}, errors.New("boom"))
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("stderr is gone") }
