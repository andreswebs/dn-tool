package run

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/andreswebs/dn-tool/internal/config"
	"github.com/andreswebs/dn-tool/internal/dnclient"
	"github.com/andreswebs/dn-tool/internal/output"
)

// exitErrorWithCode runs a trivial subprocess that exits with code, returning the
// resulting *exec.ExitError. dt-r2ks maps the daemon's exit code, which is only
// carried by a genuine *exec.ExitError; constructing one directly is not
// supported, so the honest fixture is a real process (mirroring dt-a772's
// exec-based kill test).
func exitErrorWithCode(t *testing.T, code int) error {
	t.Helper()
	err := exec.Command("sh", "-c", fmt.Sprintf("exit %d", code)).Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("want *exec.ExitError from exit %d, got %v", code, err)
	}
	if exitErr.ExitCode() != code {
		t.Fatalf("fixture exit code = %d, want %d", exitErr.ExitCode(), code)
	}
	return err
}

// signalKilledError runs a subprocess that terminates itself with a signal,
// returning the resulting *exec.ExitError. A signal-terminated process has no
// clean exit status (ExitCode() == -1) — this models the daemon that
// exec.CommandContext kills when the run context is cancelled on shutdown.
func signalKilledError(t *testing.T) error {
	t.Helper()
	err := exec.Command("sh", "-c", "kill -TERM $$").Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("want *exec.ExitError from signal kill, got %v", err)
	}
	if exitErr.ExitCode() != -1 {
		t.Fatalf("want signal-terminated (-1), got code %d", exitErr.ExitCode())
	}
	return err
}

// recorder captures the order of lifecycle steps so tests can assert the
// install → daemon → wait-ready → enroll sequence structurally rather than
// trusting comments. The daemon runs in its own goroutine, so record is mutex
// guarded; the ready-channel handshake (mockDaemon → readyWaiter) is what orders
// daemon.run before wait-ready, not the lock.
type recorder struct {
	mu     sync.Mutex
	events []string
}

