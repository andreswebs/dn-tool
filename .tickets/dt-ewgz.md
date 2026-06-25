---
id: dt-ewgz
status: closed
deps: [dt-mhir]
links: []
created: 2026-06-06T14:14:57Z
type: task
priority: 1
assignee: Andre Silva
parent: dt-koaf
tags: [dnclient, install, arch]
---
# OS/arch detection + dnclient key mapping

Detect the host OS and CPU architecture and map them to the `dnclient` download keys the downloads API uses. Fail clearly on non-Linux OS and on an unmappable architecture. Closes upstream **S4** (the bash script used `arch`, not `uname -m`, mis-detecting on some hosts).

## Public interface

```go
// internal/dnclient
type Platform struct{ OS, Arch string } // dnclient download keys, e.g. {"linux","amd64"}
func DetectPlatform(goos, goarch string) (Platform, error) // inject runtime.GOOS/GOARCH for tests
```

Inject `goos`/`goarch` (don't read `runtime.*` directly in the mapper) so the table is fully testable. Use the OS/arch key table in API reference §6.2 as the source of truth — do not invent keys.

## Behaviors (TDD order)

1. **linux/amd64 maps** — `("linux","amd64")` → the documented dnclient key pair.
2. **linux/arm64 maps** — `("linux","arm64")` → documented keys.
3. **non-Linux fails** — `("darwin",…)` → clear error (Requirement 1: Linux-only). Gate this at the install boundary so non-install commands still run on macOS for dev.
4. **unknown arch fails** — `("linux","mips")` → clear error naming the arch.

## Test strategy

Table-driven `(goos,goarch) -> (Platform | error)`. Pure function; no environment access.

## Acceptance

- Supported linux/{amd64,arm64} map to correct keys; non-Linux and unknown arch fail with clear, specific errors.

## References

- Design: [Req 1](../docs/dn-tool-design.md#1-dnclient-binary-management).
- API reference: [§6.2 OS/architecture keys](../docs/research/defined-net-api-reference.md#62-osarchitecture-keys).
- Closes upstream: [**S4**](../docs/research/quickvm-defined-systemd-units-analysis.md#summary-table).

Parent epic: [dt-koaf](dt-koaf.md).

## Notes

**2026-06-06T19:24:22Z**

Added internal/dnclient/platform.go: Platform{OS,Arch} + DetectPlatform(goos,goarch) (injected, table-testable). Supports ONLY linux/amd64 + linux/arm64 (design §2.1 production targets); every other arch — including linux/mips which the downloads API actually publishes (API ref §6.2) — fails by deliberate scope decision, matching behavior 4 and acceptance. Checks OS first (non-linux → error naming the OS), then arch (unmappable → error naming the arch). Closes upstream S4. Scope held tight: did NOT add a DownloadKey()/URL-path helper — the linux-amd64 key + linux/amd64 path construction belongs to dt-hts2 (download URL resolution), the ticket I now unblock. Platform.Arch == Go arch for both supported targets, so no translation table needed yet; dt-hts2 owns the armv5/6/7 key-vs-path divergence if those are ever added.
