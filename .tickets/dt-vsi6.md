---
id: dt-vsi6
status: closed
deps: []
links: []
created: 2026-06-06T11:01:33Z
type: epic
priority: 0
assignee: Andre Silva
tags: [scaffolding, build, flake, ci, distribution]
---

# Repo scaffolding & distribution

Stand up the dn-tool repo as a buildable, distributable Go project: module github.com/andreswebs/dn-tool, urfave/cli command skeleton (install/enroll/unenroll/run/write-config) with global flags (--env-file, --force, --assert-changed, --log-text), a Nix flake exposing `packages.<system>.default` via buildGoModule (buildable on aarch64-darwin; real targets x86_64-linux/aarch64-linux), and CI. Foundation that every other epic builds on.

## Design

Layout per §2.9: cmd/dn-tool/main.go for CLI wiring; internal/{config,api,dnclient,enroll,output}. CLI=urfave/cli (§2.10). Flake does NOT ship the NixOS module (lives in nix-packages). README gets an Acknowledgement section; UNLICENSE retained; no upstream MIT license.

## Acceptance Criteria

Go module + flake build the binary locally; CLI exposes the five subcommands and global flags as stubs; CI runs make build (fmt-check, vet, lint, test, compile).

## Current state & gaps (2026-06-06)

Pre-implementation. Present: `Makefile` (build/validate/dist/run + cross-compile), `src/go.mod` (module `github.com/andreswebs/dn-tool`, go 1.26.4), stub `src/cmd/dn-tool/main.go`, `UNLICENSE`, `README.md`, `AGENTS.md` (`CLAUDE.md` is a symlink to it). Scaffolding must still add:

- `flake.nix` (absent) — `packages.<system>.default` via `buildGoModule`; needs `vendorHash`/`go.sum`.
- ~~`internal/version` package — the Makefile `LDFLAGS` set `-X …/internal/version.Override`.~~ Added 2026-06-06: `internal/version` exposes `Override` (string) + `Current()` (Override → `debug.ReadBuildInfo` → `"dev"`), with tests; ldflags injection verified.
- The `internal/{config,api,dnclient,enroll,output}` skeleton (§2.9).
- ~~**Makefile bug:** the `dist` target copies a `LICENSE` file, but the repo ships `UNLICENSE`.~~ Fixed 2026-06-06: `dist` now copies `UNLICENSE`.

## References

- Design: [§2.1 Repository & distribution](../docs/dn-tool-design.md#21-repository-and-distribution), [§2.2 Command surface](../docs/dn-tool-design.md#22-command-surface), [§2.9 Internal structure](../docs/dn-tool-design.md#29-internal-structure-tentative), [§2.10 CLI / libraries](../docs/dn-tool-design.md#210-cli--libraries), [§2.12 step 1](../docs/dn-tool-design.md#212-build--migration-order).
- Research: [urfave/cli reference](../docs/research/urfave-cli-reference.md) for the command/flag skeleton.

## Notes

**2026-06-06T11:04:58Z**

Out-of-scope upstream findings that belong to the NixOS services.dnclient module in nix-packages (NOT this repo): B1 (StartLimitInterval=5 + RestartSec=120 makes the restart limiter impossible to trip), B2 (dnctl.service depends on bare dnclient.service that never exists; needs the dnclient@%i.service instance), D5 (no TimeoutStopSec on the stop path), SEC4 (no systemd sandboxing/hardening), D2/D3/D6 (envsubst, vestigial After=, single-network template), B5 (README claim). N/A (bash artifacts the Go rewrite removes): S1/S3/S5/D1. Obsolete: B6 (reenroll command dropped, §2.2). Tracking note only - no work here.

**2026-06-06T20:00:18Z**

CI added: .github/workflows/ci.yml runs the full quality gate via 'make build' (fmt-check, vet, lint, test, compile) on push to main + all PRs. Go version read from src/go.mod via actions/setup-go@v5 (go-version-file) so CI tracks the module toolchain; golangci-lint pinned to v2.12.2 (matching src/.golangci.yml's v2 schema) installed from the version-tagged install.sh onto GOPATH/bin and added to $GITHUB_PATH so 'make lint' finds it. permissions: contents:read (least privilege). Validated YAML structure with pyyaml and confirmed 'make build' stays green. This was the last open acceptance item for the epic; all four children (dt-yw6f/dt-ecf9/dt-prt6/dt-0mdh) were already closed, so the P0 scaffolding epic is complete. Flake build job intentionally NOT added (acceptance is 'CI runs make build' only; the FOD/nix build also can't be validated in this sandbox per dt-0mdh).
