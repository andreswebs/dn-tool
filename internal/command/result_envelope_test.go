package command

import (
	"path/filepath"
	"strings"
	"testing"
)

// ADR 0005: a successful command's stdout is one newline-terminated JSON object
// opening with schema_version then ok. Asserted through the real CLI so the
// wiring in withResult is covered, not just the envelope type.
func TestSuccessfulCommand_StdoutOpensWithEnvelopeHead(t *testing.T) {
	deps, stdout, stderr := testDeps(map[string]string{
		"DN_API_KEY":    "k",
		"DN_NETWORK_ID": "net-1",
		"DN_ROLE_ID":    "role-1",
	})
	path := filepath.Join(t.TempDir(), "dn.env")

	if got := Run([]string{"write-config", path}, deps); got != 0 {
		t.Fatalf("write-config exit code = %d, want 0 (stderr: %s)", got, stderr)
	}

	out := stdout.String()
	const head = `{"schema_version":1,"ok":true,`
	if !strings.HasPrefix(out, head) {
		t.Errorf("stdout = %q, want it to open with %s", out, head)
	}
	if strings.Count(out, "\n") != 1 || !strings.HasSuffix(out, "\n") {
		t.Errorf("stdout = %q, want exactly one trailing newline", out)
	}
}