func (r *recorder) record(event string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *recorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

func (r *recorder) contains(event string) bool {
	for _, e := range r.snapshot() {
		if e == event {
			return true
		}
	}
	return false
}

// mockDaemon is a dnclient.Client double for the foreground daemon. On Run it
// records "daemon.run", captures its args, and (when ready is non-nil) closes
// ready to signal the control socket is up — the readyWaiter blocks on that, so
// enroll deterministically follows daemon start. It then blocks until either
// exit is closed (modelling the daemon terminating on its own, e.g. after the
// host is enrolled) or ctx is cancelled (modelling a shutdown signal killing it),
// returning err.
type mockDaemon struct {
	rec     *recorder
	gotArgs []string
	err     error
	ready   chan struct{}
	exit    chan struct{}
}

var _ dnclient.Client = (*mockDaemon)(nil)

func (m *mockDaemon) Enroll(_ context.Context, _, _ string) error { return nil }

func (m *mockDaemon) Run(ctx context.Context, args ...string) error {
	m.rec.record("daemon.run")
	m.gotArgs = args
	if m.ready != nil {
		close(m.ready)
	}
	if m.exit != nil {
		select {
		case <-m.exit:
		case <-ctx.Done():
		}
	} else {
		<-ctx.Done()
	}
	return m.err
}

// readyWaiter builds a Deps.WaitReady double that blocks until ready is closed
// (the daemon signalling its socket is up) and then records "wait-ready". If ctx
// is cancelled or its deadline passes first — the readiness-timeout path — it
// returns the context error without recording, so a failed wait leaves no
// "wait-ready" event.
func readyWaiter(rec *recorder, ready <-chan struct{}) func(context.Context) error {
	return func(ctx context.Context) error {
		select {
		case <-ready:
			rec.record("wait-ready")
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// unenrollCapture snapshots the context state observed at unenroll call time.
// Capturing at call time (not after Run returns) is essential: runAndUnenroll
// defers cancelling the unenroll context, so inspecting it afterwards would
// always see a cancelled context regardless of the behavior under test.
type unenrollCapture struct {
	errAtCall   error
	hasDeadline bool
}

// recordingUnenroll builds a run.Deps.Unenroll double. It records the call,
// snapshots the handed context's state into capture (so tests assert the fresh,
// non-cancelled deadline), and returns a configurable error. capture may be nil
// when the context is not under inspection.
func recordingUnenroll(rec *recorder, capture *unenrollCapture, err error) func(context.Context) (output.Result, error) {
	return func(ctx context.Context) (output.Result, error) {
		rec.record("unenroll")
		if capture != nil {
			capture.errAtCall = ctx.Err()
			_, capture.hasDeadline = ctx.Deadline()
		}
		return output.Result{}, err
	}
}

func recordingStep(rec *recorder, name string, err error) func(context.Context) (output.Result, error) {
	return func(context.Context) (output.Result, error) {
		rec.record(name)
		return output.Result{}, err
	}
}

// enrollReleasing builds an Enroll double that records "enroll", closes exit so a
// daemon blocked on it terminates, then returns err. It models the daemon
// exiting on its own only after the host is enrolled — without a manual context
// cancel that could race the readiness wait.
func enrollReleasing(rec *recorder, exit chan struct{}, err error) func(context.Context) (output.Result, error) {
	return func(context.Context) (output.Result, error) {
		rec.record("enroll")
		close(exit)
		return output.Result{}, err
	}
}

// enrollSignalling builds an Enroll double that records "enroll", closes enrolled
// to let the test know the host is enrolled (so it can then cancel to model a
// shutdown signal), and returns nil. The daemon keeps running until cancelled.
func enrollSignalling(rec *recorder, enrolled chan struct{}) func(context.Context) (output.Result, error) {
	return func(context.Context) (output.Result, error) {
		rec.record("enroll")
		close(enrolled)
		return output.Result{}, nil
	}
}

func testConfig() *config.Config {
	return &config.Config{APIURL: "https://api.example", NetworkName: "testnet"}
}

// Behavior 1: Run composes install, then the daemon, then waits for readiness,
// then enrolls — the daemon must be up before enroll, which connects to its
// control socket.
func TestRun_ComposeOrder(t *testing.T) {
	rec := &recorder{}
	ready, exit := make(chan struct{}), make(chan struct{})
	deps := Deps{
		Install:   recordingStep(rec, "install", nil),
		Daemon:    &mockDaemon{rec: rec, ready: ready, exit: exit},
		WaitReady: readyWaiter(rec, ready),
		Enroll:    enrollReleasing(rec, exit, nil),
	}

	enrolled, err := Run(context.Background(), testConfig(), deps)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !enrolled {
		t.Error("enrolled = false, want true once enroll succeeds")
	}

	want := []string{"install", "daemon.run", "wait-ready", "enroll"}
	if got := rec.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("step order = %v, want %v", got, want)
	}
}

// Behavior: enroll runs only after readiness succeeds — the daemon's socket must
// be up first. Modelled by a daemon that never signals ready and a short ready
// timeout; enroll must never run.
func TestRun_ReadinessTimeoutAbortsBeforeEnroll(t *testing.T) {
	rec := &recorder{}
	neverReady := make(chan struct{})
	deps := Deps{
		Install:      recordingStep(rec, "install", nil),
		Daemon:       &mockDaemon{rec: rec}, // ready nil, blocks on ctx
		WaitReady:    readyWaiter(rec, neverReady),
		Enroll:       recordingStep(rec, "enroll", nil),
		ReadyTimeout: 20 * time.Millisecond,
	}

	enrolled, err := Run(context.Background(), testConfig(), deps)
	if err == nil {
		t.Fatal("Run error = nil, want a readiness-timeout error")
	}
	if enrolled {
		t.Error("enrolled = true, want false when the daemon never becomes ready")
	}
	if rec.contains("enroll") {
		t.Errorf("enroll ran despite readiness never succeeding: %v", rec.snapshot())
	}
}

// Behavior: a daemon that dies during startup (before its socket appears)
// surfaces its own error rather than the host being enrolled against a dead
// daemon. Modelled by a daemon that exits immediately and never signals ready.
func TestRun_DaemonExitsBeforeReadyAborts(t *testing.T) {
	rec := &recorder{}
	neverReady := make(chan struct{})
	bootErr := errors.New("bad -server")
	exit := make(chan struct{})
	close(exit) // daemon returns immediately
	deps := Deps{
		Install:   recordingStep(rec, "install", nil),
		Daemon:    &mockDaemon{rec: rec, err: bootErr, exit: exit}, // ready nil
		WaitReady: readyWaiter(rec, neverReady),
		Enroll:    recordingStep(rec, "enroll", nil),
	}

	enrolled, err := Run(context.Background(), testConfig(), deps)
	if !errors.Is(err, bootErr) {
		t.Fatalf("Run error = %v, want wrap of %v", err, bootErr)
	}
	if enrolled {
		t.Error("enrolled = true, want false when the daemon dies before readiness")
	}
	if rec.contains("enroll") {
		t.Errorf("enroll ran after the daemon died: %v", rec.snapshot())
	}
}

// dt-n5p5 behavior 1: a termination signal (modelled by cancelling the run
// context once the host is enrolled) stops the foreground daemon and triggers
// unenroll exactly once, after the daemon returns.
func TestRunAndUnenroll_SignalTriggersUnenroll(t *testing.T) {
	rec := &recorder{}
	ready, enrolled := make(chan struct{}), make(chan struct{})
	deps := Deps{
		Install:         recordingStep(rec, "install", nil),
		Daemon:          &mockDaemon{rec: rec, ready: ready}, // exit nil → blocks until ctx cancel
		WaitReady:       readyWaiter(rec, ready),
		Enroll:          enrollSignalling(rec, enrolled),
		Unenroll:        recordingUnenroll(rec, nil, nil),
		UnenrollTimeout: time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- runAndUnenroll(ctx, testConfig(), deps) }()
	<-enrolled // the signal arrives only once the host is running
	cancel()

	if err := <-errc; err != nil {
		t.Fatalf("runAndUnenroll returned error: %v", err)
	}

	want := []string{"install", "daemon.run", "wait-ready", "enroll", "unenroll"}
	if got := rec.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("step order = %v, want %v", got, want)
	}
}

// A signal-initiated shutdown is the expected way the foreground daemon stops:
// killing it on SIGTERM makes it return a signal-kill error, but that is not a
// failure. When the run context was cancelled and unenroll succeeded, the exit
// status must be 0 — a clean lifecycle must not report failure just because the
// daemon we stopped died by the signal we sent.
func TestRunAndUnenroll_SignalKilledDaemonOnShutdownExitsZero(t *testing.T) {
	rec := &recorder{}
	ready, enrolled := make(chan struct{}), make(chan struct{})
	deps := Deps{
		Install:         recordingStep(rec, "install", nil),
		Daemon:          &mockDaemon{rec: rec, ready: ready, err: signalKilledError(t)}, // exit nil → killed on ctx cancel
		WaitReady:       readyWaiter(rec, ready),
		Enroll:          enrollSignalling(rec, enrolled),
		Unenroll:        recordingUnenroll(rec, nil, nil),
		UnenrollTimeout: time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- runAndUnenroll(ctx, testConfig(), deps) }()
	<-enrolled
	cancel()

	if got := output.ResolveExitCode(<-errc); got != output.CodeOK {
		t.Fatalf("exit code = %d, want %d (graceful shutdown is not a failure)", got, output.CodeOK)
	}
}

// dt-n5p5 behavior 2: even though the run context is cancelled by the signal,
// unenroll runs under a fresh, non-cancelled context carrying its own deadline —
// otherwise the cancelled context would abort the remote DELETE immediately.
func TestRunAndUnenroll_UnenrollGetsFreshBoundedContext(t *testing.T) {
	rec := &recorder{}
	ready, enrolled := make(chan struct{}), make(chan struct{})
	var capture unenrollCapture
	deps := Deps{
		Install:         recordingStep(rec, "install", nil),
		Daemon:          &mockDaemon{rec: rec, ready: ready},
		WaitReady:       readyWaiter(rec, ready),
		Enroll:          enrollSignalling(rec, enrolled),
		Unenroll:        recordingUnenroll(rec, &capture, nil),
		UnenrollTimeout: time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- runAndUnenroll(ctx, testConfig(), deps) }()
	<-enrolled
	cancel()
	if err := <-errc; err != nil {
		t.Fatalf("runAndUnenroll returned error: %v", err)
	}

	if capture.errAtCall != nil {
		t.Fatalf("unenroll context already cancelled at call: %v", capture.errAtCall)
	}
	if !capture.hasDeadline {
		t.Fatal("unenroll context has no deadline; the bounded timeout was not applied")
	}
}

// dt-n5p5 behavior 3: there is no container skip knob — once the foreground
// daemon returns (here a clean exit on its own after enroll, no signal at all),
// the host is always unenrolled.
func TestRunAndUnenroll_AlwaysUnenrollsOnDaemonExit(t *testing.T) {
	rec := &recorder{}
	ready, exit := make(chan struct{}), make(chan struct{})
	deps := Deps{
		Install:         recordingStep(rec, "install", nil),
		Daemon:          &mockDaemon{rec: rec, ready: ready, exit: exit},
		WaitReady:       readyWaiter(rec, ready),
		Enroll:          enrollReleasing(rec, exit, nil),
		Unenroll:        recordingUnenroll(rec, nil, nil),
		UnenrollTimeout: time.Second,
	}

	if err := runAndUnenroll(context.Background(), testConfig(), deps); err != nil {
		t.Fatalf("runAndUnenroll returned error: %v", err)
	}

	want := []string{"install", "daemon.run", "wait-ready", "enroll", "unenroll"}
	if got := rec.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("step order = %v, want %v", got, want)
	}
}

// dt-n5p5 behavior 4: a unenroll failure on an otherwise-clean shutdown is
// surfaced (the host may still be enrolled), so it cannot be silently dropped.
func TestRunAndUnenroll_UnenrollFailureReportedOnCleanDaemonExit(t *testing.T) {
	rec := &recorder{}
	ready, exit := make(chan struct{}), make(chan struct{})
	unenrollErr := errors.New("unenroll boom")
	deps := Deps{
		Install:         recordingStep(rec, "install", nil),
		Daemon:          &mockDaemon{rec: rec, ready: ready, exit: exit},
		WaitReady:       readyWaiter(rec, ready),
		Enroll:          enrollReleasing(rec, exit, nil),
		Unenroll:        recordingUnenroll(rec, nil, unenrollErr),
		UnenrollTimeout: time.Second,
	}

	if err := runAndUnenroll(context.Background(), testConfig(), deps); !errors.Is(err, unenrollErr) {
		t.Fatalf("runAndUnenroll error = %v, want %v", err, unenrollErr)
	}
}

// dt-577o: an install failure aborts before the daemon starts, so the host was
// never enrolled — the shutdown unenroll is skipped (no misleading log), but the
// install failure still surfaces (mapping to CodeError). The daemon must never
// start.
func TestRunAndUnenroll_InstallFailureSkipsUnenroll(t *testing.T) {
	rec := &recorder{}
	installErr := errors.New("install boom")
	deps := Deps{
		Install:         recordingStep(rec, "install", installErr),
		Daemon:          &mockDaemon{rec: rec},
		WaitReady:       readyWaiter(rec, make(chan struct{})),
		Enroll:          recordingStep(rec, "enroll", nil),
		Unenroll:        recordingUnenroll(rec, nil, nil),
		UnenrollTimeout: time.Second,
	}

	err := runAndUnenroll(context.Background(), testConfig(), deps)
	if !errors.Is(err, installErr) {
		t.Fatalf("runAndUnenroll error = %v, want wrap of %v", err, installErr)
	}
	if got := output.ResolveExitCode(err); got != output.CodeError {
		t.Fatalf("exit code = %d, want %d", got, output.CodeError)
	}
	want := []string{"install"}
	if got := rec.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("steps = %v, want %v (daemon, enroll, unenroll must not run)", got, want)
	}
}

