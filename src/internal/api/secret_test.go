package api

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestEnrollmentCodeNeverLogged(t *testing.T) {
	const fakeCode = "enroll-code-do-not-log-me"
	resp := &HostAndCode{
		Host:           Host{ID: "host-1", Name: "h1"},
		EnrollmentCode: EnrollmentCode{Code: fakeCode, LifetimeSeconds: 300},
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	logger.Info("created host", slog.Any("response", resp))

	out := buf.String()
	if strings.Contains(out, fakeCode) {
		t.Fatalf("logging the create response leaked the enrollment code: %s", out)
	}
	if !strings.Contains(out, "REDACTED") {
		t.Fatalf("logged response missing REDACTED marker for the code: %s", out)
	}
}
