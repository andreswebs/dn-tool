---
id: dt-r2ks
status: closed
deps: [dt-flal]
links: []
created: 2026-06-06T14:14:58Z
type: task
priority: 1
assignee: Andre Silva
parent: dt-cmn7
tags: [run, signals]
---
# run: propagate daemon exit status

Propagate the `dnclient` daemon's termination outcome to `dn-tool`'s own exit status, so a supervisor sees the real result. Ensure the unenroll deadline fits inside the container's stop grace period.

## Public interface

Extends [RUN.compose](dt-flal.md)/[RUN.signals](dt-n5p5.md): capture the daemon's exit (the error from `dnclient.Client.Run`, including its exit code via `exec.ExitError`) and map it to `dn-tool`'s exit code ([EXIT.map](dt-icq8.md)).

## Behaviors (TDD order)

1. **Daemon exit code propagated** — daemon exits with code N → `Run` returns an error that maps to exit N (extract via `errors.As(&exec.ExitError{})`).
2. **Clean daemon exit → 0** — daemon exits 0 (e.g. after signal-driven shutdown) → `dn-tool` exits 0 (assuming unenroll succeeded).
3. **Unenroll failure dominates appropriately** — define and test precedence when both the daemon and unenroll report problems (recommended: a failed unenroll yields a non-zero exit even if the daemon exited 0, because the host may still be enrolled — document the rule).
4. **Deadline fits grace** — `DN_API_TIMEOUT` for the signal-driven unenroll is shorter than a typical container stop grace; document the relationship (mirror of module `TimeoutStopSec`).

## Test strategy

Mock `dnclient.Client.Run` to return an `*exec.ExitError` with a known code; assert the mapped `dn-tool` exit code. Add a case where unenroll fails to pin down the precedence rule.

## Acceptance

- Daemon exit status flows to `dn-tool`'s exit; unenroll-vs-daemon precedence is defined and tested; deadline/grace relationship documented.

## References

- Design: [Req 5](../docs/dn-tool-design.md#5-container-lifecycle-command).
- Exit mapping: [EXIT.map](dt-icq8.md). Composes [RUN.compose](dt-flal.md).

Parent epic: [dt-cmn7](dt-cmn7.md).

## Notes

**2026-06-06T22:17:34Z**

Daemon exit-status propagation: runAndUnenroll now does EXPLICIT mapping via daemonExitCode(err) wrapped in output.ExitError, instead of relying on the (incidental) fact that *exec.ExitError satisfies cli.ExitCoder. The mapping: errors.As(&*exec.ExitError) + ExitCode()>=0 -> N; otherwise (non-exec error, or signal-terminated process whose ExitCode()==-1) -> output.CodeError. This fixes a real bug: a signal-killed daemon previously mapped straight to -1 (-> os.Exit garbage/255). Precedence: daemon error dominates the exit code (foreground outcome); a coincident unenroll failure is slog.Error-logged, not allowed to mask the daemon code. Clean daemon exit + failed unenroll -> unenroll error returned (CodeError) since the host may remain enrolled. Both clean -> nil -> 0. Tests construct genuine *exec.ExitError via 'sh -c exit N' and signal-kill via 'sh -c kill -TERM $$' (mirrors dt-a772). Behavior-4 deadline-vs-grace relationship documented on defaultUnenrollTimeout (must be < container stop grace, mirror of module TimeoutStopSec>DN_API_TIMEOUT). HELD SCOPE: no main.go wiring -- 'run' stays notImplemented; the dt-cmn7 epic wires run.Lifecycle (per dt-n5p5/dt-flal held-scope notes).
