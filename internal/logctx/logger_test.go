package logctx

import (
	"bytes"
	"encoding/json"
	"log/slog"
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

// JSON is the handler for a non-terminal sink: an agent or a pipeline gets
// parseable diagnostics (ADR 0004).
func TestNewLogger_JSONWhenNotText(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, slog.LevelInfo, false)

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

// Text is the handler for a terminal sink: a human gets readable diagnostics.
func TestNewLogger_TextMode(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, slog.LevelInfo, true)

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

// The threshold filters identically in both handlers.
func TestNewLogger_LevelFiltering(t *testing.T) {
	for _, text := range []bool{false, true} {
		var buf bytes.Buffer
		logger := NewLogger(&buf, slog.LevelWarn, text)

		logger.Debug("debug-msg")
		logger.Info("info-msg")
		logger.Warn("warn-msg")
		logger.Error("error-msg")

		out := buf.String()
		for _, suppressed := range []string{"debug-msg", "info-msg"} {
			if strings.Contains(out, suppressed) {
				t.Errorf("text=%v: %q should be filtered at warn: %q", text, suppressed, out)
			}
		}
		for _, emitted := range []string{"warn-msg", "error-msg"} {
			if !strings.Contains(out, emitted) {
				t.Errorf("text=%v: %q should be emitted at warn: %q", text, emitted, out)
			}
		}
	}
}

// ParseLevel is validated, not silently forgiving: a bad value is an error the
// caller turns into a usage failure (ADR 0004), never a quiet fallback.
func TestParseLevel(t *testing.T) {
	valid := map[string]slog.Level{
		"debug": slog.LevelDebug,
		"DEBUG": slog.LevelDebug,
		"info":  slog.LevelInfo,
		" warn": slog.LevelWarn,
		"error": slog.LevelError,
	}
	for in, want := range valid {
		got, err := ParseLevel(in)
		if err != nil {
			t.Errorf("ParseLevel(%q) returned error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseLevel(%q) = %v, want %v", in, got, want)
		}
	}

	for _, in := range []string{"", "bogus", "trace", "WARNING"} {
		if _, err := ParseLevel(in); err == nil {
			t.Errorf("ParseLevel(%q) = nil error, want a rejection", in)
		}
	}
}

// The rejection message must name the accepted set, since it reaches the
// operator as the usage error's detail.
func TestParseLevel_ErrorNamesAcceptedValues(t *testing.T) {
	_, err := ParseLevel("bogus")
	if err == nil {
		t.Fatal("ParseLevel(bogus) returned no error")
	}
	for _, want := range []string{"bogus", "error", "warn", "info", "debug"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}
