---
id: dt-svmu
status: closed
deps: [dt-5o0x, dt-ccmn, dt-icq8]
links: []
created: 2026-06-06T14:14:57Z
type: task
priority: 1
assignee: Andre Silva
parent: dt-koaf
tags: [dnclient, install]
---
# Binary placement + result/exit semantics

Wire the install pieces into the `install` command: place the verified binary at `DN_CLIENT_BIN_DIR` (default `/var/lib/defined/bin`), creating the directory as needed, then emit the `Result` and exit code. This is the top-level orchestration of [INST.arch](dt-ewgz.md) → [INST.resolve](dt-hts2.md) → [INST.verify](dt-4fgk.md) → [INST.idempotent](dt-5o0x.md).

## Public interface

```go
// internal/dnclient
type InstallOptions struct{ BinDir, Version string }
func Install(ctx context.Context, deps InstallDeps, opts InstallOptions) (output.Result, error)
//   deps bundles the api downloads client + http client (injected for tests)
//   returns Result{Action:"install", Changed: <downloaded?>}
```

Write atomically: download to a temp file in the target dir, `fsync`, then `rename` into place (no half-written binary); set the executable bit. Skip the write when `NeedsInstall` is false.

## Behaviors (TDD order)

1. **Fresh install places the binary** — empty bin dir → binary at `BinDir/dnclient`, executable; `Result.Changed=true`.
2. **Idempotent skip** — matching binary present → no write; `Result.Changed=false` (→ exit 2 under `--assert-changed`).
3. **Mismatch replaces atomically** — old binary replaced via temp+rename; no window with a partial file.
4. **Bin dir created** — missing `DN_CLIENT_BIN_DIR` is created.
5. **Failure propagates** — verify/download failure → error, no binary placed, non-zero exit.

## Test strategy

`t.TempDir()` for `BinDir`; `httptest` for binary + `.sha256`; inject the downloads client. Assert file presence/mode, `Result.Changed`, and atomic replacement (no partial file on failure).

## Acceptance

- Verified binary installed at the configured dir; idempotent skip/replace; atomic placement; `Result`/exit wired (changed → exit 2 under assert).

## References

- Design: [Req 1](../docs/dn-tool-design.md#1-dnclient-binary-management), [§2.3 Configuration variables](../docs/dn-tool-design.md#23-configuration-variables) (`DN_CLIENT_BIN_DIR`), [§2.12 step 2](../docs/dn-tool-design.md#212-build--migration-order).
- Result/exit: [OUT.result](dt-ccmn.md), [EXIT.map](dt-icq8.md).

Parent epic: [dt-koaf](dt-koaf.md).

## Notes

**2026-06-06T21:36:40Z**

Wired the install command end-to-end + the Install orchestrator. internal/dnclient/install.go: Install(ctx, InstallDeps{API Downloader, HTTPClient, Platform}, InstallOptions{BinDir, Version}) (output.Result, error) composing ListDownloads -> ResolveDownload -> fetchChecksum -> NeedsInstall -> (on need) placeBinary. placeBinary writes to os.CreateTemp(binDir, dnclient.tmp-*), DownloadAndVerify (fail-closed) -> fsync -> close -> chmod 0755 -> os.Rename; deferred os.Remove(tmp) cleans every failure path so the final path only ever changes via atomic rename of a verified binary (never a partial). cmd/dn-tool/install.go: installAction mirrors unenroll wiring; OS/arch gated here via DetectPlatform(runtime.GOOS/GOARCH) per dt-koaf note; ctx bounded by DN_API_TIMEOUT or defaultInstallTimeout=60s (download-friendly). Added api.Client.HTTPClient() accessor (verify.go's doc already prescribed passing api's StandardClient). No API-key pre-gate (design scopes the key to enroll/unenroll; downloads auth surfaces as an API error). All 5 TDD behaviors green: fresh-install places executable + Changed=true (FreshInstallPlacesBinary); idempotent skip Changed=false with binary endpoint forced 500 to prove no download (IdempotentSkip); mismatch replaces atomically, no temp leftover (MismatchReplacesAtomically); missing bin dir created (CreatesBinDir); verify failure -> error, no binary, no temp (VerifyFailureLeavesNoBinary). make build green; CLI smoke: 'install' attempts GET /v1/downloads, retries, fails clearly exit 1 (not notImplemented). Updated TestNewApp_SubcommandsReturnNotImplemented to drop install. Last open child of dt-koaf.
