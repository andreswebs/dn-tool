---
id: dt-ecf9
status: closed
deps: []
links: []
created: 2026-06-06T13:57:39Z
type: task
priority: 0
assignee: Andre Silva
parent: dt-vsi6
tags: [scaffolding, cli]
---
# CLI skeleton: subcommands + global flags + --version

Stand up the `urfave/cli` application skeleton in `cmd/dn-tool/main.go`: the root command, the five subcommands as stubs, the global flags, and `--version` wired to `internal/version`. This is the seam every other epic slots into — keep the stubs returning a clear "not implemented" error so the binary builds and `--help`/`--version` work end to end.

## Public interface

```go
// cmd/dn-tool/main.go
func main()              // build app, run, map returned error -> process exit (see EXIT.map / dt-icq8)
func newApp() *cli.Command   // root; testable in isolation — no os.Exit, no real I/O
```

Subcommands (stubs returning `errors.New("not implemented")`): `install`, `enroll`, `unenroll`, `run`, `write-config`.
Global flags: `--env-file <path>`, `--assert-changed`, `--log-text`. `--force` is scoped to `enroll` only (design §2.2). `--version` prints `version.Current()`.

Match the API shape in the [urfave/cli reference](../docs/research/urfave-cli-reference.md) rather than guessing the version/idioms.

## Behaviors (TDD order)

Vertical slices — one test → minimal code → next. Do **not** write all tests first.

1. **App builds with five subcommands** — `newApp()` exposes commands named exactly `install/enroll/unenroll/run/write-config`.
2. **Global flags registered** — `--env-file`, `--assert-changed`, `--log-text` present at root with correct types.
3. **`--force` is enroll-scoped** — present on `enroll`, absent at root and on other commands.
4. **`--version` reports `version.Current()`** — run `["dn-tool","--version"]`, assert output contains the resolved version.
5. **Stub command returns an error** — running any subcommand returns the not-implemented error ([EXIT.map](dt-icq8.md) consumes it).

## Test strategy

Construct `newApp()` and assert on its command/flag metadata (public introspection). For `--version`/stub behavior, call `app.Run(ctx, args)` with captured stdout/stderr buffers. No global state; no `os.Exit` inside `newApp()`.

## Acceptance

- `make build` green; `dn-tool --help` lists the five commands; `dn-tool --version` prints the version.
- Every subcommand is reachable and returns the not-implemented error.

## References

- Design: [§2.2 Command surface](../docs/dn-tool-design.md#22-command-surface), [§2.9 Internal structure](../docs/dn-tool-design.md#29-internal-structure-tentative), [§2.10 CLI / libraries](../docs/dn-tool-design.md#210-cli--libraries).
- [urfave/cli reference](../docs/research/urfave-cli-reference.md).

Parent epic: [dt-vsi6](dt-vsi6.md).

## Notes

**2026-06-06T18:02:39Z**

CLI skeleton landed (urfave/cli/v3 v3.9.0). cmd/dn-tool/main.go: newApp() builds the root *cli.Command with five subcommand stubs (install/enroll/unenroll/run/write-config) each returning the exported errNotImplemented sentinel; global flags --env-file (Sources DN_ENV_FILE), --assert-changed, --log-text at root; --force scoped to enroll only. --version wired to version.Current() via Command.Version. main() runs the app, routes ExitCoder errors through cli.HandleExitCoder and prints plain errors to stderr + exit 1 (placeholder until dt-icq8 wires proper 0/1/2 exit-code mapping). Tests in main_test.go assert command set, flag placement, --version output, and that every stub returns errNotImplemented. go.mod requires only urfave/cli/v3 (stdlib-only, no indirect deps); go.sum committed for the flake vendorHash work (dt-0mdh). make build green.