// dt-577o: an enroll failure aborts after the daemon started — the host was never
// enrolled, so the daemon is stopped and unenroll is skipped while the enroll
// failure still surfaces.
func TestRunAndUnenroll_EnrollFailureStopsDaemonAndSkipsUnenroll(t *testing.T) {
	rec := &recorder{}
	ready := make(chan struct{})
	enrollErr := errors.New("enroll boom")
	deps := Deps{
		Install:         recordingStep(rec, "install", nil),
		Daemon:          &mockDaemon{rec: rec, ready: ready}, // exit nil → stopped by deferred cancel
		WaitReady:       readyWaiter(rec, ready),
		Enroll:          recordingStep(rec, "enroll", enrollErr),
		Unenroll:        recordingUnenroll(rec, nil, nil),
		UnenrollTimeout: time.Second,
	}

	err := runAndUnenroll(context.Background(), testConfig(), deps)
	if !errors.Is(err, enrollErr) {
		t.Fatalf("runAndUnenroll error = %v, want wrap of %v", err, enrollErr)
	}
	if got := output.ResolveExitCode(err); got != output.CodeError {
		t.Fatalf("exit code = %d, want %d", got, output.CodeError)
	}
	if rec.contains("unenroll") {
		t.Errorf("unenroll ran for a never-enrolled host: %v", rec.snapshot())
	}
}

