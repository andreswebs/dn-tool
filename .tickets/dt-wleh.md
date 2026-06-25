---
id: dt-wleh
status: closed
deps: [dt-854m, dt-pe29]
links: []
created: 2026-06-06T14:14:58Z
type: task
priority: 2
assignee: Andre Silva
parent: dt-hxvr
tags: [enroll, lighthouse, relay, api]
---
# Lighthouse/relay field mapping

Map the lighthouse/relay configuration onto the create-host request: the lighthouse role + its static addresses; the relay role; and the listen port (use the configured port, else request a system-selected port). Extends [ENR.request](dt-j2ab.md)'s request builder and the [ENR.create](dt-pe29.md) enroll path.

## Public interface

Extends `buildCreateRequest` (dt-j2ab) to populate the role/lighthouse/relay/static-address/listen-port fields of `api.CreateHostRequest`. Field names per API reference §4.1 / §5 — do not guess; confirm against the host object.

## Behaviors (TDD order)

1. **Lighthouse request** — `IsLighthouse` → request carries the lighthouse role + `StaticAddrs` + listen port.
2. **Relay request** — `IsRelay` → request carries the relay role + listen port.
3. **Configured port used** — non-zero `ListenPort` → that port in the request.
4. **No port → system-selected** — for a plain (non-lighthouse/relay) host with `ListenPort==0`, request the auto/system-selected port (0/omitted per API semantics).
5. **Plain host unaffected** — neither role set → no lighthouse/relay fields added (regression guard on ENR.request).

## Test strategy

Table-driven `(Config) -> CreateHostRequest`, asserting the role and port/static-address fields. Validation (mutual exclusion etc.) is already guaranteed by [LH.gates](dt-854m.md), so these tests assume valid input.

## Acceptance

- Lighthouse enrolls with role + static addresses; relay with relay role; configured port used else auto; plain host path unchanged.

## References

- Design: [Req 3](../docs/dn-tool-design.md#3-lighthouse-and-relay-enrollment), [§2.3 Configuration variables](../docs/dn-tool-design.md#23-configuration-variables).
- API reference: [§4.1 Create host & code](../docs/research/defined-net-api-reference.md#41-create-host--enrollment-code), [§5 Host object](../docs/research/defined-net-api-reference.md#5-host-object).

Parent epic: [dt-hxvr](dt-hxvr.md).

## Notes

**2026-06-06T22:25:25Z**

Mapped lighthouse/relay fields onto buildCreateRequest (internal/enroll/enroll.go): IsLighthouse -> req.IsLighthouse + req.StaticAddresses=cfg.StaticAddrs; IsRelay -> req.IsRelay; ListenPort passed through verbatim (0=auto-select, non-zero=configured). Static addresses set ONLY under lighthouse, not relay/plain. Plain-host path unchanged. Validation (validateRoles, dt-854m) already guarantees valid input, so field-mapping tests assume validity. New fields_test.go: table-driven RoleFields (lighthouse/relay/configured-port/auto-port) + PlainHostHasNoRoleFields regression guard (StaticAddrs in config but neither role -> no role fields, nil StaticAddresses). CreateHostRequest struct (api/endpoints.go) already had all fields from dt-j2ab. Closes epic dt-hxvr's last open child -> dt-hxvr now verify-and-close ready. make build green.
