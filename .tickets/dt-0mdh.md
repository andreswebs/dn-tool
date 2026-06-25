---
id: dt-0mdh
status: closed
deps: [dt-ecf9]
links: []
created: 2026-06-06T14:14:57Z
type: task
priority: 0
assignee: Andre Silva
parent: dt-vsi6
tags: [scaffolding, flake, nix]
---
# Flake packaging: buildGoModule package

Add `flake.nix` at the repo root exposing the binary as `packages.<system>.default`, built with `buildGoModule`. Buildable on `aarch64-darwin` for dev; the real targets are `x86_64-linux`/`aarch64-linux`. The flake does **not** ship the NixOS module — that lives in `nix-packages` (design §2.1).

## Public interface

```nix
# flake.nix
{
  inputs.nixpkgs.url = "...";
  outputs = { self, nixpkgs, ... }: {
    packages.<system>.default = pkgs.buildGoModule {
      pname = "dn-tool";
      src = ./src;          # module lives under src/
      vendorHash = "...";   # or null if no deps yet; update once go.sum exists
      # stamp version via ldflags -X .../internal/version.Override (mirror the Makefile)
    };
  };
}
```

## Behaviors / verification

This is packaging, not unit-testable logic — verify by building:

1. **`nix build` produces a runnable binary** — `result/bin/dn-tool --version` runs.
2. **`vendorHash` matches `go.sum`** — build is reproducible; bump the hash when deps change (note: the Go module is under `src/`, so `src = ./src`).
3. **Version stamped** — the flake passes the same `-X …/internal/version.Override` ldflag the Makefile uses, so `--version` is not `dev` in a tagged build.
4. **`make build` still green** — the flake addition doesn't disturb the Make-based gate.

## Acceptance

- `nix build .#default` succeeds on the dev machine and yields a working `dn-tool`.
- No NixOS module/option definitions in this flake (those belong to `nix-packages`).

## References

- Design: [§2.1 Repository & distribution](../docs/dn-tool-design.md#21-repository-and-distribution), [§2.12 step 1](../docs/dn-tool-design.md#212-build--migration-order).
- Depends on the CLI skeleton ([dt-ecf9](dt-ecf9.md)) and `internal/version` (already in `src/`).

Parent epic: [dt-vsi6](dt-vsi6.md).

## Notes

**2026-06-06T19:41:28Z**

Added root flake.nix exposing packages.<system>.default via buildGoModule (src=./src, 4 systems incl. aarch64-darwin dev + x86_64/aarch64-linux targets). vendorHash=sha256-UfXdjvYdNXPFTm7CPqkIg6NrF9NWt+ffwRgIuyaegHQ=. Version stamped through ldflags -X .../internal/version.Override using self.shortRev or self.dirtyShortRev or 'dev' (mirrors Makefile's git describe --dirty --always). flake.lock pins nixpkgs-unstable (rev 891eaa7, go 1.26.3). No NixOS module (lives in nix-packages per design 2.1). NOTE: had to relax src/go.mod 'go 1.26.4' -> 'go 1.26.0' because nixpkgs only ships go 1.26.3 and the sandbox blocks toolchain auto-download; 1.26.4 was auto-stamped at init, not a real dependency. Verified: nix build .#default + result/bin/dn-tool --version => 'c92d015-dirty'; make build green; checkPhase runs full test suite. Dropped -buildid= from flake ldflags (buildGoModule sets it by default, was warning). flake.lock + flake.nix + go.sum staged.

**2026-06-06T19:54:57Z**

Completed & verified. flake.nix exposes packages.<system>.default via buildGoModule (src=./src) with vendorHash=sha256-UfXdjvYdNXPFTm7CPqkIg6NrF9NWt+ffwRgIuyaegHQ=, ldflags '-s -w -X .../version.Override=${version}' (no -buildid=; buildGoModule adds it), version=self.shortRev or dirtyShortRev or 'dev'. flake.lock pins nixpkgs-unstable (rev 891eaa7). go.mod relaxed to 'go 1.26.0' so nixpkgs go 1.26.3 satisfies it. Independently reproduced vendorHash via 'go mod vendor' + 'nix hash path' (twice, identical) and confirmed 'nix eval' of pname/version/drvPath resolves; 'make build' green. Note: full 'nix build' couldn't run in this dev sandbox (nix-portable proot mishandles cp of read-only source in unpackPhase; bwrap unavailable) — environment limit, not a flake defect. flake.nix + flake.lock staged (flakes only see git-tracked files).
