// Package run composes the container/pipeline lifecycle command: install the
// dnclient binary, enroll the host, then run the daemon in the foreground
// (design Req 5 / §2.2). It is the non-systemd counterpart to the three units in
// §2.7 and assumes no systemd context. Signal-driven unenroll (dt-n5p5) and
// daemon exit-status propagation (dt-r2ks) layer on top of this compose core.
package run

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/andreswebs/dn-tool/internal/config"
	"github.com/andreswebs/dn-tool/internal/dnclient"
	"github.com/andreswebs/dn-tool/internal/output"
)

// defaultUnenrollTimeout bounds the signal-driven unenroll when Deps.UnenrollTimeout
// is unset. It mirrors the unenroll command's own default (design §2.3/§2.5) and
// must stay shorter than the surrounding container stop grace period so the
// remote DELETE completes before the runtime SIGKILLs the process (finding D5).
//
// Deadline-vs-grace contract (dt-r2ks behavior 4): on SIGTERM the container
// runtime gives the process a stop grace period (Docker's default is 10s, but
// orchestrators routinely raise it) before SIGKILL. The unenroll runs inside
// that window under this bounded deadline, so it must stay strictly shorter than
// the deployment's grace period — the binary-side mirror of the systemd module's
// TimeoutStopSec > DN_API_TIMEOUT rule (§2.5 / finding D5). Operators raising
// DN_API_TIMEOUT must raise the runtime grace period in step, or unenroll is
// SIGKILLed mid-DELETE and the host is left enrolled.
const defaultUnenrollTimeout = 10 * time.Second

// defaultReadyTimeout bounds the wait for the daemon's control socket when
// Deps.ReadyTimeout is unset. The daemon must open its socket before
// `dnclient enroll` can connect to it; if it never does (e.g. it comes up but
// wedges), run must fail rather than block forever, so enrollment errors are
// surfaced to the supervisor instead of hanging the container start.
const defaultReadyTimeout = 30 * time.Second

// Deps are the injected lifecycle steps Run and Lifecycle compose. Install,
// Enroll and Unenroll are the standalone commands' cores, wired as closures by
// the command layer so run reuses them verbatim rather than forking their logic;
// tests substitute recorders. Daemon runs `dnclient run` in the foreground.
// WaitReady blocks until the daemon's control socket is ready, gating enroll on
// it. UnenrollTimeout bounds the signal-driven unenroll (zero selects
// defaultUnenrollTimeout); ReadyTimeout bounds the readiness wait (zero selects
// defaultReadyTimeout).
type Deps struct {
	Install         func(context.Context) (output.Result, error)
	Daemon          dnclient.Client
	WaitReady       func(context.Context) error
	Enroll          func(context.Context) (output.Result, error)
	Unenroll        func(context.Context) (output.Result, error)
	ReadyTimeout    time.Duration
	UnenrollTimeout time.Duration
}

// Run installs the dnclient binary, starts the daemon in the background, waits
// for its control socket, enrolls the host, then blocks until the daemon exits.
//
// The daemon must be running before enroll: `dnclient enroll` is a client of the
// running `dnclient run` daemon — it connects to the daemon's control socket to
// hand it the enrollment code (upstream finding: enrollment fails if the daemon
// is not active first). So the order is install → daemon → wait-ready → enroll,
// not the install → enroll → daemon order a single-process reading of Req 5 might
// suggest. The daemon is invoked as `dnclient run -server <api-url> -name <network>`
// (design §2.7).
//
// Any failure before enroll succeeds — install, the daemon dying during startup,
// the socket never appearing, or enroll itself — aborts with enrolled=false: the
// daemon is stopped (the deferred cancel) and the host was never enrolled, so
// nothing is left half-running (Req 5). Once enroll succeeds the daemon's error
// is returned unwrapped — it is the foreground process whose termination is run's
// outcome, and dt-r2ks maps it (including its exec.ExitError exit code) onto
// dn-tool's exit status.
//
// enrolled reports whether the host was actually enrolled: false on any
// pre-enroll abort, true once enroll returns. runAndUnenroll consumes it to skip
// the shutdown unenroll against a never-enrolled host (dt-577o); carrying that
// control-flow fact as a return value rather than a sentinel in the error chain
// means a future pre-enroll step cannot silently re-enable a misleading unenroll
// by forgetting to wrap. The phase label stays in the error message for humans.
func Run(ctx context.Context, cfg *config.Config, deps Deps) (enrolled bool, err error) {
	if _, err := deps.Install(ctx); err != nil {
		return false, fmt.Errorf("install: %w", err)
	}

	// Start the daemon before enroll and stop it on any pre-enroll abort. The
	// buffered channel lets the daemon goroutine deliver its exit status without
	// blocking even when an abort path never reads it.
	daemonCtx, stopDaemon := context.WithCancel(ctx)
	defer stopDaemon()
	daemonDone := make(chan error, 1)
	go func() {
		daemonDone <- deps.Daemon.Run(daemonCtx, "-server", cfg.APIURL, "-name", cfg.NetworkName)
	}()

	if err := waitReady(ctx, deps, daemonDone); err != nil {
		return false, err
	}

	if _, err := deps.Enroll(ctx); err != nil {
		return false, fmt.Errorf("enroll: %w", err)
	}

	return true, <-daemonDone
}