// The daemon's termination is run's outcome: when the daemon errors, that error
// is returned (for dt-r2ks to map onto the exit status) even if unenroll
// succeeds. This pins the interim precedence dt-r2ks builds on.
func TestRunAndUnenroll_DaemonErrorTakesPrecedence(t *testing.T) {
	rec := &recorder{}
	ready, exit := make(chan struct{}), make(chan struct{})
	daemonErr := errors.New("daemon boom")
	deps := Deps{
		Install:         recordingStep(rec, "install", nil),
		Daemon:          &mockDaemon{rec: rec, ready: ready, exit: exit, err: daemonErr},
		WaitReady:       readyWaiter(rec, ready),
		Enroll:          enrollReleasing(rec, exit, nil),
		Unenroll:        recordingUnenroll(rec, nil, nil),
		UnenrollTimeout: time.Second,
	}

	if err := runAndUnenroll(context.Background(), testConfig(), deps); !errors.Is(err, daemonErr) {
		t.Fatalf("runAndUnenroll error = %v, want %v", err, daemonErr)
	}
}

// Lifecycle wraps runAndUnenroll with signal.NotifyContext; a cancelled parent
// context (after the host is enrolled) propagates to the daemon's context, so the
// wiring is exercised without real OS signals.
func TestLifecycle_ParentCancellationStopsDaemonAndUnenrolls(t *testing.T) {
	rec := &recorder{}
	ready, enrolled := make(chan struct{}), make(chan struct{})
	deps := Deps{
		Install:         recordingStep(rec, "install", nil),
		Daemon:          &mockDaemon{rec: rec, ready: ready},
		WaitReady:       readyWaiter(rec, ready),
		Enroll:          enrollSignalling(rec, enrolled),
		Unenroll:        recordingUnenroll(rec, nil, nil),
		UnenrollTimeout: time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- Lifecycle(ctx, testConfig(), deps) }()
	<-enrolled
	cancel()

	if err := <-errc; err != nil {
		t.Fatalf("Lifecycle returned error: %v", err)
	}
	want := []string{"install", "daemon.run", "wait-ready", "enroll", "unenroll"}
	if got := rec.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("step order = %v, want %v", got, want)
	}
}

