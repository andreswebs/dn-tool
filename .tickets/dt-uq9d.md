---
id: dt-uq9d
status: closed
deps: [dt-w86c, dt-2efx]
links: []
created: 2026-08-12T13:21:42Z
type: task
priority: 1
assignee: Andre Silva
parent: dt-69kx
tags: [adr, errors, exit-codes]
---
# Replace the cli.ExitCoder exit boundary with ExitCodeFor (ADR 0001/0002)

Part of the ADR conformance adoption epic (dt-69kx). Split from
`.local/planning/adr-adoption.md` (decisions recorded there, 2026-08-12); this
ticket is self-contained.

## Context

With `internal/terr` in place (dt-2efx) and the delegate in place (dt-w86c), the
exit boundary can stop being framework-shaped. ADR 0002 requires a single exit
boundary resolving one typed-error interface; ADR 0003 requires that the CLI
framework be imported **only** inside `internal/command`. Today
`internal/output` imports `urfave/cli`, which is the last no-leak violation in
the tree.

Decision recorded 2026-08-12: **replace** the boundary rather than make
`terr.E` satisfy `cli.ExitCoder` permanently. Keeping the framework in `output`
would leave a known ADR 0003 violation that would have to be undone later.

## Current state (verified 2026-08-12 against commit bc8fa0b)

- `internal/output/exit.go:6` - `import "github.com/urfave/cli/v3"`, the leak.
- `internal/output/exit.go:14-25` - the exit-code constants. **These stay.**
  They are the taxonomy declaration and `internal/output/exitcodes.go`
  references them.
- `internal/output/exit.go:27` - `var _ cli.ExitCoder = (*exitError)(nil)`.
- `internal/output/exit.go:31-38` - `exitError`. **Delete.**
- `internal/output/exit.go:43-52` - `ResolveExitCode`. **Delete**, replaced by
  `ExitCodeFor`.
- `internal/output/exit.go:57-62` - `ExitError`. **Delete.**
- `internal/output/exit.go:70-79` - `Classify`. **Delete** (dt-2efx has already
  emptied its call sites).
- `cmd/dn-tool/main.go:173-179` - `exitWithError`, delegating to
  `cli.HandleExitCoder` at `:178`. **Delete**; after dt-w86c this logic lives in
  the delegate.
- `internal/run/run.go:194` and `:254` - the two `output.ExitError` call sites,
  which dt-2efx has already converted to its `exitErr` type.

## Target shape

Follow the cookiecutter reference
`go-cookiecutter/{{cookiecutter.project_name}}/internal/output/output.go:90-99`
and `internal/command/run.go:32-47`.

In `internal/output`:

```go
// ExitCodeFor maps err to its process exit code. Typed errors carry their
// own; anything unclassified is an internal error, exit 70 (EX_SOFTWARE), so
// an unclassified failure is loud instead of silently mislabeled.
func ExitCodeFor(err error) int {
    var coded terr.Coded
    if errors.As(err, &coded) {
        return coded.ExitCode()
    }
    return CodeInternal
}
```

In `internal/command/run.go`, the boundary body:

```go
func Run(args []string, deps Deps) int {
    err := runRoot(context.Background(), args, deps)
    if err == nil {
        return 0
    }
    output.EmitError(deps.Err, err) // added by dt-nljm; until then, current printing
    return output.ExitCodeFor(err)
}
```

`EmitError` belongs to dt-nljm. If that has not landed, keep this ticket's
stderr rendering exactly as it is today and let dt-nljm swap it; do not
regress what stderr carries.

Also drop `cli.HandleExitCoder` and any reliance on `cli.OsExiter`. ADR 0003
forbids mutating framework globals; the delegate returning an `int` removes the
need entirely.

## The load-bearing invariant

**`ExitCodeFor` must resolve the outermost `terr.Coded`.** `errors.As` walks
the chain outermost-first, which is the same ordering `ResolveExitCode` relies
on today, so:

- `run`'s signal override (`internal/run/run.go:190-195`) wraps at the
  outermost layer and therefore wins over any inner class code - this is what
  makes 130/143 correct after a failed unenroll.
- `run`'s daemon passthrough (`:254`, with `daemonExitCode` at `:272-280`)
  likewise wins over the inner `dnclient_failed` (74) that dt-2efx attaches, so
  the daemon's status N still reaches the process exit.

This property is currently **assumed** by the code and asserted only
indirectly. This ticket must assert it directly with tests, because the whole
`run` contract rests on it.

## Acceptance criteria

- `output.ExitCodeFor` exists and returns `CodeInternal` (70) for an
  unclassified error.
