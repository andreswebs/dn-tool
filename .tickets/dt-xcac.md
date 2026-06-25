---
id: dt-xcac
status: closed
deps: [dt-pe29]
links: []
created: 2026-06-06T14:14:58Z
type: task
priority: 0
assignee: Andre Silva
parent: dt-i0yx
tags: [enroll, state-machine]
---
# Enroll orphan/force cell

Implement the orphan cells of the §2.4 state machine: **local absent + remote present**. By default this is an error with operator guidance (re-run with `--force`); with `--force` set, delete the stale remote record by `id` and enroll afresh. The default-fail is deliberate (Q8 → C): silently deleting a remote record could disrupt a host legitimately enrolled under the same name elsewhere.

## Public interface

Extends `Enroll` (dt-pe29). The list-and-match lookup already yields the existing host's `id`; the `--force` path calls `api.DeleteHost(ctx, id)` then the create→code→enroll flow.

## Behaviors (TDD order)

1. **Orphan without `--force` → fail w/ guidance** — local absent, remote match found → error instructing re-run with `--force`; no delete, no enroll, `Changed=false`.
2. **`--force` → delete then re-enroll** — deletes the matched `id`, then create→code→`dnclient enroll`; `Changed=true`.
3. **Delete fails under `--force`** — `DeleteHost` error → abort before create/enroll, surface it.
4. **Force on absent/absent is harmless** — `--force` with no remote present behaves like the plain create path (no spurious delete).

## Test strategy

Mock api client returning a matching host on `ListHosts`; assert the no-force error text mentions `--force`, and that the force path calls `DeleteHost(id)` before `CreateHostAndEnrollmentCode`. Mock `dnclient.Client` for the enroll leg.

## Acceptance

- Orphan fails by default with clear `--force` guidance; `--force` deletes by id then re-enrolls; delete failure aborts cleanly.

## References

- Design: [Req 2](../docs/dn-tool-design.md#2-host-enrollment), [§2.4 state machine](../docs/dn-tool-design.md#24-enrollment-state-machine) (rows 3–4), [§2.5](../docs/dn-tool-design.md#25-the-unenroll--shutdown-tension) (orphan only arises from a failed enroll, given the unenroll invariant).
- API reference: [§4.4 Delete host](../docs/research/defined-net-api-reference.md#44-delete-host) (v1).

Parent epic: [dt-i0yx](dt-i0yx.md).

## Notes

**2026-06-06T21:55:20Z**

Implemented the §2.4 orphan/force cell (local absent + remote present) in internal/enroll/state.go. Added Force bool to enroll.Deps (the --force opt-in; an option, not config/collaborator, but Deps keeps Enroll's signature stable). The list-and-match loop now: on a name match with !Force returns a guidance error naming the host and id and instructing --force (no delete, no create, no enroll, Changed=false); with Force calls api.DeleteHost(ctx, h.ID) then falls through to the existing create→code→enroll path. Delete failure wraps with %w and aborts before create/enroll (errors.As crosses to *api.APIError). Force on absent/absent never deletes (the match loop just doesn't fire). Removed errOrphanUnimplemented placeholder and the now-unused errors import. Tests in orphan_test.go (4 behaviors); extended scriptedAPI in create_test.go with guarded delete recording (allowDelete + deleteErr + deletedID + createSawDelete) so create-cell 'must not delete' guards stay intact while ordering (delete-before-create) is asserted. make build green.
