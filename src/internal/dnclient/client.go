// Package dnclient runs the proprietary dnclient binary as a subprocess: Client
// abstracts the enroll/run invocations so callers are testable without the real
// binary, and BinaryPath single-sources the executable's location so the
// installer (dninstall) writes where this package execs. Reading local state is
// dnstate; downloading and placing the binary is dninstall.
package dnclient

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
)

// binaryName is the dnclient executable's file name within the bin dir.
const binaryName = "dnclient"

// BinaryPath is the dnclient executable path within binDir. It single-sources
// the path the installer (dninstall) writes to and the enroll/run commands
// exec, so the two can never disagree; the command layer computes it once and
// passes the value to both NewExecClient and dninstall.Install.
func BinaryPath(binDir string) string {
	return filepath.Join(binDir, binaryName)
}

// Client abstracts the proprietary dnclient subprocess so enroll and run are
// testable without the real binary (design §2.9, Q24 → A). Implementations
// invoke the installed dnclient binary; tests substitute a mock.
type Client interface {
	// Enroll runs `dnclient enroll -name <networkName> -code <code>`, joining the
	// host to the network. The code is single-use and sensitive: it is handed to
	// the subprocess in-memory only and never logged (design Req 7 / SEC5).
	Enroll(ctx context.Context, networkName, code string) error
	// Run runs `dnclient run <args...>` — the foreground daemon, invoked with
	// e.g. `-server <api-url> -name <networkName>`.
	Run(ctx context.Context, args ...string) error
}

// execClient is the production Client: it execs the installed dnclient binary.
type execClient struct {
	binPath string
}

var _ Client = (*execClient)(nil)

// NewExecClient returns a Client that execs the dnclient binary at binPath
// (placed by Install under DN_CLIENT_BIN_DIR).
func NewExecClient(binPath string) Client {
	return &execClient{binPath: binPath}
}

func (c *execClient) Enroll(ctx context.Context, networkName, code string) error {
	return c.exec(ctx, "enroll", "enroll", "-name", networkName, "-code", code)
}

func (c *execClient) Run(ctx context.Context, args ...string) error {
	return c.exec(ctx, "run", append([]string{"run"}, args...)...)
}

// exec runs the binary with args under ctx. The child's stdout and stderr are
// wired to the parent's stderr — stdout is reserved for the JSON result
// contract (design §2.8). action names the operation for diagnostics; the
// argument list, which may carry the enrollment code, is never logged.
func (c *execClient) exec(ctx context.Context, action string, args ...string) error {
	slog.Default().Debug("running dnclient", "action", action, "bin", c.binPath)
	cmd := exec.CommandContext(ctx, c.binPath, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("dnclient %s failed: %w", action, err)
	}
	return nil
}
