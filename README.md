# dn-tool

**Note**: This tool was completely AI-generated. I'm using it personally, but don't assume it is safe to use. Do your due diligence before using it for anything important.

`dn-tool` is a control-plane CLI that enrolls and unenrolls Linux hosts in a
[defined.net](https://defined.net) Managed Nebula network. It calls the
defined.net REST API to create and delete host records and obtain single-use
enrollment codes, then orchestrates the proprietary `dnclient` daemon — which it
downloads and verifies at runtime. It is strictly control plane: it never
reimplements Nebula and never replaces `dnclient`.

## Commands

| Command        | Purpose                                                                      |
| -------------- | ---------------------------------------------------------------------------- |
| `install`      | Download and checksum-verify `dnclient` into `$DN_CLIENT_BIN_DIR`.           |
| `enroll`       | Create the remote host record + enrollment code, then run `dnclient enroll`. |
| `unenroll`     | Delete the remote host record and remove the local config.                   |
| `run`          | `install` → `enroll` → exec `dnclient run`; unenroll on `SIGTERM`/`SIGINT`.  |
| `write-config` | Persist the current environment config to a `0600` key-value file.           |

Global flags: `--env-file <path>`, `--force` (on `enroll`), `--assert-changed`,
`--log-text`. See the [command surface](docs/dn-tool-design.md#22-command-surface)
in the design document for details.

## Configuration

All configuration is read from `DN_*` environment variables (optionally loaded
from an `--env-file`). The full list of variables, defaults, and meanings is in
the [configuration reference](docs/dn-tool-design.md#23-configuration-variables).

### Precedence

Each setting is resolved in this order, highest first:

1. **Live environment variable** — a `DN_*` variable set in the process
   environment.
2. **Env-file value** — a key loaded from the `--env-file` path.
3. **Built-in default** — the documented default, where one exists.

A live environment variable only overrides the env-file when it is **non-empty**.
Because the environment cannot distinguish a variable set to empty (`DN_X=`) from
an unset one, an empty live value is treated as unset and falls through to the
env-file (and then the default).

## NixOS

The flake ships, alongside the binary (`packages.<system>.default`), a
`services.dnclient` NixOS module and an overlay:

```nix
{
  inputs.dn-tool.url = "github:andreswebs/dn-tool";

  outputs = { nixpkgs, dn-tool, ... }: {
    nixosConfigurations.host = nixpkgs.lib.nixosSystem {
      modules = [
        dn-tool.nixosModules.dnclient
        { nixpkgs.overlays = [ dn-tool.overlays.default ]; }
        {
          services.dnclient = {
            enable = true;
            environmentFile = "/run/secrets/dn-config.env"; # root-0600, DN_API_KEY=... (+ optional DN_* overrides)
            network = {
              networkId = "network-...";
              roleId = "role-...";
            };
          };
        }
      ];
    };
  };
}
```

The module declares four systemd units — install, the `dnclient@<name>` daemon,
enroll-on-boot, and unenroll-on-poweroff — and is documented in the
[design document](docs/dn-tool-design.md#27-nixos-module-shape-servicesdnclient).
The overlay adds `pkgs.dn-tool` (the module's `package` default) built against
your own nixpkgs. The `environmentFile` is a root-only systemd EnvironmentFile
defining `DN_API_KEY=...` (and optionally other `DN_*` overrides); keep it out of
the Nix store.

## Acknowledgement

`dn-tool` is inspired by [quickvm/defined-systemd-units](https://github.com/quickvm/defined-systemd-units).

## Authors

**Andre Silva** - [@andreswebs](https://github.com/andreswebs)

## License

This project is released into the public domain under the
[Unlicense](UNLICENSE).
