---
id: dt-zwgc
status: closed
deps: [dt-vsi6]
links: []
created: 2026-06-06T11:01:33Z
type: epic
priority: 1
assignee: Andre Silva
tags: [exit-codes, cli]
---

# Exit status semantics

Requirement 8. Exit 0 on success; non-zero on failure; a distinct non-error, non-zero status (exit 2) when --assert-changed is set and a command made no change.

## Design

Centralized exit-code mapping used by all commands. The no-op/changed signal flows from each command's result (ties into Output epic).

## Acceptance Criteria

Success=0; failure=non-zero; --assert-changed with a no-op yields the distinct change-assertion status (2).

## Notes for a fresh agent

Canonical mapping all commands share:

| Code | Meaning                                                           |
| ---- | ----------------------------------------------------------------- |
| 0    | Success (a change was made, or no-op without `--assert-changed`). |
| 1    | Failure.                                                          |
| 2    | No-op while `--assert-changed` is set (distinct, non-error).      |

- `2` is reserved for the assert-changed no-op only — never reuse it for failures.
- The "made no change" signal flows from each command's result object (the `changed` flag owned by [dt-cq78](dt-cq78.md)); this epic only maps that flag + error to the process exit code.
- With `urfave/cli`, return a `cli.ExitCoder` (e.g. `cli.Exit(msg, code)`) rather than calling `os.Exit` mid-command, so deferred cleanup still runs.

## References

- Design: [Req 8 Exit status semantics](../docs/dn-tool-design.md#8-exit-status-semantics), [§2.4 Enrollment state machine](../docs/dn-tool-design.md#24-enrollment-state-machine) (no-op → exit 2).

**2026-06-06T21:18:47Z**

Verify-and-close: both children closed (dt-icq8 exit-code mapping, dt-4h21 --assert-changed). All three Req 8 acceptance bullets covered by green tests: success=0 (TestRun_SuccessCommandReturnsNil, TestAssertChanged_ChangeExitsZero/NoOpWithoutFlagExitsZero, TestResolveExitCode nil->0); failure=non-zero (TestExitWithError_PlainErrorMapsToCodeError, TestAssertChanged_FailureExitsOneWithFlag); exit-2 assert-changed no-op (TestAssertChanged_NoOpExitsTwo, TestExitError_WrapsCodeAndMessage). Mapping centralized in internal/output (exit.go: CodeOK/CodeError/CodeNoOpAssert, ExitError, ResolveExitCode) and wired in main (exitWithError + withResult + Before hook). Zero code change to close — the withResult seam is correctly deferred to each command ticket. make build green. Unblocks P0 command tickets.