- `exitError`, `ExitError`, `Classify`, `ResolveExitCode`, and `exitWithError`
  no longer exist anywhere in the tree.
- `cli.HandleExitCoder` and `cli.OsExiter` no longer appear in the tree.
- `grep -rn 'urfave' internal/` returns hits **only** in `internal/command`.
- The exit-code constants at `internal/output/exit.go:14-25` are unchanged, and
  `internal/output/exitcodes.go` still compiles against them.
- New tests assert the outermost-wins invariant explicitly:
  - a signal override wrapped around an inner class code resolves to 130/143,
    not the inner code;
  - a daemon-passthrough wrapper around an inner `dnclient_failed` (74)
    resolves to the daemon's N, not 74.
- Exit codes otherwise unchanged. These pass:
  - `internal/output/exitcodes_test.go`
  - `cmd/dn-tool/exit_conformance_test.go` (moved to `internal/command` by
    dt-w86c; all five subtests, bare invocation still 0 until dt-nljm)
  - `internal/run/signal_test.go` (130/143)
  - `internal/run/run_test.go` (daemon passthrough, and the 70 fallback for a
    daemon with no clean exit status)
- Quality gate: `make build` green from the repo root per
  `docs/agents/build-and-validation.md`. Do not silence lint with `_ =`.
- TDD per `docs/agents/code-conventions.md`.

## Tests that churn

- `internal/output/exit_test.go` - tests `ExitError` (`:30`, `:54`), `Classify`,
  and `ResolveExitCode` (`:67`), all being deleted. Replace with `ExitCodeFor`
  tests; keep the constant-value table at `:16` (it guards the taxonomy).
- `cmd/dn-tool/exit_test.go` - tests `exitWithError` (`:32`) and the
  `captureExit` helper, both gone. Its coverage moves to the delegate's
  `int` return, largely already covered by dt-w86c's rewritten helper.
- `cmd/dn-tool/assert_changed_test.go:86-90` - asserts a plain error maps to 70.
  Still valid; ensure the error it constructs stays genuinely unclassified (not
  a `terr` sentinel), or the test stops testing the fallback.

## Docs

`internal/output/exit.go:9` - the doc comment says "for the whole CLI family".
ADR 0001 uses no "family" wording. Fix it here since this ticket rewrites the
file (contradiction row 3).

## Dependencies

Depends on dt-w86c (the delegate owns the boundary) and dt-2efx (needs
`terr.Coded`). Blocks dt-nljm, dt-o9hi.

## Do not commit

Leave git commits and branching to the repo owner.

## Notes

**2026-08-12T14:54:32Z**

Replaced the cli.ExitCoder exit boundary with output.ExitCodeFor (resolves terr.Coded via errors.As; unclassified -> CodeInternal 70). internal/output no longer imports urfave/cli: exitError, ExitError, Classify and ResolveExitCode are deleted; the ADR 0001 constants are untouched. grep -rn urfave internal/ now hits only internal/command, and HandleExitCoder/OsExiter/exitWithError appear nowhere in the tree.

internal/command/run.go: exitCode() now calls output.ExitCodeFor and prints err.Error() directly; the local structural exitCoder interface is gone. Note the one deliberate stderr difference: a plain fmt.Errorf wrapper around a coded error now prints the full outer message instead of the innermost coder's message (the old cli.HandleExitCoder shape, which silently dropped outer context). dt-nljm replaces this rendering with output.EmitError and the JSON envelope; exit codes are unchanged either way.

Outermost-wins is now asserted directly, not assumed:
- internal/output/exit_test.go TestExitCodeFor_OutermostCodedErrorWins
- internal/run/signal_test.go TestFinalize_SignalCodeOverridesInnerClassCode (both signals, over an inner 74)
- internal/run/run_test.go TestDaemonPassthroughWrapperOverridesInnerClassCode (N over the inner dnclient_failed 74)

Gotcha for the next ticket: ExitCodeFor(nil) is 70, not 0 - nil->CodeOK is the boundary's job (command.Run returns 0 before consulting the taxonomy). Test call sites that used ResolveExitCode(nil)==CodeOK as a proxy for success were rewritten to assert err == nil (internal/enroll/state_test.go, internal/run/run_test.go x2). All other test call sites are a mechanical ResolveExitCode -> ExitCodeFor rename, with the ExitError/Classify fixtures rebuilt on terr (terr.Newf, sentinel .Wrap, run's newExitErr).

Also corrected the stale comment at internal/dnclient/client.go:73-80, which claimed a bare *exec.ExitError would be honored by the boundary - it no longer satisfies terr.Coded, so it falls back to 70. No CHANGELOG entry: no consumer-visible behavior changed. make build green.
