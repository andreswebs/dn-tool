---
id: dt-ihil
status: closed
deps: [dt-z99h]
links: []
created: 2026-06-06T14:14:57Z
type: task
priority: 0
assignee: Andre Silva
parent: dt-8h9t
tags: [api, http]
---
# API cursor pagination for list endpoints

Implement cursor pagination for the list endpoints so callers receive the full result set. The host-list endpoint has **no `name` filter** (API reference §4.2), so enroll's "remote present?" check must walk every page and match client-side ([ENR.create](dt-pe29.md)). Pagination must be a first-class capability of the client, not something each caller re-implements.

## Public interface

```go
// internal/api
// Internal page-walking helper that the typed list methods build on:
func (c *Client) listAll(ctx context.Context, path string, q url.Values, each func(item json.RawMessage) error) error
//   follows the cursor in the pagination envelope until exhausted
```

Follow the cursor/metadata form documented in API reference §2.4. Expose list methods (in [API.endpoints](dt-255n.md)) that aggregate or stream pages; this task owns the page-walking mechanism.

## Behaviors (TDD order)

1. **Single page returned** — no cursor → one request, all items.
2. **Multiple pages aggregated** — page 1 returns a cursor; page 2 has none → items from both pages, in order.
3. **Cursor threaded correctly** — the second request carries the cursor from page 1 (assert the server saw it).
4. **Empty result** — zero items → no error, empty aggregate.
5. **Error mid-pagination** — a 5xx on page 2 surfaces the error (after retry per API.retry), doesn't silently truncate.

## Test strategy

`httptest.Server` that returns a scripted sequence of pages keyed by the cursor query param. Assert aggregated items and the cursor values the server received.

## Acceptance

- All pages are walked; cursor correctly threaded; partial failure surfaces, never silently truncates.

## References

- API reference: [§2.4 Pagination](../docs/research/defined-net-api-reference.md#24-pagination), [§4.2 List hosts](../docs/research/defined-net-api-reference.md#42-list-hosts) (no name filter).

Parent epic: [dt-8h9t](dt-8h9t.md).

## Notes

**2026-06-06T18:27:04Z**

Implemented listAll(ctx, path, url.Values, each func(json.RawMessage) error) in internal/api/pagination.go — walks cursor pages until metadata.hasNextPage is false or nextCursor is empty, invoking each on every item in order. All 5 TDD behaviors covered in pagination_test.go (single page, multi-page aggregation in order, cursor threading asserted server-side, empty result, 5xx mid-pagination surfaces without silent truncation), plus a filter-preservation test.

Key decisions:
- Refactored client.go: extracted shared request execution (auth, headers, status-check-before-body D4 invariant) into execute(ctx,method,path,body)([]byte,error); do() now builds on it and unwraps {data}. listAll uses execute + decodes {data:[]RawMessage, metadata}. All pre-existing client_test.go assertions still pass (do's status-before-body and out-never-populated-on-failure invariants preserved).
- Defaults pageSize=500 (maxPageSize, the API §2.4 max) when caller omits it, to minimize round-trips for enroll's full-list name match (no name filter exists). Caller filters (e.g. filter.networkID) are copied, never mutated.
- Per-page failure currently surfaces after one attempt (httpClient still http.DefaultClient); dt-egz4 will swap in retryablehttp so 5xx is retried before surfacing. No change needed here — execute already returns the typed error.

Unblocks dt-255n (typed list endpoint methods build on listAll).
