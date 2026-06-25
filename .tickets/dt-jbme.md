---
id: dt-jbme
status: closed
deps: []
links: [dt-iwl2, dt-9lem, dt-nutn]
created: 2026-06-07T21:01:41Z
type: chore
priority: 2
assignee: Andre Silva
parent: dt-koaf
tags: [dnclient, refactor, architecture]
---
# Split dnclient into three single-purpose packages (exec / dnstate / dninstall)

Split the dnclient grab-bag (subprocess exec + local-state reads + binary installer, three jobs under one name with a package doc that describes only one) into three single-purpose packages: dnclient (exec Client), dnstate (local config/host-id reads), dninstall (download/verify/place). Decouple dninstall from dnclient by injecting the resolved BinaryPath rather than importing it. Unexport the installer's internal helpers. Surfaced by an architecture review (deepening candidate 3).

## Problem

`internal/dnclient` is three unrelated jobs under one package name:

- **A — exec client** (`client.go`): `Client`, `NewExecClient` — runs the proprietary `dnclient` subprocess (`os/exec`).
- **B — local state** (`config.go`, `hostid.go`): `ConfigExists`, `ReadHostID`, `ErrNotEnrolled` — reads `<root>/<network>/dnclient.yml` (`os`, `gopkg.in/yaml.v3`).
- **C — installer** (`install.go`, `download.go`, `verify.go`, `platform.go`): `Install`, `DetectPlatform`, `Platform`, `InstallDeps`, `InstallOptions` — downloads/verifies/places the binary (`internal/api`, `net/http`, `crypto/sha256`).

Evidence of the conflation:

- The only `// Package dnclient` doc (`hostid.go:1`) describes job B alone ("reads local dnclient state written by the proprietary dnclient daemon") — silent on exec and install, misleading any reader/agent.
- `ResolveDownload`, `NeedsInstall`, `DownloadAndVerify`, `Resolved`, `Downloader` are exported but have no external callers (the only `dnclient.DownloadAndVerify` reference is a doc comment in `api/client.go:145`); they are internal seams exposed through the interface.
- `enroll` imports `dnclient` for two unrelated reasons at once — `ConfigExists` (state) and `Client` (exec).

## Decisions (scoped via architecture-review interview)

1. **Three packages**, one job each: `dnclient` (exec), `dnstate` (local state), `dninstall` (installer).
2. **Decouple dninstall from dnclient by injection** (not by importing `BinaryPath`). `dninstall` must not depend on `dnclient`.
3. **Unexport the installer's pure internals**; keep only the real interface exported.

## Design

### BinaryPath stays single-sourced, injected by the composition root

`BinaryPath(binDir) string` lives in `dnclient` (the package about the dnclient binary as an executable; it owns the `"dnclient"` filename). `Install` no longer calls it — the cmd layer computes the path once and passes the same value to both the exec client and the installer, so "install writes where the client execs" is preserved without a `dninstall → dnclient` dependency:

```go
// cmd — single point that wires the shared path into both
binPath := dnclient.BinaryPath(cfg.ClientBinDir)
client  := dnclient.NewExecClient(binPath)                        // runs it
res, _  := dninstall.Install(ctx, deps, dninstall.InstallOptions{ // writes it
	BinaryPath: binPath,   // was BinDir; Install derives dir via filepath.Dir
	Version:    cfg.ClientVersion,
})
```

`InstallOptions.BinDir` → `InstallOptions.BinaryPath`; inside `Install`, `binDir := filepath.Dir(opts.BinaryPath)` for `MkdirAll` + the same-dir temp file (atomic-rename invariant intact). The `binaryName` const leaves the installer entirely — the filename identity lives only in `dnclient.BinaryPath`.

### Package layout