// waitReady blocks until the daemon's control socket is ready (deps.WaitReady),
// racing that against the daemon exiting first. A daemon that dies before it is
// ready — a bad -server, a missing tun device — surfaces its own error rather
// than waiting out the readiness deadline. The readiness wait is bounded by
// ReadyTimeout so a daemon that comes up but never opens its socket cannot hang
// run forever; WaitReady returns the deadline error, which is wrapped here.
func waitReady(ctx context.Context, deps Deps, daemonDone <-chan error) error {
	readyCtx, cancel := context.WithTimeout(ctx, readyTimeout(deps))
	defer cancel()

	ready := make(chan error, 1)
	go func() { ready <- deps.WaitReady(readyCtx) }()

	select {
	case daemonErr := <-daemonDone:
		if daemonErr == nil {
			return errors.New("dnclient daemon exited before it was ready")
		}
		return fmt.Errorf("dnclient daemon exited before it was ready: %w", daemonErr)
	case err := <-ready:
		if err != nil {
			return fmt.Errorf("waiting for dnclient daemon: %w", err)
		}
		return nil
	}
}

// Lifecycle is the container/pipeline entry point: it installs, enrolls, and runs
// the daemon in the foreground, and on SIGTERM/SIGINT stops the daemon and
// unenrolls the host before returning (design Req 5 / §2.7). Because there is no
// systemd context here, it always unenrolls on termination — the module-only
// DN_SKIP_UNENROLL and reboot-vs-poweroff policy (§2.7) does not apply. The
// signal handling is the only difference from runAndUnenroll, which tests drive
// with an injected cancellable context instead of real OS signals.
func Lifecycle(ctx context.Context, cfg *config.Config, deps Deps) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return runAndUnenroll(ctx, cfg, deps)
}

// runAndUnenroll runs the lifecycle under ctx — signal-aware in production, an
// injected cancellable context in tests — then unenrolls the host. The daemon
// blocks until ctx is cancelled (a termination signal) or it exits on its own;
// either way the foreground process is over and the host is unenrolled before
// return. Unenroll runs under a fresh bounded context rooted at
// context.Background(), never the cancelled run context, so the shutdown signal
// does not abort the remote DELETE before it starts (the binary-side counterpart
// to the module's TimeoutStopSec, §2.5 / finding D5).
//
// Exit-code precedence (dt-r2ks): the daemon is the foreground process, so when
// it terminates on its own with an error that outcome dominates dn-tool's exit
// status — its exit code N (carried by an *exec.ExitError) is propagated via
// daemonExitCode, and a coincident unenroll failure is logged rather than allowed
// to mask the daemon's code.
//
// A signal-initiated shutdown is the exception: when the run context is cancelled
// (SIGTERM/SIGINT), killing the daemon is the *intended* way it stops, so the
// signal-kill error it returns is not a failure. In that case the exit status is
// governed by unenroll alone — a clean unenroll yields 0, so a graceful container
// stop is not reported as a failure to the supervisor. Only an unenroll failure
// (the host may remain enrolled) makes a signalled shutdown non-zero. The
// daemon-died-on-its-own check is therefore gated on ctx.Err() == nil. When the
// daemon exits cleanly and unenroll fails, the unenroll error is returned; both
// clean → nil → 0.
func runAndUnenroll(ctx context.Context, cfg *config.Config, deps Deps) error {
	enrolled, daemonErr := Run(ctx, cfg, deps)

	// A pre-enroll step failed (install, daemon startup, readiness, or enroll
	// itself): the host was never enrolled, so there is nothing to unenroll.
	// Surface the failure (it maps to CodeError) but skip the shutdown unenroll,
	// which would only log a misleading "host may still be enrolled" error against
	// a never-enrolled host (dt-577o). The "already enrolled" no-op enroll returns
	// nil, so a clean run still unenrolls on shutdown — the §2.7 container contract
	// is untouched.
	if !enrolled {
		return daemonErr
	}

	unenrollCtx, cancel := context.WithTimeout(context.Background(), unenrollTimeout(deps))
	defer cancel()
	_, unenrollErr := deps.Unenroll(unenrollCtx)

	if ctx.Err() == nil && daemonErr != nil {
		if unenrollErr != nil {
			slog.Error("unenroll failed during shutdown; host may still be enrolled", "error", unenrollErr.Error())
		}
		return output.ExitError(daemonErr, daemonExitCode(daemonErr))
	}
	if unenrollErr != nil {
		slog.Error("unenroll failed during shutdown; host may still be enrolled", "error", unenrollErr.Error())
		return unenrollErr
	}
	return nil
}

// daemonExitCode maps the foreground daemon's termination error onto a process
// exit code, so a supervisor sees the daemon's real result (Req 5). A dnclient
// run that exited with status N — carried by an *exec.ExitError, even when the
// exec layer wraps it — maps to N. A failure with no clean exit status (a
// non-exec error, or a process terminated by a signal, whose ExitCode() is -1)
// has no meaningful N to propagate and maps to output.CodeError rather than a
// negative, out-of-range exit status.
func daemonExitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if code := exitErr.ExitCode(); code >= 0 {
			return code
		}
	}
	return output.CodeError
}

// unenrollTimeout is Deps.UnenrollTimeout when set, else defaultUnenrollTimeout.
func unenrollTimeout(deps Deps) time.Duration {
	if deps.UnenrollTimeout > 0 {
		return deps.UnenrollTimeout
	}
	return defaultUnenrollTimeout
}

// readyTimeout is Deps.ReadyTimeout when set, else defaultReadyTimeout.
func readyTimeout(deps Deps) time.Duration {
	if deps.ReadyTimeout > 0 {
		return deps.ReadyTimeout
	}
	return defaultReadyTimeout
}