// Behavior 4: the daemon runs in the foreground, invoked as
// `dnclient run -server <api-url> -name <network>` (design §2.7).
func TestRun_DaemonInvokedWithServerAndName(t *testing.T) {
	rec := &recorder{}
	ready, exit := make(chan struct{}), make(chan struct{})
	daemon := &mockDaemon{rec: rec, ready: ready, exit: exit}
	deps := Deps{
		Install:   recordingStep(rec, "install", nil),
		Daemon:    daemon,
		WaitReady: readyWaiter(rec, ready),
		Enroll:    enrollReleasing(rec, exit, nil),
	}

	enrolled, err := Run(context.Background(), testConfig(), deps)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !enrolled {
		t.Error("enrolled = false, want true once enroll succeeds")
	}

	want := []string{"-server", "https://api.example", "-name", "testnet"}
	if !reflect.DeepEqual(daemon.gotArgs, want) {
		t.Fatalf("daemon args = %v, want %v", daemon.gotArgs, want)
	}
}

// An install failure aborts before the daemon starts and before enroll.
func TestRun_InstallFailureAbortsBeforeDaemon(t *testing.T) {
	rec := &recorder{}
	installErr := errors.New("install boom")
	deps := Deps{
		Install:   recordingStep(rec, "install", installErr),
		Daemon:    &mockDaemon{rec: rec},
		WaitReady: readyWaiter(rec, make(chan struct{})),
		Enroll:    recordingStep(rec, "enroll", nil),
	}

	enrolled, err := Run(context.Background(), testConfig(), deps)
	if !errors.Is(err, installErr) {
		t.Fatalf("Run error = %v, want wrap of %v", err, installErr)
	}
	if enrolled {
		t.Error("enrolled = true, want false when install aborts before the daemon")
	}
	want := []string{"install"}
	if got := rec.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("steps = %v, want %v (daemon and enroll must not run)", got, want)
	}
}

