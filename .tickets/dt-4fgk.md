---
id: dt-4fgk
status: closed
deps: [dt-hts2]
links: []
created: 2026-06-06T14:14:57Z
type: task
priority: 1
assignee: Andre Silva
parent: dt-koaf
tags: [dnclient, install, checksum]
---
# Download + SHA-256 verify (fail-closed)

Download the `dnclient` binary and verify it against the published sibling `<url>.sha256`. Verification is **mandatory and fail-closed**: never install on a checksum mismatch *or* if the `.sha256` cannot be fetched. Closes upstream **SEC1** (no integrity verification of the downloaded binary).

## Public interface

```go
// internal/dnclient
func DownloadAndVerify(ctx context.Context, httpClient *http.Client, r Resolved, dest io.Writer) error
//   1. GET r.ChecksumURL  -> expected hex digest   (fetch failure => error, NO install)
//   2. GET r.URL, stream into dest while computing sha256
//   3. compare; mismatch => error, NO install (dest must not be treated as valid)
```

There is **no** configured-checksum override (`DN_CLIENT_SHA256` was removed — see [dt-koaf](dt-koaf.md) notes); the published `.sha256` is the only source. Stream the hash while downloading (don't buffer the whole binary). Reuse the resilient HTTP client where sensible.

## Behaviors (TDD order)

1. **Matching checksum → success** — `httptest` serves binary + correct `.sha256`; verify passes and `dest` holds the bytes.
2. **Mismatch → fail closed** — wrong `.sha256` → error; the result must be unusable/not installed (assert caller does not place it).
3. **Checksum fetch failure → fail closed** — `.sha256` returns 404/500 → error, no install.
4. **Binary fetch failure → error** — binary URL 5xx → error (after retry).
5. **Digest computed over the actual stream** — truncated/corrupt body → mismatch.

## Test strategy

`httptest.Server` with routes for the binary and its `.sha256`. Cover match / mismatch / missing-checksum / missing-binary. Assert that on any failure no valid output is produced.

## Acceptance

- Install proceeds only on a verified match; mismatch and fetch-failure both fail closed with clear errors.

## References

- API reference: [§6.3 Checksum](../docs/research/defined-net-api-reference.md#63-checksum).
- Design: [Req 1](../docs/dn-tool-design.md#1-dnclient-binary-management).
- Closes upstream: [**SEC1**](../docs/research/quickvm-defined-systemd-units-analysis.md#summary-table).

Parent epic: [dt-koaf](dt-koaf.md).

## Notes

**2026-06-06T20:37:15Z**

Implemented DownloadAndVerify(ctx, *http.Client, Resolved, io.Writer) in internal/dnclient/verify.go (new file). Fail-closed SHA-256 verification (closes SEC1).

Key decisions:
- Checksum fetched FIRST (fetchChecksum), so a missing/5xx .sha256 aborts before the binary is ever downloaded — the cheapest fail-closed path. dest stays empty in that case (asserted by test).
- Binary streamed into dest via io.TeeReader(resp.Body, sha256.New()) — never buffers the whole binary. On mismatch we return an error; the caller (dt-5o0x install idempotency) must not place dest on error. dest may hold (partial) bytes after a mismatch by design: the contract is 'on error, don't trust dest', and the installer writes to a temp/rename — this fn is just the sink.
- Status checked before reading body for BOTH GETs (same D4 discipline as api.do). 404/500 -> error.
- Checksum body parsed tolerantly: strings.ToLower(TrimSpace(body)); validated as exactly sha256.Size*2 (64) hex chars via hex.DecodeString. §6.3 says single lowercase 64-hex with no trailing data, but real CDN/file artifacts often carry a trailing newline — tolerating whitespace avoids a brittle false mismatch.
- Takes a plain *http.Client param (not api.Client) so the install command injects api's StandardClient() for retry on transient binary 5xx ('after retry' behavior); unit tests pass http.DefaultClient. No api import needed -> no new dep edge.

Tests (verify_test.go): match / mismatch / checksum-fetch-fail (404, asserts dest empty) / binary-fetch-fail (500) / digest-over-truncated-stream / whitespace-tolerance. Uses an httptest mux serving /dnclient + /dnclient.sha256.

Unblocks dt-5o0x (install idempotency: skip/replace) which composes ResolveDownload (dt-hts2) + this.
