---
id: dt-n5p5
status: closed
deps: [dt-flal, dt-2t72]
links: []
created: 2026-06-06T14:14:58Z
type: task
priority: 1
assignee: Andre Silva
parent: dt-cmn7
tags: [run, signals]
---
# run: signal handling -> unenroll

Add signal handling to `run`: while the daemon runs, a termination signal (`SIGTERM`/`SIGINT`) triggers `unenroll`, then the process exits. Use `signal.NotifyContext` (graceful-shutdown pattern) to cancel the daemon's context and run the unenroll path ([UNE.delete](dt-2t72.md)).

## Public interface

Wraps [RUN.compose](dt-flal.md): build `ctx, stop := signal.NotifyContext(parent, SIGTERM, SIGINT)`; pass `ctx` to the daemon; on cancellation, run `Unenroll` under a fresh bounded context (`DN_API_TIMEOUT`), then return.

## Behaviors (TDD order)

1. **Signal triggers unenroll** — simulate cancellation (cancel the injected context) → `Unenroll` is called exactly once.
2. **Unenroll runs under its own bounded context** — even though the run context is cancelled, unenroll gets a fresh `DN_API_TIMEOUT` deadline (otherwise the cancelled ctx would abort the DELETE immediately).
3. **Always unenroll in container mode** — there is no `DN_SKIP_UNENROLL`/reboot-vs-poweroff branching here (that is systemd-module-only, §2.7); `run` always unenrolls on signal. (If a container skip knob is ever wanted, it's a separate decision — flag, don't add silently.)
4. **Unenroll failure is reported** — a failing unenroll surfaces its error/exit (ties to RUN.exit), but the daemon is already stopping.

## Test strategy

Inject a cancellable context and a mock daemon `Run` that blocks until ctx cancel; mock `Unenroll`. Trigger cancel; assert `Unenroll` called once with a non-cancelled (fresh-deadline) context. Avoid real OS signals in unit tests; test the wiring function with an injected trigger.

## Acceptance

- SIGTERM/SIGINT → unenroll → exit; unenroll uses a fresh bounded context; always unenrolls (no module skip logic); failures reported.

## References

- Design: [Req 5](../docs/dn-tool-design.md#5-container-lifecycle-command), [§2.7 NixOS module shape](../docs/dn-tool-design.md#27-nixos-module-shape-servicesdnclient) (why skip/reboot logic is module-only).
- golang skill: graceful-shutdown (`signal.NotifyContext`). Unenroll path: [UNE.delete](dt-2t72.md).

Parent epic: [dt-cmn7](dt-cmn7.md).

## Notes

**2026-06-06T22:11:06Z**

Added run.Lifecycle (signal.NotifyContext SIGINT/SIGTERM wrapper) and the testable core runAndUnenroll in internal/run/run.go. Extended Deps with Unenroll func + UnenrollTimeout (zero -> defaultUnenrollTimeout 10s, mirrors unenroll cmd default / D5). Flow: Run (install->enroll->daemon) blocks until ctx cancel (signal) or clean daemon exit, then ALWAYS unenrolls (no container skip knob; DN_SKIP_UNENROLL/reboot-vs-poweroff is module-only per 2.7). Unenroll runs under a FRESH context.WithTimeout(context.Background(),...) — NOT the cancelled run ctx — so the shutdown signal doesn't abort the DELETE before it starts. Precedence (interim, dt-r2ks owns final + exec.ExitError code mapping): daemon error is returned (foreground outcome); a unenroll failure on a clean daemon exit is returned; a unenroll failure coinciding with a daemon error is slog.Error-logged, not dropped. Test subtlety: snapshot the unenroll ctx state AT CALL TIME (unenrollCapture{errAtCall,hasDeadline}) — runAndUnenroll defers cancel(), so inspecting the captured ctx after Run returns always sees cancelled. Tests use injected cancellable ctx + blockUntilCancel mock daemon, no real OS signals; Lifecycle covered via pre-cancelled parent ctx. HELD SCOPE: no main.go wiring — run cmd still notImplemented; dt-cmn7 epic wires run.Lifecycle and dt-r2ks adds exit-code mapping.