// The daemon is the foreground process: its termination outcome is Run's outcome
// (full exit-code mapping is dt-r2ks; here we only pin that the error flows out).
func TestRun_DaemonErrorPropagates(t *testing.T) {
	rec := &recorder{}
	ready, exit := make(chan struct{}), make(chan struct{})
	daemonErr := errors.New("daemon exited")
	deps := Deps{
		Install:   recordingStep(rec, "install", nil),
		Daemon:    &mockDaemon{rec: rec, ready: ready, exit: exit, err: daemonErr},
		WaitReady: readyWaiter(rec, ready),
		Enroll:    enrollReleasing(rec, exit, nil),
	}

	enrolled, err := Run(context.Background(), testConfig(), deps)
	if !errors.Is(err, daemonErr) {
		t.Fatalf("Run error = %v, want %v", err, daemonErr)
	}
	if !enrolled {
		t.Error("enrolled = false, want true once enroll succeeds (even when the daemon later errors)")
	}
}

// dt-r2ks behavior 1: the daemon exits with code N → runAndUnenroll returns an
// error that maps (via output.ResolveExitCode) to exit N, so a supervisor sees
// the daemon's real result. The daemon error stays in the chain.
func TestRunAndUnenroll_DaemonExitCodePropagated(t *testing.T) {
	rec := &recorder{}
	ready, exit := make(chan struct{}), make(chan struct{})
	daemonErr := exitErrorWithCode(t, 7)
	deps := Deps{
		Install:         recordingStep(rec, "install", nil),
		Daemon:          &mockDaemon{rec: rec, ready: ready, exit: exit, err: daemonErr},
		WaitReady:       readyWaiter(rec, ready),
		Enroll:          enrollReleasing(rec, exit, nil),
		Unenroll:        recordingUnenroll(rec, nil, nil),
		UnenrollTimeout: time.Second,
	}

	err := runAndUnenroll(context.Background(), testConfig(), deps)
	if got := output.ResolveExitCode(err); got != 7 {
		t.Fatalf("exit code = %d, want 7", got)
	}
	if !errors.Is(err, daemonErr) {
		t.Fatalf("daemon error dropped from chain: %v", err)
	}
}

// dt-r2ks behavior 1 (production shape): the real exec client wraps the
// dnclient error with %w, so the *exec.ExitError is nested. The mapping must
// still extract code N through the wrapping.
func TestRunAndUnenroll_WrappedDaemonExitCodePropagated(t *testing.T) {
	rec := &recorder{}
	ready, exit := make(chan struct{}), make(chan struct{})
	daemonErr := fmt.Errorf("dnclient run failed: %w", exitErrorWithCode(t, 3))
	deps := Deps{
		Install:         recordingStep(rec, "install", nil),
		Daemon:          &mockDaemon{rec: rec, ready: ready, exit: exit, err: daemonErr},
		WaitReady:       readyWaiter(rec, ready),
		Enroll:          enrollReleasing(rec, exit, nil),
		Unenroll:        recordingUnenroll(rec, nil, nil),
		UnenrollTimeout: time.Second,
	}

	if got := output.ResolveExitCode(runAndUnenroll(context.Background(), testConfig(), deps)); got != 3 {
		t.Fatalf("exit code = %d, want 3", got)
	}
}

// dt-r2ks behavior 2: a clean daemon exit with a successful unenroll maps to 0.
func TestRunAndUnenroll_CleanDaemonExitMapsToZero(t *testing.T) {
	rec := &recorder{}
	ready, exit := make(chan struct{}), make(chan struct{})
	deps := Deps{
		Install:         recordingStep(rec, "install", nil),
		Daemon:          &mockDaemon{rec: rec, ready: ready, exit: exit},
		WaitReady:       readyWaiter(rec, ready),
		Enroll:          enrollReleasing(rec, exit, nil),
		Unenroll:        recordingUnenroll(rec, nil, nil),
		UnenrollTimeout: time.Second,
	}

	if got := output.ResolveExitCode(runAndUnenroll(context.Background(), testConfig(), deps)); got != output.CodeOK {
		t.Fatalf("exit code = %d, want %d", got, output.CodeOK)
	}
}

