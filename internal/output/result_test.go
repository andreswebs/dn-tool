package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// TestWriteResultShape asserts a full result marshals to the documented schema
// (design §2.8): action/changed/hostId/network with the exact JSON tags.
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
		"action":  "enroll",
		"changed": true,
		"hostId":  "host-abc",
		"network": "defined",
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

// TestWriteResultOmitsEmpty asserts optional fields with omitempty are absent
// when unset; changed is always present (no omitempty).
func TestWriteResultOmitsEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteResult(&buf, Result{Action: "unenroll", Changed: false}); err != nil {
		t.Fatalf("WriteResult: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if _, ok := got["hostId"]; ok {
		t.Errorf("hostId should be omitted when empty, got %v", got)
	}
	if _, ok := got["network"]; ok {
		t.Errorf("network should be omitted when empty, got %v", got)
	}
	if _, ok := got["changed"]; !ok {
		t.Errorf("changed must always be present, got %v", got)
	}
	if _, ok := got["action"]; !ok {
		t.Errorf("action must always be present, got %v", got)
	}
}
