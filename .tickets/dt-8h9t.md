---
id: dt-8h9t
status: closed
deps: [dt-vsi6, dt-uzx6]
links: []
created: 2026-06-06T11:01:33Z
type: epic
priority: 0
assignee: Andre Silva
tags: [api, http, resilience]
---

# Management API resilience (defined.net REST client)

Requirement 9. Typed defined.net REST client that verifies HTTP status before acting on a body; retries transient errors (network/5xx/429) with bounded exponential backoff up to a configurable overall timeout (DN_API_TIMEOUT); never retries 4xx (bad input / auth failure).

## Design

internal/api on hashicorp/go-retryablehttp wrapping net/http; its default CheckRetry/Backoff encode the policy; honors context deadline from DN_API_TIMEOUT; route logger through slog; client.StandardClient() feeds the typed layer. Tested against httptest.Server: success, 4xx no-retry, 5xx/429 retry, timeout, malformed body.

## Acceptance Criteria

Transient failures retried with backoff within timeout; 4xx not retried and fail clearly; every response's HTTP status checked before body use; all calls bounded by DN_API_TIMEOUT.

## Notes for a fresh agent

- Auth is a bearer token (`Authorization: Bearer $DN_API_KEY`) on `DN_API_URL` (default `https://api.defined.net`) — API reference §2.1.
- Responses use a success envelope (`{ "data": … }`) and an error envelope (`{ "errors": [ { "code", "message", "path" } ] }`) — §2.2 / §2.3. The client should surface `code` and `path` to callers, not just the HTTP status: enroll's orphan detection keys on `ERR_DUPLICATE_VALUE` at `path: name` (§2.4 of the design), so don't flatten errors into a bare string.
- List endpoints are cursor-paginated (§2.4) — the client must expose pagination so enroll's list-and-match can walk all pages.
- v1/v2 coexist (§3): create/get/list hosts are **v2**; delete host is **v1 only**. The client should not assume one version prefix.

## References

- Design: [Req 9 Management API resilience](../docs/dn-tool-design.md#9-management-api-resilience), [§2.9 Internal structure](../docs/dn-tool-design.md#29-internal-structure-tentative), [§2.10 CLI / libraries](../docs/dn-tool-design.md#210-cli--libraries) (`hashicorp/go-retryablehttp`), [§2.11 Testing](../docs/dn-tool-design.md#211-testing).
- API reference: [§2.1 Auth](../docs/research/defined-net-api-reference.md#21-base-url-and-authentication), [§2.2 Response envelope](../docs/research/defined-net-api-reference.md#22-response-envelope), [§2.3 Error envelope](../docs/research/defined-net-api-reference.md#23-error-envelope), [§2.4 Pagination](../docs/research/defined-net-api-reference.md#24-pagination), [§3 API versioning](../docs/research/defined-net-api-reference.md#3-api-versioning).
- Research: upstream finding [D4](../docs/research/quickvm-defined-systemd-units-analysis.md#summary-table) — no HTTP status checking (errors silently consumed by `jq`).

## Notes

**2026-06-06T11:04:58Z**

Fixes upstream D4 (no curl had HTTP status checking; API errors were silently consumed by jq) -> verify HTTP status before reading any body. Underpins the B3 fix: the unenroll DELETE must check status before mutating local state.

**2026-06-06T20:09:56Z**

Verified complete; closed with no code change. All 5 children closed (egz4 retry/backoff, z99h client core + status-before-body D4, 255n endpoints, ihil pagination, 4b0e error typing). Re-read epic acceptance against reality (the dt-uzx6/dt-vsi6 discipline): all 4 bullets met & tested, the named httptest matrix (success/4xx-no-retry/5xx+429-retry/timeout/malformed-body) all pass, public New(cfg) exercised by ~24 tests, code/path surfaced via APIError.Has, pagination exposed via ListHosts, no version-prefix assumption (full paths). UNLIKE dt-uzx6 there is NO unowned residual seam: the only acceptance phrase that looks open -- 'all calls bounded by DN_API_TIMEOUT' -- is satisfied at the client level (honors any ctx deadline; TestOverallDeadlineBoundsRetries proves retries are bounded). Binding config.APITimeout -> call ctx is per-command consumption (defaults differ: ~30s enroll / ~10s unenroll, config.go leaves it 0 'command picks default', unenroll.go already assumes 'bounded at command layer'), so it belongs to the downstream blocked tickets dt-3gvq/dt-i0yx/dt-koaf, NOT the epic. Building it here would pre-empt them (the dt-toqi/dt-j2ab scope-by-omission rule). Closing unblocks those 3 command tickets.
