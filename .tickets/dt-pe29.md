---
id: dt-pe29
status: closed
deps: [dt-j2ab, dt-a772, dt-4b0e]
links: []
created: 2026-06-06T14:14:57Z
type: task
priority: 0
assignee: Andre Silva
parent: dt-i0yx
tags: [enroll, state-machine]
---
# Enroll create cell: local+remote absent

Implement the happy-path cell of the §2.4 state machine: **local config absent + no remote record** → create the remote host record, obtain the single-use enrollment code, and run `dnclient enroll`. Also enforce: any management-API error → fail **without** running `dnclient enroll`; `dnclient enroll` non-zero → fail and surface it. The "remote absent?" check uses list-and-match (no name filter). Closes upstream **B4** (orphan when the record is created but enroll then fails) and **D7** (non-atomic state).

## Public interface

Extends `Enroll` (dt-brug). Remote-presence check via `api.ListHosts(ctx, networkID)` + client-side `name` match (API reference §4.2 — no name filter). On absent: `api.CreateHostAndEnrollmentCode` → `dnclient.Client.Enroll(ctx, code)`.

## Behaviors (TDD order)

1. **Absent/absent → create→code→enroll** — local absent, no matching remote host → create record, get code, call `dnclient enroll` with it; `Result.Changed=true`, includes `hostId`.
2. **List-and-match finds no name** — `ListHosts` returns hosts but none match → treated as absent (proceeds to create).
3. **API error aborts before enroll** — `CreateHostAndEnrollmentCode` returns `*APIError` → `Enroll` fails and the mock `dnclient.Client` is **never** called (the B4 guard).
4. **`dnclient enroll` non-zero surfaced** — record created, mock enroll returns error → `Enroll` fails surfacing it. (Note: this is the narrow state that produces the §2.5 enroll-path orphan, recovered via `--force` in [ENR.orphan](dt-xcac.md).)
5. **Code never logged** — the obtained code is passed in-memory to enroll only.

## Test strategy

Mock api client (scripted `ListHosts`/`CreateHostAndEnrollmentCode`) + mock `dnclient.Client` recording calls. Assert call ordering, that enroll is skipped on API error, and `Result` fields.

## Acceptance

- Clean create→code→enroll on absent/absent; API error aborts before subprocess; subprocess failure surfaced; changed/hostId in result.

## References

- Design: [Req 2](../docs/dn-tool-design.md#2-host-enrollment), [§2.4 state machine](../docs/dn-tool-design.md#24-enrollment-state-machine) (row 2), [§2.5 unenroll/shutdown tension](../docs/dn-tool-design.md#25-the-unenroll--shutdown-tension) (how the orphan arises).
- API reference: [§4.1 Create host & code](../docs/research/defined-net-api-reference.md#41-create-host--enrollment-code), [§4.2 List hosts](../docs/research/defined-net-api-reference.md#42-list-hosts).
- Closes upstream: [**B4 / D7**](../docs/research/quickvm-defined-systemd-units-analysis.md#summary-table).

Parent epic: [dt-i0yx](dt-i0yx.md).

## Notes

**2026-06-06T21:51:24Z**

Implemented the §2.4 create cell (local absent + remote absent) in internal/enroll/state.go. Flow: buildCreateRequest (required-param validation) -> API.ListHosts + client-side name match (no name filter, ref §4.2) -> API.CreateHostAndEnrollmentCode -> DNClient.Enroll(networkName, code.Reveal()). Added DNClient dnclient.Client field to Deps (the field dt-a772/dt-brug deferred to 'the cell that runs enroll'). Guards: list failure or create *APIError aborts before the subprocess (B4 guard, errors via %w so errors.As crosses); dnclient enroll error surfaced verbatim (errors.Is). Remote-present (orphan) branch returns errOrphanUnimplemented placeholder — owned by dt-xcac (orphan/force cell). Result{Action:enroll,Changed:true,HostID,Network}. Code revealed only at the hand-off, never in Result/logs (SEC5). Tests in create_test.go cover all 5 ticket behaviors + a list-error abort case.
