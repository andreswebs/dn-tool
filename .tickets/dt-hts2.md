---
id: dt-hts2
status: closed
deps: [dt-255n, dt-ewgz]
links: []
created: 2026-06-06T14:14:57Z
type: task
priority: 1
assignee: Andre Silva
parent: dt-koaf
tags: [dnclient, install, download]
---
# Resolve dnclient download URL (version vs latest)

Resolve the `dnclient` binary download URL for the detected platform from the downloads API, selecting the configured `DN_CLIENT_VERSION` when set, otherwise the version the API designates as `latest`.

## Public interface

```go
// internal/dnclient
type Resolved struct {
    URL         string // binary URL
    ChecksumURL string // sibling <URL>.sha256 (INST.verify)
    Version     string
}
func ResolveDownload(ctx context.Context, dl *api.Downloads, p Platform, version string) (Resolved, error)
//   version == "latest" -> API-designated latest; else exact match or clear error
```

Takes the downloads object from `api.ListDownloads` ([API.endpoints](dt-255n.md)). The checksum URL is the **sibling `<binary-url>.sha256`**, derived here, not a field in the downloads JSON (API reference §6.3).

## Behaviors (TDD order)

1. **latest selected when version unset/`latest`** — picks the API's latest entry for the platform.
2. **exact version selected when configured** — `DN_CLIENT_VERSION=1.2.3` → that entry.
3. **unknown configured version fails** — no matching entry → clear error naming the version.
4. **platform with no download fails** — clear error.
5. **checksum URL derived** — `Resolved.ChecksumURL == URL + ".sha256"`.

## Test strategy

Construct an `api.Downloads` fixture (mirroring §6.1) and assert `ResolveDownload` returns the right URL/version/checksum URL per case. Pure given the fixture.

## Acceptance

- Configured-version and latest selection both work; unknown version/platform fail clearly; checksum URL is the sibling `.sha256`.

## References

- API reference: [§6.1 Downloads object](../docs/research/defined-net-api-reference.md#61-downloads-object-get-v1downloads), [§6.3 Checksum](../docs/research/defined-net-api-reference.md#63-checksum), [§4.5 List downloads](../docs/research/defined-net-api-reference.md#45-list-software-downloads).
- Design: [Req 1](../docs/dn-tool-design.md#1-dnclient-binary-management).

Parent epic: [dt-koaf](dt-koaf.md).

## Notes

**2026-06-06T20:34:14Z**

Added internal/dnclient/download.go: ResolveDownload(dl *api.Downloads, p Platform, version string) (Resolved, error). All 5 ticket behaviors covered + 1 extra (no-latest-reported). Key decisions: (1) DROPPED the ctx param from the ticket signature — the function is pure (dl is already fetched by api.ListDownloads, zero I/O), so a context arg would be misleading dead weight; consistent with the repo's 'trust the real contract over a toy signature' precedent (dt-4h21/dt-brug). dt-4fgk's caller resolves first, then does the I/O with its own ctx. (2) When version is unset/'latest', resolve to the CONCRETE version string via VersionInfo.Latest.DNClient (not the literal 'latest' map alias) and look the URL up under that key, so Resolved.Version is always concrete — dt-4fgk's skip-if-matches needs a concrete version to compare. Empty latest -> clear error. (3) os-arch key 'linux-amd64' built via an unexported Platform.downloadKey() method (p.OS+'-'+p.Arch); this is the key-construction dt-ewgz deliberately deferred to this ticket — Platform stays a pure data holder. Errors name the offending version (behavior 3) and platform key (behavior 4). Unblocks dt-4fgk (download + SHA-256 verify).
