---
id: dt-s5tr
status: closed
deps: []
links: [dt-577o]
created: 2026-06-07T11:09:13Z
type: task
priority: 0
assignee: Andre Silva
parent: dt-cmn7
tags: [run, cli, wiring]
---
# Wire the run command (container lifecycle) into the CLI

Connect internal/run to the run subcommand; it is currently Action: notImplemented despite the internal package being complete and tested.

## Design

internal/run (Lifecycle/Run/exit-mapping) is complete and fully unit-tested, but the `run` subcommand in cmd/dn-tool/main.go is still `Action: notImplemented` — so `dn-tool run` returns "not implemented", exit 1. The internal package was never connected to the CLI (no child of epic dt-cmn7 owned main.go wiring). This task wires it, on a best-effort basis, following TDD where the seams allow.

## Public interface (cmd/dn-tool, package main)

```go
// run.go (new)
func runAction(ctx context.Context, cmd *cli.Command) error           // cli.ActionFunc: loadConfig -> runRun
func runRun(ctx context.Context, cfg *config.Config, configRoot string) error  // run.Lifecycle(ctx, cfg, productionRunDeps(...))
func productionRunDeps(cfg *config.Config, configRoot string) run.Deps // assembles the lifecycle deps from the existing command cores

// install.go (refactor — extract reusable core)
func runInstall(ctx context.Context, cfg *config.Config) (output.Result, error) // installAction delegates to it
```

`productionRunDeps` reuses the standalone command cores verbatim (the design intent for internal/run):
- Install:  func(ctx) { return runInstall(ctx, cfg) }
- Enroll:   func(ctx) { return runEnroll(ctx, cfg, configRoot, false) }   // run has no --force
- Daemon:   dnclient.NewExecClient(dnclient.BinaryPath(cfg.ClientBinDir))
- Unenroll: func(ctx) { return runUnenroll(ctx, cfg, configRoot) }
- UnenrollTimeout: unenrollTimeout(cfg)   // honor DN_API_TIMEOUT; don't let run's 10s default cap a larger value

The `run` command is a plain cli.ActionFunc (NOT withResult): run is a long-running foreground command whose outcome is the daemon's exit (run.Lifecycle returns an output.ExitError carrying the daemon's code, which main's exitWithError already honors). --assert-changed does not apply to run.

## Why not deeper cmd-level behavior tests

run.Lifecycle/Run/exit-code mapping are already exhaustively tested in internal/run with mock deps. The cmd layer is a thin adapter; its real risk is (a) the command stays unwired and (b) the deps are misassembled. Install closure + Daemon need a real Linux host + real dnclient binary + network, so they are not hermetically testable cross-OS (executing `run` on Linux CI would hit api.defined.net). I therefore test the two hermetic deps closures behaviorally and the rest structurally.

## TDD plan (vertical slices)

- Cycle 0 (refactor @ GREEN): extract runInstall from installAction; installAction = loadConfig + runInstall. No behavior change. `make build` stays green. (install has no cmd-level test today; dnclient.Install is covered in internal/dnclient.)
- Cycle 1 (RED->GREEN, tracer): run_test.go — productionRunDeps(cfg, root).Unenroll(ctx) performs a real unenroll: httptest server asserts DELETE /v1/hosts/<id>, temp configRoot's network dir is removed, Result{Action:"unenroll",Changed:true,...}. Proves run reuses the unenroll core. Create run.go to pass.
- Cycle 2 (RED->GREEN): productionRunDeps(cfg, root).Enroll(ctx) with a local dnclient.yml present returns the no-op cell (Changed=false), zero network/dnclient. Proves run reuses the enroll core.
- Cycle 3 (RED->GREEN): deps completeness — Daemon non-nil and Install/Enroll/Unenroll non-nil (a nil field would nil-panic in Lifecycle); UnenrollTimeout == unenrollTimeout(cfg).
- Cycle 4 (wire + cleanup @ GREEN): main.go run command Action=runAction; remove now-unused notImplemented func + errNotImplemented var; update main_test.go — drop TestNewApp_SubcommandsReturnNotImplemented (no stubs remain) and add a guard that every subcommand has a non-nil Action.

