package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// decodeLines parses the buffer as newline-delimited JSON log records.
func decodeLines(t *testing.T, b []byte) []map[string]any {
	t.Helper()
	var records []map[string]any
	dec := json.NewDecoder(bytes.NewReader(b))
	for dec.More() {
		var rec map[string]any
		if err := dec.Decode(&rec); err != nil {
			t.Fatalf("log output is not valid JSON: %v (%q)", err, b)
		}
		records = append(records, rec)
	}
	return records
}

// TestNewLoggerJSONByDefault asserts behavior 1: a logged record is valid JSON
// carrying msg/level and any attributes.
func TestNewLoggerJSONByDefault(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, LogOptions{Level: "info"})

	logger.Info("hello", "host", "abc")

	records := decodeLines(t, buf.Bytes())
	if len(records) != 1 {
		t.Fatalf("expected exactly one JSON record, got %d (%q)", len(records), buf.String())
	}
	rec := records[0]
	if rec["msg"] != "hello" {
		t.Errorf("msg = %v, want %q", rec["msg"], "hello")
	}
	if rec["level"] != "INFO" {
		t.Errorf("level = %v, want %q", rec["level"], "INFO")
	}
	if rec["host"] != "abc" {
		t.Errorf("attr host = %v, want %q", rec["host"], "abc")
	}
}

// TestNewLoggerLevelFiltering asserts behavior 2: at level=warn, Info is
// suppressed while Warn and Error are emitted.
func TestNewLoggerLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, LogOptions{Level: "warn"})

	logger.Debug("debug-msg")
	logger.Info("info-msg")
	logger.Warn("warn-msg")
	logger.Error("error-msg")

	records := decodeLines(t, buf.Bytes())
	var msgs []string
	for _, rec := range records {
		if m, ok := rec["msg"].(string); ok {
			msgs = append(msgs, m)
		}
	}
	want := []string{"warn-msg", "error-msg"}
	if strings.Join(msgs, ",") != strings.Join(want, ",") {
		t.Errorf("emitted msgs = %v, want %v", msgs, want)
	}
}

// TestNewLoggerTextMode asserts behavior 1: with Text=true the handler emits
// slog text format (key=value), not JSON, while still carrying msg/level/attrs.
func TestNewLoggerTextMode(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, LogOptions{Level: "info", Text: true})

	logger.Info("hello", "host", "abc")

	out := buf.String()
	var rec map[string]any
	if json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec) == nil {
		t.Fatalf("text mode produced JSON, want plain text: %q", out)
	}
	for _, want := range []string{"msg=hello", "level=INFO", "host=abc"} {
		if !strings.Contains(out, want) {
			t.Errorf("text output %q missing %q", out, want)
		}
	}
}

// TestNewLoggerTextModeLevelFiltering asserts behavior 3: text mode honors the
// level threshold identically to JSON mode.
func TestNewLoggerTextModeLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, LogOptions{Level: "warn", Text: true})

	logger.Info("info-msg")
	logger.Warn("warn-msg")

	out := buf.String()
	if strings.Contains(out, "info-msg") {
		t.Errorf("info-msg should be filtered at level=warn: %q", out)
	}
	if !strings.Contains(out, "warn-msg") {
		t.Errorf("warn-msg should be emitted at level=warn: %q", out)
	}
}

// TestNewLoggerLevelParsing asserts each documented DN_LOG_LEVEL string maps to
// the right threshold (case-insensitive), and behavior 3: an unknown or empty
// level falls back to info rather than failing.
func TestNewLoggerLevelParsing(t *testing.T) {
	tests := []struct {
		level        string
		debugEmitted bool
		infoEmitted  bool
		warnEmitted  bool
	}{
		{"debug", true, true, true},
		{"DEBUG", true, true, true},
		{"info", false, true, true},
		{"warn", false, false, true},
		{"error", false, false, false},
		{"", false, true, true},      // empty -> info fallback
		{"bogus", false, true, true}, // unknown -> info fallback
	}
	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			var buf bytes.Buffer
			logger := NewLogger(&buf, LogOptions{Level: tt.level})
			logger.Debug("d")
			logger.Info("i")
			logger.Warn("w")

			emitted := map[string]bool{}
			for _, rec := range decodeLines(t, buf.Bytes()) {
				if m, ok := rec["msg"].(string); ok {
					emitted[m] = true
				}
			}
			if emitted["d"] != tt.debugEmitted {
				t.Errorf("debug emitted = %v, want %v", emitted["d"], tt.debugEmitted)
			}
			if emitted["i"] != tt.infoEmitted {
				t.Errorf("info emitted = %v, want %v", emitted["i"], tt.infoEmitted)
			}
			if emitted["w"] != tt.warnEmitted {
				t.Errorf("warn emitted = %v, want %v", emitted["w"], tt.warnEmitted)
			}
		})
	}
}
