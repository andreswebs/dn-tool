---
id: dt-icq8
status: closed
deps: [dt-ecf9]
links: []
created: 2026-06-06T14:14:57Z
type: task
priority: 1
assignee: Andre Silva
parent: dt-zwgc
tags: [exit-codes, cli]
---
# Exit-code mapping (0/1) + ExitCoder wiring

Centralize exit-code mapping and wire it into `main`: success → 0, failure → non-zero (1). Use `urfave/cli`'s `ExitCoder` so commands return errors and `main` exits with the right code **without** calling `os.Exit` mid-command (deferred cleanup must run).

## Public interface

```go
// internal/output (or a small internal/exit)
const (
    CodeOK        = 0
    CodeError     = 1
    CodeNoOpAssert = 2  // owned by EXIT.assert / dt-4h21
)
func ExitError(err error, code int) cli.ExitCoder   // wrap a command error with a code
```

`main` runs the app; on a returned error it relies on urfave/cli's `HandleExitCoder` (or equivalent) to set the process status. Commands return plain errors (→ 1) or `ExitError(err, code)` for special codes.

## Behaviors (TDD order)

1. **Success → 0** — a command returning `nil` yields exit 0.
2. **Failure → 1** — a command returning a plain error yields exit 1.
3. **`ExitCoder` honored** — a returned `cli.ExitCoder` sets its code.
4. **No `os.Exit` inside commands** — verified by structure: commands return errors; deferred cleanup runs (test a command with a `defer` side-effect on the error path).

## Test strategy

Drive `newApp().Run(ctx, args)` with stub commands returning nil / error / `ExitError`; capture the resolved exit code via the ExitCoder mechanism (avoid actually exiting the test process).

## Acceptance

- 0 success / 1 failure mapping is centralized and used by all commands.
- No command calls `os.Exit`; cleanup always runs.

## References

- Design: [Req 8](../docs/dn-tool-design.md#8-exit-status-semantics).
- Parent epic table (canonical codes): [dt-zwgc](dt-zwgc.md).

Parent epic: [dt-zwgc](dt-zwgc.md).

## Notes

**2026-06-06T19:13:42Z**

Exit-code mapping centralized in internal/output (exit.go). Added: constants CodeOK=0/CodeError=1/CodeNoOpAssert=2 (canonical per dt-zwgc); ExitError(err,code) cli.ExitCoder wrapping that preserves the chain via Unwrap (errors.Is/As cross it); ResolveExitCode(err)int pure mapper (nil->0, ExitCoder->its code, else 1). main.go now delegates failures to exitWithError(), which honors any cli.ExitCoder and otherwise wraps to CodeError, then calls cli.HandleExitCoder (prints msg to stderr + cli.OsExiter) — no os.Exit mid-command, so deferred cleanup runs. Tests: internal/output/exit_test.go (constants, wrap/unwrap, nil->nil-interface, ResolveExitCode table) + cmd/dn-tool/exit_test.go (plain err->1 via OsExiter capture, ExitCoder honored, wrapped ExitCoder, success->nil, deferred-cleanup-on-error). Put it in internal/output (not internal/exit) to match the ticket's primary suggestion and avoid revive stutter (output.ExitError doesn't stutter; exit.ExitError would). NOTE for dt-4h21: use output.ExitError(err, output.CodeNoOpAssert) from the command for the assert-changed no-op. NOTE: urfave/cli auto-exits inside Command.Run when an Action returns a cli.ExitCoder (calls HandleExitCoder->os.Exit there), so override cli.OsExiter in any test whose action returns an ExitCoder; plain errors fall through to main. Unknown-command exit 3 is urfave/cli's own cli.Exit(msg,3) (help.go:319), correctly honored as an ExitCoder.
