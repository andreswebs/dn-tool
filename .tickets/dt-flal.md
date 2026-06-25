---
id: dt-flal
status: closed
deps: [dt-svmu, dt-pe29, dt-a772]
links: []
created: 2026-06-06T14:14:58Z
type: task
priority: 1
assignee: Andre Silva
parent: dt-cmn7
tags: [run, lifecycle, container]
---
# run: compose install/enroll/daemon lifecycle

Implement the core of the `run` lifecycle command for containers/pipelines: install the binary, enroll the host, then exec `dnclient run` in the foreground. `run` is the **non-systemd counterpart** to the three units in §2.7 — it must not assume a systemd context. Signal-driven unenroll is [RUN.signals](dt-n5p5.md); exit propagation is [RUN.exit](dt-r2ks.md).

## Public interface

```go
// internal/run (or cmd)
func Run(ctx context.Context, cfg *config.Config, deps Deps) error
//   1. install (INST.place)  2. enroll (ENR.create)  3. dnclient.Client.Run(ctx, ...) foreground
//   abort before step 3 if install or enroll fails
```

Extends the `dnclient.Client` interface's `Run` method ([ENR.subprocess](dt-a772.md)). Compose the existing building blocks — do **not** fork install/enroll logic.

## Behaviors (TDD order)

1. **Compose order** — `Run` calls install, then enroll, then `dnclient.Client.Run` (assert ordering via mocks).
2. **Enroll failure aborts** — if enroll returns an error, the daemon is **not** started; `Run` returns the error.
3. **Install failure aborts** — likewise before enroll.
4. **Daemon started in foreground** — on success, `dnclient.Client.Run` is invoked and blocks until exit.

## Test strategy

Mock `dnclient.Client` (records `Enroll`/`Run` calls) + mock install/enroll deps. Assert call order and that failures short-circuit before the daemon starts.

## Acceptance

- install → enroll → daemon, in order; any pre-daemon failure aborts cleanly without starting the daemon.

## References

- Design: [Req 5](../docs/dn-tool-design.md#5-container-lifecycle-command), [§2.2 Command surface](../docs/dn-tool-design.md#22-command-surface), [§2.7 NixOS module shape](../docs/dn-tool-design.md#27-nixos-module-shape-servicesdnclient) (the systemd path this parallels).
- Composes [INST.place](dt-svmu.md), [ENR.create](dt-pe29.md), [ENR.subprocess](dt-a772.md).

Parent epic: [dt-cmn7](dt-cmn7.md).

## Notes

**2026-06-06T22:03:50Z**

Implemented internal/run.Run — the compose core of the run lifecycle: install -> enroll -> dnclient.Client.Run(foreground), strictly ordered, aborting before the daemon on any pre-daemon failure. Deps uses function-typed Install/Enroll closures (the standalone command cores, wired by the command layer to reuse — not fork — install/enroll logic) plus a dnclient.Client Daemon. Daemon invoked as 'run -server <APIURL> -name <NetworkName>' (§2.7); its error returns unwrapped so dt-r2ks can map exec.ExitError; install/enroll errors wrap with %w. HELD SCOPE: no main.go command wiring and no signal/exit handling — the run command stays notImplemented because dt-n5p5 (signals: wrap with signal.NotifyContext) and dt-r2ks (exit-status mapping) are the children that build the command-layer context + exit plumbing on top of this core; wiring now would pre-empt their tests. 5 tests cover the 4 ticket behaviors + daemon-error propagation.
