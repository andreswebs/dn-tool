---
id: dt-z99h
status: closed
deps: [dt-mhir]
links: []
created: 2026-06-06T14:14:57Z
type: task
priority: 0
assignee: Andre Silva
parent: dt-8h9t
tags: [api, http]
---
# API client core: auth + status-checked requests

Build the core of the typed defined.net REST client: construction from `Config`, bearer authentication, and a request/decode helper that **verifies the HTTP status before reading the body**. This is the foundation the retry, error-typing, pagination, and endpoint tasks extend. Closes upstream **D4** (no HTTP status checking; API errors were silently consumed by `jq`).

## Public interface

```go
// internal/api
type Client struct { /* baseURL, apiKey, httpClient */ }

func New(cfg *config.Config) *Client   // baseURL=cfg.APIURL, auth=cfg.APIKey

// internal helper used by all endpoint methods:
func (c *Client) do(ctx context.Context, method, path string, body any, out any) error
//   - sets Authorization: Bearer <apiKey>, Content-Type, Accept
//   - checks resp.StatusCode BEFORE decoding; non-2xx -> error (typed in API.errors)
//   - unwraps the {"data": …} success envelope into out
```

`ctx` is the first parameter on every method (never stored on the struct). Auth via `Authorization: Bearer <DN_API_KEY>` against `cfg.APIURL` (default `https://api.defined.net`), API reference §2.1.

## Behaviors (TDD order)

1. **2xx success decodes data** — `httptest` returns `{"data":{…}}`; `do` unwraps into `out`.
2. **Bearer header set** — server asserts `Authorization: Bearer <key>` present.
3. **Status checked before body** — a 500 with a junk body returns an error, does **not** populate `out` (D4).
4. **Context cancellation propagates** — a cancelled `ctx` aborts the request.

## Test strategy

`httptest.Server` returning canned responses; construct `New` pointed at the test server URL. Assert on returned `(out, err)` and on the request the server received (headers/method/path). No real network.

## Acceptance

- Every response's status is verified before its body is used.
- Auth header present on all requests; success envelope unwrapped.
- `ctx` honored as the first parameter throughout.

## References

- Design: [Req 9](../docs/dn-tool-design.md#9-management-api-resilience), [§2.10 libraries](../docs/dn-tool-design.md#210-cli--libraries).
- API reference: [§2.1 Auth](../docs/research/defined-net-api-reference.md#21-base-url-and-authentication), [§2.2 Response envelope](../docs/research/defined-net-api-reference.md#22-response-envelope).
- Closes upstream: [**D4**](../docs/research/quickvm-defined-systemd-units-analysis.md#summary-table).

Parent epic: [dt-8h9t](dt-8h9t.md).

## Notes

**2026-06-06T18:10:05Z**

API client core implemented in internal/api/client.go (TDD). Client{baseURL,apiKey,httpClient}; New(*config.Config) wires APIURL+APIKey, uses http.DefaultClient. Unexported do(ctx,method,path,body,out) sets Bearer auth + Accept, Content-Type only when a body is sent, marshals body via json, checks resp.StatusCode (2xx) BEFORE decoding (closes D4), unwraps {"data":...} envelope into out, and supports nil body / nil out. ctx is first param, never stored. Non-2xx returns a plain 'unexpected status N' error for now — proper typed errors (code/path, 4xx no-retry) are dt-4b0e's job, and retry/backoff is dt-egz4; both extend do. Tests use httptest covering all 4 behaviors + the request-body/Content-Type path. golangci-lint clean (do is exercised by same-package tests so not flagged unused).
