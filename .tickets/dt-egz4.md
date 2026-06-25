---
id: dt-egz4
status: closed
deps: [dt-z99h]
links: []
created: 2026-06-06T14:14:57Z
type: task
priority: 0
assignee: Andre Silva
parent: dt-8h9t
tags: [api, resilience]
---
# API retry/backoff bounded by DN_API_TIMEOUT

Add bounded retry with exponential backoff for transient failures (connection errors, 5xx, 429), bounded by an overall `DN_API_TIMEOUT` deadline. Use `hashicorp/go-retryablehttp` wrapping stdlib `net/http`; its default `CheckRetry`/`Backoff` already encode the Requirement 9 policy.

## Public interface

```go
// internal/api — wire retryablehttp into New()
//   - retry on connection errors, 5xx, 429; tune RetryMax / RetryWaitMin / RetryWaitMax
//   - route its logger through slog (OUT.slog)
//   - client.StandardClient() yields a *http.Client for the typed `do` helper
//   - the overall DN_API_TIMEOUT is enforced via context deadline on each command
```

Backoff/retry decision lives in the transport; the per-command deadline comes from `context.WithTimeout(ctx, cfg.APITimeout)` set by the command, not the client.

## Behaviors (TDD order)

1. **Transient 5xx retried then succeeds** — `httptest` returns 503 twice then 200 → call succeeds; assert request count ≥ 3.
2. **429 retried** — rate-limit response is retried (then succeeds or exhausts).
3. **Connection error retried** — a closed/refused endpoint triggers retry.
4. **Overall deadline bounds retries** — with a short `ctx` deadline, the call returns a deadline error rather than retrying forever; assert it stops near the deadline.
5. **Retry budget exhaustion surfaces the last error** — persistent 503 → returns an error after `RetryMax`.

## Test strategy

`httptest.Server` with a request counter to script status sequences. Use small `RetryWaitMin/Max` in tests to keep them fast. Assert retry counts and timing bounds.

## Acceptance

- Transient failures retried with exponential backoff; bounded by `DN_API_TIMEOUT` (context).
- retryablehttp's logger flows through slog (no stray stderr noise).

## References

- Design: [Req 9](../docs/dn-tool-design.md#9-management-api-resilience), [§2.10 libraries](../docs/dn-tool-design.md#210-cli--libraries) (go-retryablehttp rationale).

Parent epic: [dt-8h9t](dt-8h9t.md).

## Notes

**2026-06-06T18:39:22Z**

Wired hashicorp/go-retryablehttp v0.7.8 into api.New() via unexported newRetryableClient(logger, retryMax, waitMin, waitMax) *retryablehttp.Client: sets CheckRetry=retryPolicy (the dt-4b0e fn, used verbatim), tunes RetryMax/WaitMin/WaitMax (prod defaults 4/1s/30s), and feeds client.StandardClient() into Client.httpClient. execute/do untouched — they just get a retrying *http.Client. Overall deadline is the per-call context (DN_API_TIMEOUT), set by the command layer, NOT the transport. Logger routed through slog via *slogLeveledLogger adapter (Debug/Info/Warn/Error -> slog); New passes slog.Default() so retries obey DN_LOG_LEVEL once dt-zxwf lands; nil logger leaves transport silent (used in tests). go.mod gains go-retryablehttp + indirect go-cleanhttp; go.sum updated (dt-0mdh flake vendorHash will pick these up). Two pre-existing tests had to move to the fast client because retry is now live: TestDoChecksStatusBeforeBody (500) and TestListAllSurfacesErrorMidPagination (5xx mid-page) would otherwise do full prod backoff (~15s); added fastClient(baseURL, retryMax) white-box helper (1ms/2ms waits) and routed them through it — intent preserved (error surfaces, out untouched, no silent truncation). All 4xx tests still assert count==1 (terminal, unaffected).
