package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

const secretValue = "dnkey-super-secret-value"

// logToJSON logs v as a single attribute through a JSON slog handler and returns
// the captured stderr-style output.
func logToJSON(t *testing.T, key string, v any) string {
	t.Helper()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	logger.Info("msg", slog.Any(key, v))
	return buf.String()
}

func TestSecretRedactsUnderSlog(t *testing.T) {
	out := logToJSON(t, "apiKey", Secret(secretValue))
	if strings.Contains(out, secretValue) {
		t.Fatalf("slog output leaked the secret: %s", out)
	}
	if !strings.Contains(out, "REDACTED") {
		t.Fatalf("slog output missing REDACTED marker: %s", out)
	}
}

func TestSecretRedactsUnderFmtAndJSON(t *testing.T) {
	s := Secret(secretValue)

	for _, verb := range []string{"%v", "%s", "%q", "%+v", "%#v"} {
		if got := fmt.Sprintf(verb, s); strings.Contains(got, secretValue) {
			t.Errorf("fmt %s leaked the secret: %s", verb, got)
		}
	}

	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal(Secret) error: %v", err)
	}
	if strings.Contains(string(b), secretValue) {
		t.Fatalf("json.Marshal leaked the secret: %s", b)
	}
}

func TestLoggingConfigDoesNotLeakAPIKey(t *testing.T) {
	cfg := &Config{
		APIKey:    Secret(secretValue),
		NetworkID: "network-1",
		RoleID:    "role-1",
	}

	out := logToJSON(t, "config", cfg)
	if strings.Contains(out, secretValue) {
		t.Fatalf("logging Config leaked the API key: %s", out)
	}
	if !strings.Contains(out, "REDACTED") {
		t.Fatalf("logged Config missing REDACTED marker for the key: %s", out)
	}
}

func TestSecretRevealReturnsRawValue(t *testing.T) {
	s := Secret(secretValue)
	if got := s.Reveal(); got != secretValue {
		t.Fatalf("Reveal() = %q, want %q", got, secretValue)
	}
}
