---
id: dt-f7nx
status: closed
deps: [dt-2t72, dt-ccmn, dt-icq8]
links: []
created: 2026-06-06T14:14:58Z
type: task
priority: 0
assignee: Andre Silva
parent: dt-3gvq
tags: [unenroll, invariant]
---
# Unenroll failure invariant

Enforce the §2.5 invariant on the failure path: if the remote `DELETE` fails with anything other than success/`404` within the deadline, **retain** the local configuration, report the failure, and exit non-zero. Closes upstream **B3** (unenroll deleted the local config even when the API DELETE failed, creating an unrecoverable orphan).

## Public interface

Extends `Unenroll` (UNE.delete): on a non-2xx/non-404 delete error (incl. deadline exceeded), do **not** remove the local config; return an error and a non-zero exit. The local config and remote record are removed **together** or not at all.

## Behaviors (TDD order)

1. **Delete 5xx → retain local, non-zero exit** — local config dir **still present** after the call; error returned; exit ≠ 0.
2. **Deadline exceeded → retain local** — `ctx` deadline trips during delete → local retained, error surfaced.
3. **Failure message is honest** — states plainly: remote record may persist, local config retained, host remains enrolled and will resume on next boot (no `--force` surprise — no orphan is produced).
4. **Invariant holds across cases** — never a state where local is removed while remote may still exist.

## Test strategy

Reuse the UNE.delete harness with the mock api client returning 5xx / a context that cancels mid-delete. Assert the local dir is intact and the error/exit are correct.

## Acceptance

- Non-404 delete failure retains local config and exits non-zero; invariant never broken; failure message matches §2.5.

## References

- Design: [Req 4](../docs/dn-tool-design.md#4-host-unenrollment), [§2.5 unenroll/shutdown tension](../docs/dn-tool-design.md#25-the-unenroll--shutdown-tension).
- Closes upstream: [**B3**](../docs/research/quickvm-defined-systemd-units-analysis.md#summary-table); deadline counterpart to module **D5** (`TimeoutStopSec`).

Parent epic: [dt-3gvq](dt-3gvq.md).

## Notes

**2026-06-06T19:17:14Z**

Enforced §2.5 unenroll failure invariant (closes upstream B3). The retain-local gating already existed from dt-2t72; this ticket's delta was the honest failure message + the deadline/exit-code tests. Added package const unenrollFailureAdvisory ("remote record may persist, local config retained, host remains enrolled and will resume on next boot") appended to the delete-failure error via fmt.Errorf("...: %w; %s", ...) so the wrapped chain (errors.Is) is preserved. Tests added: DeleteFailureExitsNonZero (asserts output.ResolveExitCode != CodeOK/CodeNoOpAssert — plain error → exit 1), DeadlineExceededRetainsLocal (ctx past deadline; fakeDeleter now honors ctx.Err(); asserts errors.Is(context.DeadlineExceeded) + local dir intact), DeleteFailureMessageIsHonest (substring checks). No production behavior change beyond the message; the local/remote-removed-together invariant is unchanged. Unenroll still returns a plain error, not output.ExitError — main maps it to CodeError(1), which satisfies 'exit != 0'.
