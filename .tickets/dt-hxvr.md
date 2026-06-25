---
id: dt-hxvr
status: closed
deps: [dt-i0yx]
links: []
created: 2026-06-06T11:01:33Z
type: epic
priority: 2
assignee: Andre Silva
tags: [enroll, lighthouse, relay]
---

# Lighthouse & relay enrollment

Requirement 3. Enroll a host as a lighthouse (lighthouse role + configured static addresses) or relay (relay role). Mutually exclusive: both set => fail. Lighthouse requires >=1 static address. Lighthouse/relay require a non-zero listen port. With a configured port enroll with it, else request a system-selected port.

## Design

Extends the enroll state machine with role/port/static-address validation gates evaluated before any remote calls.

## Acceptance Criteria

Lighthouse enrolls with role + static addresses; relay enrolls with relay role; both-set fails; lighthouse without static address fails; lighthouse/relay without non-zero port fails; configured port used else auto port requested.

## Notes for a fresh agent

- This extends the [dt-i0yx](dt-i0yx.md) state machine — validation gates (mutual exclusion, lighthouse needs ≥1 static address, lighthouse/relay need a non-zero port) must run **before** any remote API call, so a misconfigured host fails fast and creates no record.
- The lighthouse/relay/static-address/listen-port fields map onto the create-host request body — see the field list in API reference §4.1 and the host object in §5; don't guess field names.
- Config inputs are `DN_IS_LIGHTHOUSE`, `DN_IS_RELAY`, `DN_STATIC_ADDRESSES` (JSON array), `DN_LISTEN_PORT` (design §2.3), already parsed by [dt-uzx6](dt-uzx6.md).

## References

- Design: [Req 3 Lighthouse and relay enrollment](../docs/dn-tool-design.md#3-lighthouse-and-relay-enrollment), [§2.3 Configuration variables](../docs/dn-tool-design.md#23-configuration-variables).
- API reference: [§4.1 Create host & enrollment code](../docs/research/defined-net-api-reference.md#41-create-host--enrollment-code), [§5 Host object](../docs/research/defined-net-api-reference.md#5-host-object).

**2026-06-06T22:27:06Z**

Verify-and-close: both children (dt-854m validation gates, dt-wleh field mapping) were already closed and the epic's acceptance is fully met. Confirmed against code + tests before closing:

- validateRoles (internal/enroll/validate.go) runs at the TOP of Enroll, before ConfigExists and any remote call, with fixed check order: mutual-exclusion -> lighthouse-needs-static-addr -> (lighthouse||relay)-needs-non-zero-port. Plain host (neither flag) short-circuits to nil.
- buildCreateRequest (enroll.go) maps role fields: IsLighthouse sets req.IsLighthouse + req.StaticAddresses; IsRelay sets req.IsRelay; ListenPort is passed through UNCONDITIONALLY (0 == API auto-select), so 'configured port used else auto' collapses to one assignment.
- Tests: TestValidateRoles covers all 7 acceptance cases (both-set, lighthouse-no-addr, lighthouse-no-port, relay-no-port, valid lighthouse, valid relay, plain). TestEnrollValidatesRolesBeforeRemoteCalls proves gates fire before API traffic (failingAPI mock + empty config root). TestBuildCreateRequestRoleFields + PlainHostHasNoRoleFields lock the mapping and prove role fields never bleed into the common path.

No code change needed. make build green (all packages pass).
