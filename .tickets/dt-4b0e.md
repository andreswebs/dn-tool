---
id: dt-4b0e
status: closed
deps: [dt-z99h]
links: []
created: 2026-06-06T14:14:57Z
type: task
priority: 0
assignee: Andre Silva
parent: dt-8h9t
tags: [api, errors]
---
# API error typing: 4xx no-retry + code/path

Define the typed API error and the no-retry rule for client errors: a 4xx (bad input / auth failure) must **not** be retried and must fail with a clear error. The error must carry the defined.net error envelope's `code` and `path` so callers can branch on them — enroll's orphan detection keys on `400 ERR_DUPLICATE_VALUE` at `path: name` ([ENR.create](dt-pe29.md)/[ENR.orphan](dt-xcac.md)).

## Public interface

```go
// internal/api
type APIError struct {
    StatusCode int
    Errors     []APIErrorItem // from {"errors":[{code,message,path}]}
}
func (e *APIError) Error() string
func (e *APIError) Has(code, path string) bool   // e.g. Has("ERR_DUPLICATE_VALUE","name")

type APIErrorItem struct{ Code, Message, Path string }
```

Configure retryablehttp's `CheckRetry` so 4xx are terminal (no retry); parse the error envelope (API reference §2.3) into `APIError` in the `do` helper. Use `errors.As` at call sites to inspect `code`/`path`.

## Behaviors (TDD order)

1. **4xx not retried** — `httptest` 400 → single request, returns `*APIError`.
2. **401/403 not retried** — auth failures are terminal with a clear message.
3. **Error envelope parsed** — `{"errors":[{"code":"ERR_DUPLICATE_VALUE","path":"name"}]}` → `APIError.Has("ERR_DUPLICATE_VALUE","name")` true.
4. **`errors.As` recovers the typed error** — callers can extract `*APIError` from a wrapped error.
5. **Non-JSON 4xx body** — still returns a useful `*APIError` with the status (no panic).

## Test strategy

`httptest.Server` returning scripted 4xx + envelopes; assert request count (==1) and `errors.As(err, &apiErr)` plus `Has(...)`.

## Acceptance

- 4xx never retried; typed `*APIError` carries `code`/`path`.
- Callers can branch on `Has(code, path)` without string-matching messages.

## References

- API reference: [§2.3 Error envelope](../docs/research/defined-net-api-reference.md#23-error-envelope), [§4.1 create host](../docs/research/defined-net-api-reference.md#41-create-host--enrollment-code) (ERR_DUPLICATE_VALUE on `name`).
- Design: [Req 9](../docs/dn-tool-design.md#9-management-api-resilience), [§2.4 state machine](../docs/dn-tool-design.md#24-enrollment-state-machine) (why `code`/`path` matter).

Parent epic: [dt-8h9t](dt-8h9t.md).

## Notes

**2026-06-06T18:13:58Z**

Implemented typed APIError in internal/api/client.go: APIError{StatusCode, Errors []APIErrorItem} with Error() and Has(code,path). The do helper's non-2xx branch now reads the body, parses the {"errors":[{code,message,path}]} envelope, and returns a wrapped *APIError (errors.As recovers it). Non-JSON 4xx bodies still yield a *APIError with the status and empty Errors (no panic). Added retryPolicy(ctx, resp, err) matching retryablehttp.CheckRetry's signature (pure stdlib types) — 4xx terminal, 5xx/429/conn-error retried, cancelled ctx stops with ctx.Err(); dt-egz4 wires it into the retry transport. The D4 ordering invariant is preserved: out is never populated on the failure branch (only apiErr is filled). Tests: envelope parse + Has, 400/401/403/404 terminal with request-count==1, non-JSON body, retryPolicy table + cancelled-context. Unblocks dt-255n, dt-pe29, dt-2t72.