## Files
- src/cmd/dn-tool/install.go   (refactor: extract runInstall)
- src/cmd/dn-tool/run.go       (new: runAction, runRun, productionRunDeps)
- src/cmd/dn-tool/run_test.go  (new: hermetic Unenroll + Enroll-no-op + deps-completeness tests)
- src/cmd/dn-tool/main.go      (wire run; remove notImplemented/errNotImplemented)
- src/cmd/dn-tool/main_test.go (drop stub test; add all-commands-wired guard)

## Out of scope
No new run flags; no real-binary/network e2e (that is the separate Layer 2/3 integration harness). Epic dt-cmn7 was closed before this wiring existed; not reopening it — this task completes that gap.

## Acceptance Criteria

- `dn-tool run` is wired: the run command Action is runAction, not the notImplemented stub; notImplemented func and errNotImplemented var are removed.
- runRun composes install -> enroll -> foreground daemon -> unenroll via run.Lifecycle, reusing runInstall/runEnroll/runUnenroll verbatim.
- Hermetic cmd tests pass: Unenroll closure performs the real DELETE + local removal; Enroll closure returns the no-op cell when locally enrolled; all run.Deps fields are populated; UnenrollTimeout honors DN_API_TIMEOUT.
- make build is green (gofmt, vet, golangci-lint 0 issues, all tests, cross-compile).


## Notes

**2026-06-07T11:16:11Z**

DONE. Wired internal/run into the `run` subcommand.

Changes:
- cmd/dn-tool/install.go: extracted runInstall(ctx, cfg) core from installAction (refactor @ GREEN; installAction = loadConfig + runInstall). Mirrors the enroll/unenroll core+action split and lets run reuse install verbatim.
- cmd/dn-tool/run.go (new): runAction (cli.ActionFunc) -> runRun(ctx, cfg, defaultConfigRoot) -> run.Lifecycle(ctx, cfg, productionRunDeps(cfg, root)). productionRunDeps assembles run.Deps from the existing cores: Install=runInstall, Enroll=runEnroll(...,false) (no --force on the container path), Daemon=dnclient.NewExecClient(BinaryPath(cfg.ClientBinDir)), Unenroll=runUnenroll, UnenrollTimeout=unenrollTimeout(cfg).
- cmd/dn-tool/main.go: run command Action notImplemented -> runAction; removed now-unused notImplemented func + errNotImplemented var.
- cmd/dn-tool/main_test.go: dropped the stale stub test; added TestNewApp_AllSubcommandsWired (nil-Action guard — the regression class that left run unwired).

Decisions:
- run is a plain cli action, NOT withResult: it is a long-running foreground command whose outcome is the daemon's exit (run.Lifecycle returns an output.ExitError carrying the daemon's code, honored by main.exitWithError). --assert-changed does not apply; run emits no stdout JSON result (its inner step Results are discarded by run.Run by design — output is the daemon's stderr + exit code).
- UnenrollTimeout=unenrollTimeout(cfg) so a configured DN_API_TIMEOUT is honored on shutdown rather than capped by run's 10s default.

TDD: tracer = TestProductionRunDeps_UnenrollClosureUnenrolls (httptest DELETE + temp configRoot removal proves the unenroll core is reused). Then EnrollClosureNoOpWhenLocallyEnrolled (no-op cell, hermetic) and AllStepsWired (no nil step/daemon; UnenrollTimeout honored). internal/run's Lifecycle/Run/exit-mapping were already exhaustively tested, so cmd tests cover only the wiring; the Install closure + Daemon need a real Linux host + dnclient + network, so they are not hermetically testable cross-OS (asserted structurally).

Verification: make build green (vet, golangci-lint 0 issues, all tests incl internal/run, cross-compile). Binary smoke: `dn-tool run --help` shows the real command; `env -i dn-tool run` reaches the install OS-gate ("requires linux") + runs the shutdown-unenroll path, exit 1 (no longer "not implemented").

OBSERVATION (out of scope, not changed): run.Lifecycle always attempts unenroll on termination even when install/enroll failed early (host never enrolled) — visible as the "unenroll failed; DN_API_KEY required" log on the darwin smoke. This is internal/run's established "always unenroll" contract (dt-n5p5), benign/idempotent (unenroll of a not-enrolled host is a clear error, no orphan), but noisy. Possible follow-up: skip unenroll when enroll never succeeded. Left to internal/run owners.
