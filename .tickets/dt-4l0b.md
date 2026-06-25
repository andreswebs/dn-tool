---
id: dt-4l0b
status: closed
deps: [dt-jbme]
links: [dt-577o]
created: 2026-06-07T20:23:50Z
type: chore
priority: 3
assignee: Andre Silva
parent: dt-cmn7
tags: [run, refactor, architecture]
---
# Make daemon-reached an explicit Run return instead of the errBeforeDaemon sentinel

Move the 'did the daemon start?' control-flow fact out of the error chain: Run returns (daemonStarted bool, err error); runAndUnenroll gates the shutdown unenroll on !started instead of errors.Is(errBeforeDaemon). Behavior-preserving; deletes the errBeforeDaemon sentinel. Revisits the mechanism dt-577o shipped (decision unchanged, implementation hardened against a future pre-daemon step forgetting to wrap). Surfaced by an architecture review (deepening candidate 4).

## Problem

The fact "did we reach the daemon?" is produced in `Run` and consumed in `runAndUnenroll`, but it travels between them smuggled inside the error chain via the `errBeforeDaemon` sentinel (`internal/run/run.go:45,73,76,119`):

```go
return fmt.Errorf("install: %w: %w", errBeforeDaemon, err)   // line 73
return fmt.Errorf("enroll:  %w: %w", errBeforeDaemon, err)   // line 76
// runAndUnenroll:
if errors.Is(daemonErr, errBeforeDaemon) { return daemonErr }  // line 119
```

The interface between the two functions is an error value overloaded to also carry control-flow state. Fragility (noted in dt-577o's own resolution): a future pre-daemon step (e.g. a new "prepare" phase) that forgets `%w: errBeforeDaemon` silently re-enables the misleading shutdown unenroll against a host that was never enrolled. Nothing forces the wrap.

## Blast radius (contained)

- `run.Run` and `errBeforeDaemon` have zero callers outside `internal/run/`; cmd uses only `run.Lifecycle` (`cmd/dn-tool/run.go:31`), whose signature does NOT change.
- `Run` is called by `runAndUnenroll` (`run.go:111`) and directly by package tests (`run_test.go:127,344,364,384,405`).
- So the change is confined to `internal/run/run.go` + `internal/run/run_test.go`.

## Decisions (scoped via architecture-review interview)

1. **Boolean return.** `Run` returns `(daemonStarted bool, err error)` — exactly the one fact the unenroll gate consumes. Chosen over a phase enum (phaseInstall/phaseEnroll/phaseDaemon): only the daemon-reached boundary is consumed today, so richer granularity is unused (YAGNI). Hard to misuse — you can't return without producing the bool, and only the daemon path returns true.
2. **New ticket, supersede dt-577o + breadcrumb.** dt-577o stays closed with its decision record intact; a note there points here. This ticket owns the mechanism replacement. (The dt-577o *decision* — gate unenroll on the daemon having been reached — is unchanged; only its implementation is hardened.)

## Design

```go
// Run installs, enrolls, then runs the daemon in the foreground. daemonStarted
// reports whether the daemon was reached (install and enroll both succeeded):
// runAndUnenroll skips the shutdown unenroll when it is false, since a host that
// was never enrolled has nothing to unenroll (dt-577o). The phase label stays in
// the error message for humans; the machine-readable fact is the bool.
func Run(ctx context.Context, cfg *config.Config, deps Deps) (daemonStarted bool, err error) {
	if _, err := deps.Install(ctx); err != nil {
		return false, fmt.Errorf("install: %w", err)
	}
	if _, err := deps.Enroll(ctx); err != nil {
		return false, fmt.Errorf("enroll: %w", err)
	}
	return true, deps.Daemon.Run(ctx, "-server", cfg.APIURL, "-name", cfg.NetworkName)
}
```

```go
func runAndUnenroll(ctx context.Context, cfg *config.Config, deps Deps) error {
	started, daemonErr := Run(ctx, cfg, deps)
	if !started {
		return daemonErr // pre-daemon failure: nothing was enrolled; skip unenroll
	}

	unenrollCtx, cancel := context.WithTimeout(context.Background(), unenrollTimeout(deps))
	defer cancel()
	_, unenrollErr := deps.Unenroll(unenrollCtx)
	// … exit-code precedence (daemonExitCode, unenroll-vs-daemon) UNCHANGED
}
```

Delete the `errBeforeDaemon` var and its doc block.

## Behavior preserved (all five cases traced)

- **Clean run:** install+enroll ok → `started=true`, daemon runs → unenroll on shutdown. Unchanged.
- **Already-enrolled no-op:** enroll returns nil → `started=true` → daemon runs → still unenrolls. The §2.7 container contract is untouched.
- **Install fails:** `started=false` → skip unenroll, return `install: …` (no `exec.ExitError` → maps to `output.CodeError`). Unchanged.
- **Enroll fails:** `started=false` → skip, return `enroll: …` → `CodeError`. Unchanged.
- **Daemon exits non-zero:** `started=true` → unenroll runs; daemon code dominates via `daemonExitCode`; a coincident unenroll failure is logged, not returned. Unchanged.

## Where

- `internal/run/run.go` — `Run` signature + body; delete `errBeforeDaemon`; `runAndUnenroll` gates on `!started`.
- `internal/run/run_test.go` — Run-direct callsites adopt `started, err := Run(...)`; the dt-577o tests (`TestRunAndUnenroll_{Install,Enroll}FailureSkipsUnenroll`, ~lines 250/276) survive at the `runAndUnenroll` interface and should keep asserting unenroll was NOT invoked (recorder) plus `errors.Is(err, install/enrollErr)`.

## Acceptance criteria

- `Run` returns `(daemonStarted bool, err error)`: `false` on install/enroll failure, `true` once `deps.Daemon.Run` is invoked.
- The `errBeforeDaemon` sentinel is deleted; no `errors.Is(..., errBeforeDaemon)` remains.
- `runAndUnenroll` skips the shutdown unenroll iff `!started`; the exit-code precedence logic is otherwise unchanged.
- All five behaviors above hold; the dt-577o skip-unenroll tests pass (adapted only at the `Run` callsite, not in intent).
- `Lifecycle`'s signature is unchanged; cmd is untouched.
- `make build` is green (quality gate).

## Non-blocking observation (out of scope)

`Run` is exported but has no callers outside `internal/run/`. It could be unexported in a separate cleanup; not folded in here to keep this change minimal.

## References

- Architecture review (deepening candidate 4), report at `.local/tmp/architecture-review-20260607-125102.md`.
- dt-577o (the closed decision whose mechanism this supersedes; decision unchanged).
- design Req 5 / §2.7 (container lifecycle, always-unenroll-on-termination contract).