// dt-r2ks behavior 3: a clean daemon exit (0) with a failed unenroll still yields
// a non-zero exit (CodeError), because the host may remain enrolled — the
// unenroll failure must not be masked by the daemon's clean status.
func TestRunAndUnenroll_UnenrollFailureYieldsNonZeroOnCleanExit(t *testing.T) {
	rec := &recorder{}
	ready, exit := make(chan struct{}), make(chan struct{})
	unenrollErr := errors.New("unenroll boom")
	deps := Deps{
		Install:         recordingStep(rec, "install", nil),
		Daemon:          &mockDaemon{rec: rec, ready: ready, exit: exit},
		WaitReady:       readyWaiter(rec, ready),
		Enroll:          enrollReleasing(rec, exit, nil),
		Unenroll:        recordingUnenroll(rec, nil, unenrollErr),
		UnenrollTimeout: time.Second,
	}

	err := runAndUnenroll(context.Background(), testConfig(), deps)
	if got := output.ResolveExitCode(err); got != output.CodeError {
		t.Fatalf("exit code = %d, want %d", got, output.CodeError)
	}
	if !errors.Is(err, unenrollErr) {
		t.Fatalf("unenroll error dropped from chain: %v", err)
	}
}

// dt-r2ks behavior 3 (precedence): when the daemon errors with a code AND
// unenroll fails, the daemon's exit code dominates the exit status (it is the
// foreground outcome); the unenroll failure is logged, not lost to the exit code.
func TestRunAndUnenroll_DaemonExitCodeDominatesUnenrollFailure(t *testing.T) {
	rec := &recorder{}
	ready, exit := make(chan struct{}), make(chan struct{})
	daemonErr := exitErrorWithCode(t, 5)
	deps := Deps{
		Install:         recordingStep(rec, "install", nil),
		Daemon:          &mockDaemon{rec: rec, ready: ready, exit: exit, err: daemonErr},
		WaitReady:       readyWaiter(rec, ready),
		Enroll:          enrollReleasing(rec, exit, nil),
		Unenroll:        recordingUnenroll(rec, nil, errors.New("unenroll boom")),
		UnenrollTimeout: time.Second,
	}

	if got := output.ResolveExitCode(runAndUnenroll(context.Background(), testConfig(), deps)); got != 5 {
		t.Fatalf("exit code = %d, want 5 (daemon code dominates)", got)
	}
}

// dt-r2ks: a daemon failure without a clean exit status — a non-exec error, or a
// process terminated by a signal — has no code N to propagate, so it maps to the
// generic CodeError rather than a nonsensical negative exit status.
func TestRunAndUnenroll_NonExecDaemonErrorMapsToCodeError(t *testing.T) {
	rec := &recorder{}
	ready, exit := make(chan struct{}), make(chan struct{})
	daemonErr := errors.New("daemon boom")
	deps := Deps{
		Install:         recordingStep(rec, "install", nil),
		Daemon:          &mockDaemon{rec: rec, ready: ready, exit: exit, err: daemonErr},
		WaitReady:       readyWaiter(rec, ready),
		Enroll:          enrollReleasing(rec, exit, nil),
		Unenroll:        recordingUnenroll(rec, nil, nil),
		UnenrollTimeout: time.Second,
	}

	if got := output.ResolveExitCode(runAndUnenroll(context.Background(), testConfig(), deps)); got != output.CodeError {
		t.Fatalf("exit code = %d, want %d", got, output.CodeError)
	}
}

// dt-r2ks: a daemon terminated by a signal carries no clean exit status
// (*exec.ExitError.ExitCode() == -1). Mapping that straight through would yield a
// garbage process exit code (e.g. 255); it must map to CodeError instead.
func TestRunAndUnenroll_SignalKilledDaemonMapsToCodeError(t *testing.T) {
	rec := &recorder{}
	ready, exit := make(chan struct{}), make(chan struct{})
	deps := Deps{
		Install:         recordingStep(rec, "install", nil),
		Daemon:          &mockDaemon{rec: rec, ready: ready, exit: exit, err: signalKilledError(t)},
		WaitReady:       readyWaiter(rec, ready),
		Enroll:          enrollReleasing(rec, exit, nil),
		Unenroll:        recordingUnenroll(rec, nil, nil),
		UnenrollTimeout: time.Second,
	}

	if got := output.ResolveExitCode(runAndUnenroll(context.Background(), testConfig(), deps)); got != output.CodeError {
		t.Fatalf("exit code = %d, want %d (no negative/garbage code)", got, output.CodeError)
	}
}