```
dnclient/   Client, NewExecClient, BinaryPath              deps: os/exec
dnstate/    ConfigExists, ReadHostID, ErrNotEnrolled       deps: os, yaml
            (private configPath; gets the accurate package doc)
dninstall/  Install, InstallDeps, InstallOptions,          deps: api, net/http, crypto/sha256
            DetectPlatform, Platform, Downloader           (NO dependency on dnclient)
```

### Unexport (interface = test surface; tests already in-package)

- Unexport: `ResolveDownload`→`resolveDownload`, `NeedsInstall`→`needsInstall`, `DownloadAndVerify`→`downloadAndVerify`, `Resolved`→`resolved` (already-private: `fetchChecksum`, `placeBinary`, `fileSHA256`, `get`).
- Keep exported: `Install`, `InstallDeps`, `InstallOptions`, `DetectPlatform`, `Platform`, and `Downloader` — the last is the type of the exported `InstallDeps.API` field (the downloads-API injection seam), a legitimate part of the interface, not an internal seam.

## Consumer import changes

- `internal/enroll/state.go` — `dnclient.ConfigExists` → `dnstate.ConfigExists`; keeps `dnclient.Client` (Deps.DNClient). Now imports two dn* packages for two explicit reasons.
- `internal/unenroll/unenroll.go` — `dnclient.ReadHostID`/`ErrNotEnrolled` → `dnstate.*`.
- `internal/run/run.go` — unchanged (`dnclient.Client` stays in `dnclient`).
- `cmd/dn-tool/install.go` — `dnclient.{Install,DetectPlatform,InstallDeps,InstallOptions}` → `dninstall.*`; computes `dnclient.BinaryPath(...)` and passes `InstallOptions.BinaryPath`.
- `cmd/dn-tool/enroll.go`, `cmd/dn-tool/run.go` — `dnclient.{NewExecClient,BinaryPath}` unchanged.

## Sequencing

This lands FIRST: every other open architecture ticket is made to depend on it (see deps), so they are written against the final package layout and avoid an import-churn rebase. Real file overlap: dt-nutn / dt-iwl2 / dt-41ww (enroll/state.go), dt-9lem (unenroll/unenroll.go). Lighter overlap (same file, different lines): dt-4l0b (run.go — import actually unchanged), dt-wt7j (cmd/install.go). Depend-on enforces the "do the wide split before the local edits" order.

## Tests (move, don't rewrite)

- Split the existing in-package tests by destination: `client_test.go`→dnclient; `config_test.go`/`hostid_test.go`→dnstate; `install_test.go`/`download_test.go`/`verify_test.go`/`platform_test.go`→dninstall. They stay `package <dest>` (white-box) so unexported helpers remain testable.
- `cmd/dn-tool/enroll_test.go` uses `dnclient.BinaryPath` — unchanged.
- No behavior assertions change; this is a move + rename + injection refactor.

## Acceptance criteria

- Three packages exist: `internal/dnclient` (exec), `internal/dnstate` (local state), `internal/dninstall` (installer).
- `dninstall` does NOT import `dnclient`; `Install` takes `InstallOptions.BinaryPath` and derives the dir from it; the atomic-rename-in-same-dir invariant holds.
- `BinaryPath` is single-sourced in `dnclient`; cmd passes the identical value to `NewExecClient` and `Install`.
- `dnstate` carries an accurate package doc; the old job-B-only doc no longer mislabels a multi-job package.
- Installer internals (`resolveDownload`, `needsInstall`, `downloadAndVerify`, `resolved`) are unexported; `Install`/`InstallDeps`/`InstallOptions`/`DetectPlatform`/`Platform`/`Downloader` stay exported.
- All consumers compile against the new imports; no behavior change.
- `make build` is green (quality gate).

## References

- Architecture review (deepening candidate 3), report at `.local/tmp/architecture-review-20260607-125102.md`.
- design §2.9 (internal structure), Req 1 (binary management), §2.6 (host-id retrieval).
- Blocks: dt-nutn, dt-iwl2, dt-41ww, dt-9lem, dt-4l0b, dt-wt7j (all made to depend on this).

