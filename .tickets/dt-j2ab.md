---
id: dt-j2ab
status: closed
deps: [dt-toqi, dt-255n]
links: []
created: 2026-06-06T14:14:57Z
type: task
priority: 0
assignee: Andre Silva
parent: dt-i0yx
tags: [enroll, api]
---
# Enroll request building + required-param validation

Build the create-host request from config and validate the required enrollment parameters. Required: API key, network ID, role ID — a missing/empty one fails with a clear error **naming the parameter**. Resolve the enrollment hostname, request the tun device name = network name, and include the optional static IP and tags.

## Public interface

```go
// internal/enroll
func buildCreateRequest(cfg *config.Config) (api.CreateHostRequest, error)
//   validates required fields; returns a clear error naming the first missing one
```

Maps `cfg` → `api.CreateHostRequest` ([API.endpoints](dt-255n.md)): `name` = `cfg.Hostname` (already defaulted to system hostname in CFG.load), `networkID`, `roleID`, tun/network name = `cfg.NetworkName`, optional `ipAddress`, `tags`. Lighthouse/relay/static-address/port fields are added by [LH.fields](dt-wleh.md); this task covers the common host path.

## Behaviors (TDD order)

1. **Missing API key fails, names it** — empty `DN_API_KEY` → error mentioning the API key.
2. **Missing network ID / role ID fail, named** — each absent required field → specific error.
3. **Hostname resolution** — uses `cfg.Hostname` (configured or system default).
4. **tun device = network name** — request asks for the tun/device matching `cfg.NetworkName`.
5. **Optional static IP and tags included** — when set, present in the request; when unset, omitted.

## Test strategy

Table-driven `(Config) -> (CreateHostRequest | error)`. Pure; no network. Assert the error names the specific missing parameter.

## Acceptance

- All required-param omissions fail clearly by name; hostname/tun/IP/tags mapped correctly into the request.

## References

- Design: [Req 2](../docs/dn-tool-design.md#2-host-enrollment), [§2.4 state machine](../docs/dn-tool-design.md#24-enrollment-state-machine).
- API reference: [§4.1 Create host & code](../docs/research/defined-net-api-reference.md#41-create-host--enrollment-code), [§5 Host object](../docs/research/defined-net-api-reference.md#5-host-object).

Parent epic: [dt-i0yx](dt-i0yx.md).

## Notes

**2026-06-06T18:53:28Z**

Created internal/enroll package with buildCreateRequest(cfg) (api.CreateHostRequest, error). Validates required params in order API key->network ID->role ID, naming the first missing one by DN_* var name. Maps Name=cfg.Hostname, NetworkID, RoleID, Tags; DN_IP_ADDRESS -> single IPAddresses entry (v2 shape). KEY DECISION: no tun/network-name field in the body (per dt-255n) — tun device naming is a dnclient-invocation concern, locked by TestBuildCreateRequestOmitsTunField. Lighthouse/relay/static/port deliberately NOT set here (listenPort stays 0=auto); dt-wleh layers those onto buildCreateRequest. buildCreateRequest is unexported, exercised only by same-package tests (not flagged unused, per dt-z99h pattern). make build green.
