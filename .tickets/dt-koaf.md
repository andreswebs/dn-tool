---
id: dt-koaf
status: closed
deps: [dt-uzx6, dt-8h9t, dt-cq78, dt-zwgc]
links: []
created: 2026-06-06T11:01:33Z
type: epic
priority: 1
assignee: Andre Silva
tags: [dnclient, install, download]
---
# dnclient binary management (install)

Requirement 1 / install command. Download the dnclient binary from the URL advertised by the defined.net downloads API for the host OS+arch into a configurable dir (default /var/lib/defined/bin); install configured version or API 'latest'; verify SHA-256 against the published sibling `<binary-url>.sha256` (mandatory/fail-closed — not installed if the checksum can't be fetched or doesn't match); skip when an existing binary matches; re-download on mismatch; fail clearly on non-Linux OS or unmappable arch or checksum failure.

## Design

internal/dnclient handles download/verify/install. Verification source is the published sibling `<binary-url>.sha256` only (the downloads API JSON carries no checksum); there is no configured-checksum override (DN_CLIENT_SHA256 removed). Verification is mandatory/fail-closed: never install on checksum mismatch or if the .sha256 can't be fetched. Idempotent: existing binary matching version+checksum => skip.

## Acceptance Criteria

Correct OS/arch binary downloaded and verified against the published .sha256; version selection (configured vs latest); checksum cases (match / mismatch / fetch-failure) behave with mismatch and fetch-failure both failing closed (no install); skip-on-match and re-download-on-mismatch; clear errors for non-Linux, unknown arch, checksum failure.

## Notes for a fresh agent

- The download URL comes from `GET /v1/downloads` (API reference §4.5 / §6.1); pick the entry matching the host OS/arch. The OS/arch key mapping (Go `GOARCH` amd64/arm64 → dnclient download keys) is §6.2 — do not invent it.
- The checksum is a **sibling `<binary-url>.sha256` file**, not a field in the downloads JSON (§6.3). Fetch it, compare, and fail closed if it can't be fetched or doesn't match.
- Linux-only by requirement, but the binary builds/runs on darwin for dev — gate the OS check at the `install` boundary, not at startup, so non-install commands stay testable on macOS.

## References

- Design: [Req 1 dnclient binary management](../docs/dn-tool-design.md#1-dnclient-binary-management), [§2.2 Command surface](../docs/dn-tool-design.md#22-command-surface), [§2.3 Configuration variables](../docs/dn-tool-design.md#23-configuration-variables), [§2.12 step 2](../docs/dn-tool-design.md#212-build--migration-order).
- API reference: [§4.5 List software downloads](../docs/research/defined-net-api-reference.md#45-list-software-downloads), [§6.1 Downloads object](../docs/research/defined-net-api-reference.md#61-downloads-object-get-v1downloads), [§6.2 OS/architecture keys](../docs/research/defined-net-api-reference.md#62-osarchitecture-keys), [§6.3 Checksum](../docs/research/defined-net-api-reference.md#63-checksum).
- Research: upstream findings [SEC1 / S4 / D1 / D2](../docs/research/quickvm-defined-systemd-units-analysis.md#summary-table) — no integrity check, `arch` vs `uname -m`, dead code after `mkdir`, `envsubst` over all env vars.

## Notes

**2026-06-06T11:04:58Z**

Fixes upstream findings (quickvm-defined-systemd-units.md): SEC1 (no checksum verification of downloaded binary -> SHA-256 verify, configured + API-advertised), S4 (use uname -m, not arch, for the OS/arch -> dnclient download-string mapping).

**2026-06-06T13:01:45Z**

Scope change: removed the DN_CLIENT_SHA256 env var / configured-checksum override. Research (API reference §6.3) confirmed the downloads service publishes a per-binary sibling <url>.sha256 (verified to match the binary), so a user-supplied checksum is redundant. Verification is now solely against the published .sha256 and is mandatory/fail-closed. The SEC1 fix above still holds; ignore its 'configured + API-advertised' wording (now published-only).

**2026-06-06T21:36:52Z**

Verify-and-close: all 5 children closed and every acceptance bullet met + tested. Correct OS/arch binary: DetectPlatform (dt-ewgz, linux/amd64+arm64, fails clearly otherwise), gated at the install command boundary. Resolve URL + version selection (configured vs API latest): ResolveDownload (dt-hts2). Download + SHA-256 verify against published sibling .sha256, fail-closed on mismatch/fetch-failure: DownloadAndVerify (dt-4fgk, httptest matrix). Skip-on-match / re-download-on-mismatch: NeedsInstall (dt-5o0x). Placement at DN_CLIENT_BIN_DIR (default /var/lib/defined/bin) with atomic temp+fsync+rename + result/exit wiring, dir created as needed: Install/placeBinary + installAction wired with withResult (dt-svmu). Clear errors for non-Linux/unknown-arch (DetectPlatform) and checksum failure (DownloadAndVerify). make build green throughout; install command smoke-tested end-to-end. Unblocks dt-flal (run) and dt-a772 (Enroller).
