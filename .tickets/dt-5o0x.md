---
id: dt-5o0x
status: closed
deps: [dt-4fgk]
links: []
created: 2026-06-06T14:14:57Z
type: task
priority: 1
assignee: Andre Silva
parent: dt-koaf
tags: [dnclient, install, download]
---
# Install idempotency: skip/replace

Make `install` idempotent: when a binary already exists at the target path and matches the expected version and checksum, **skip** the download; when it exists but does not match, **re-download and replace** it. This is what lets boot-time `install` be a cheap no-op on a healthy host.

## Public interface

```go
// internal/dnclient
func NeedsInstall(ctx context.Context, path string, want Resolved) (need bool, reason string, err error)
//   false  -> existing file matches want.Version + checksum (skip; changed=false)
//   true   -> missing, or version/checksum mismatch (re-download; changed=true)
```

Compute the on-disk binary's sha256 and compare to the expected digest from `want.ChecksumURL` (already fetched in INST.verify). "Matches version" can be the checksum identity (a given version has a fixed digest); document the chosen check.

## Behaviors (TDD order)

1. **Missing file → need install** — no file at path → `true`.
2. **Existing match → skip** — file present with the expected digest → `false`, `changed=false`.
3. **Existing mismatch → replace** — file present with a different digest → `true` (re-download).
4. **Unreadable/locked target → clear error** — surface, don't silently skip.

## Test strategy

Use a temp dir (`t.TempDir()`): write fixture binaries with known digests; assert `NeedsInstall` decisions. No network — the expected digest is passed in.

## Acceptance

- Matching binary is skipped (no write); mismatch triggers replacement; missing triggers install. The `changed` outcome feeds the result/exit layer.

## References

- Design: [Req 1](../docs/dn-tool-design.md#1-dnclient-binary-management) ("WHEN a binary already exists … and matches … skip … otherwise re-download").

Parent epic: [dt-koaf](dt-koaf.md).

## Notes

**2026-06-06T21:30:00Z**

Added internal/dnclient/install.go: NeedsInstall(path, expectedDigest string) (need bool, reason string, err error) + streaming fileSHA256 helper. Pure local-file check, NO network — deviates from the ticket's draft NeedsInstall(ctx, path, want Resolved) because (a) the ticket's own test strategy mandates 'No network — the expected digest is passed in', and (b) Resolved carries only the checksum URL, never the digest (the digest is the sibling .sha256 fetched by dt-4fgk's DownloadAndVerify/fetchChecksum). Keeping it network-free single-sources fail-closed verification in DownloadAndVerify. Digest identity = version+integrity check (a version has a fixed digest). On inspection error returns need=false (never reinstall on an unchecked error); os.ErrNotExist is the only need=true error. All 4 TDD behaviors + a digest-normalization case green: missing->install (TestNeedsInstall_MissingFileNeedsInstall), match->skip changed=false (MatchingDigestSkips), mismatch->replace (MismatchNeedsReplace), unreadable target->clear error need=false via directory-at-path stand-in (UnreadableTargetErrors), normalized expected digest (NormalizesExpectedDigest). make build green (vet, golangci-lint 0 issues, all tests). Remaining dt-koaf child: dt-svmu (binary placement + result/exit), which consumes this need/changed signal.
