package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// objectKeys returns the keys of the top-level JSON object in data in the order
// they were emitted, so a test can pin the envelope head first rather than trust
// encoding/json's field ordering.
func objectKeys(t *testing.T, data []byte) []string {
	t.Helper()

	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		t.Fatalf("reading opening token: %v (%q)", err, data)
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		t.Fatalf("output should be a JSON object, got %v (%q)", tok, data)
	}

	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			t.Fatalf("reading key: %v (%q)", err, data)
		}
		key, ok := tok.(string)
		if !ok {
			t.Fatalf("expected an object key, got %v (%q)", tok, data)
		}
		keys = append(keys, key)

		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			t.Fatalf("reading value of %q: %v (%q)", key, err, data)
		}
	}
	return keys
}

// TestWriteResultShape asserts a full result marshals to the documented envelope
// (ADR 0005): the schema_version/ok head plus action/changed/host_id/network with
// the exact JSON tags.
func TestWriteResultShape(t *testing.T) {
	var buf bytes.Buffer
	r := Result{Action: "enroll", Changed: true, HostID: "host-abc", Network: "defined"}
	if err := WriteResult(&buf, r); err != nil {
		t.Fatalf("WriteResult: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v (%q)", err, buf.String())
	}

	want := map[string]any{
		"schema_version": float64(SchemaVersion),
		"ok":             true,
		"action":         "enroll",
		"changed":        true,
		"host_id":        "host-abc",
		"network":        "defined",
	}
	if len(got) != len(want) {
		t.Fatalf("key set mismatch: got %v want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("field %q = %v, want %v", k, got[k], v)
		}
	}
}

// TestWriteResultKeyOrder asserts the envelope opens with schema_version then ok
// ahead of the payload. Embedded-struct inlining puts them there, but head-first
// order is the consumer-visible contract, so it is pinned rather than assumed.
func TestWriteResultKeyOrder(t *testing.T) {
	var buf bytes.Buffer
	r := Result{Action: "enroll", Changed: true, HostID: "host-abc", Network: "defined"}
	if err := WriteResult(&buf, r); err != nil {
		t.Fatalf("WriteResult: %v", err)
	}

	want := []string{"schema_version", "ok", "action", "changed", "host_id", "network"}
	got := objectKeys(t, buf.Bytes())
	if len(got) != len(want) {
		t.Fatalf("key order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("key order = %v, want %v", got, want)
		}
	}
}

// TestWriteResultSingleObject asserts the writer emits exactly one JSON object
// followed by a single trailing newline and nothing else.
func TestWriteResultSingleObject(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteResult(&buf, Result{Action: "install", Changed: false}); err != nil {
		t.Fatalf("WriteResult: %v", err)
	}

	out := buf.String()
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("output should end with a newline, got %q", out)
	}
	if strings.Count(out, "\n") != 1 {
		t.Errorf("output should contain exactly one newline, got %q", out)
	}

	dec := json.NewDecoder(strings.NewReader(out))
	var first map[string]any
	if err := dec.Decode(&first); err != nil {
		t.Fatalf("decoding first object: %v", err)
	}
	if dec.More() {
		t.Errorf("expected exactly one JSON object, found trailing data in %q", out)
	}
}

// TestWriteResultOmitsEmpty asserts optional payload fields with omitempty are
// absent when unset; the head and the unconditional payload fields always appear.
func TestWriteResultOmitsEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteResult(&buf, Result{Action: "unenroll", Changed: false}); err != nil {
		t.Fatalf("WriteResult: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if _, ok := got["host_id"]; ok {
		t.Errorf("host_id should be omitted when empty, got %v", got)
	}
	if _, ok := got["network"]; ok {
		t.Errorf("network should be omitted when empty, got %v", got)
	}
	for _, key := range []string{"schema_version", "ok", "action", "changed"} {
		if _, ok := got[key]; !ok {
			t.Errorf("%s must always be present, got %v", key, got)
		}
	}
}

// TestWriteResultNoHTMLEscaping asserts the result stream matches the error
// stream: <, > and & read as themselves rather than as <, so a value stays
// legible on a terminal. No envelope is ever embedded in HTML.
func TestWriteResultNoHTMLEscaping(t *testing.T) {
	var buf bytes.Buffer
	r := Result{Action: "write-config", Changed: true, Network: "a<b>c&d"}
	if err := WriteResult(&buf, r); err != nil {
		t.Fatalf("WriteResult: %v", err)
	}

	if !strings.Contains(buf.String(), `"a<b>c&d"`) {
		t.Errorf("output should not HTML-escape values, got %q", buf.String())
	}
}

// TestOKHead asserts the success head carries the current contract version and a
// true ok, the two fields a consumer branches on before reading any payload.
func TestOKHead(t *testing.T) {
	head := OKHead()
	if head.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", head.SchemaVersion, SchemaVersion)
	}
	if !head.OK {
		t.Error("OKHead should set OK true")
	}
}

// TestEmitJSONWritesOneLine asserts the shared emitter terminates its object with
// exactly one newline: every stdout consumer reading line by line depends on it.
func TestEmitJSONWritesOneLine(t *testing.T) {
	var buf bytes.Buffer
	if err := EmitJSON(&buf, ResultEnvelope{Head: OKHead(), Result: Result{Action: "install"}}); err != nil {
		t.Fatalf("EmitJSON: %v", err)
	}

	out := buf.String()
	if strings.Count(out, "\n") != 1 || !strings.HasSuffix(out, "\n") {
		t.Errorf("EmitJSON should write exactly one trailing newline, got %q", out)
	}
}
