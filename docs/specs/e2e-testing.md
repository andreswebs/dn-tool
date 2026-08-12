# End-to-end testing plan

How to verify that `dn-tool` actually works, beyond the unit suite. `dn-tool` is
a control plane: it wraps the [defined.net REST API](../research/defined-net-api-reference.md)
and orchestrates the proprietary `dnclient` daemon. "Does it work" therefore
spans four layers, from a fast offline CLI contract check to a full NixOS VM
integration test. This document specifies each layer: its goal, prerequisites,
what it covers, concrete cases, and a harness sketch.

See also the authoritative [design](../dn-tool-design.md) (§2.8 output contract,
§2.11 testing, §2.7 module shape) and the
[API reference](../research/defined-net-api-reference.md) for the exact endpoint
and download shapes the fakes must mimic.

## What makes this non-trivial (read first)

Two structural facts shape every layer below:

1. **The `dnclient` daemon is proprietary and Linux-only.** `dn-tool` never
   reimplements it — it downloads it and execs it. The exec boundary is the
   `dnclient.Client` interface (`internal/dnclient/client.go`), so unit tests
   mock it. Anything past that boundary needs either a _fake `dnclient`
   executable_ (Layer 2) or the _real binary on Linux_ (Layer 3/4).
2. **Install is gated to Linux.** `runInstall` calls `DetectPlatform` and fails
   on non-Linux with `dn-tool requires linux`. So `install`, `enroll`, and `run`
   only do real work on Linux. macOS dev hosts can run Layer 1 plus the
   `install` path only up to the OS gate.

Two configurability facts that the harnesses exploit or work around:

- **`DN_API_URL` is honored end to end** (default `https://api.defined.net`) →
  point it at a local fake server to test the API paths without an account.
- **`DN_CLIENT_BIN_DIR` is configurable** → point `install` at a temp dir.
- **`DN_CLIENT_CONFIG_DIR` is configurable** (default `/etc/defined`) → point the
  dnclient config root (where `<network>/dnclient.yml` lives) at a temp dir, so
  binary-level `enroll`/`unenroll`/`run` filesystem behavior is testable without
  root or a container. (For the _real_ `dnclient`, this root must match where the
  daemon itself writes `dnclient.yml`; the default matches its convention. The
  override's payoff is the fake-`dnclient` harness below.)

### Reference: the contracts the fakes must honor

| Concern             | Contract (from code)                                                                                                                                                                                                                                                                                                                                       |
| ------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Management API auth | `Authorization: Bearer $DN_API_KEY` against `DN_API_URL`                                                                                                                                                                                                                                                                                                   |
| Create host + code  | `POST /v2/host-and-enrollment-code` → `{data:{host, enrollmentCode}}`                                                                                                                                                                                                                                                                                      |
| List hosts          | `GET /v2/hosts?filter.networkID=…` (cursor-paginated)                                                                                                                                                                                                                                                                                                      |
| Delete host         | `DELETE /v1/hosts/{hostID}` (v1 only; 404 == idempotent success)                                                                                                                                                                                                                                                                                           |
| Downloads           | `GET /v1/downloads` → table; binary at `<url>`, checksum at sibling `<url>.sha256`                                                                                                                                                                                                                                                                         |
| dnclient enroll     | execs `<DN_CLIENT_BIN_DIR>/dnclient enroll -name <network> -code <code>`                                                                                                                                                                                                                                                                                   |
| dnclient run        | execs `<DN_CLIENT_BIN_DIR>/dnclient run -server <DN_API_URL> -name <network>`                                                                                                                                                                                                                                                                              |
| Local config        | `<root>/<network>/dnclient.yml` with a `host_id:` field                                                                                                                                                                                                                                                                                                    |
| stdout              | exactly one JSON result envelope per command (`install`/`enroll`/`unenroll`/`write-config`), opening with the `schema_version`/`ok` head; `run` emits none                                                                                                                                                                                                 |
| stderr              | slog diagnostics at or above `--log-level` (default `warn`): JSON when stderr is not a terminal, text when it is; secrets redacted to `REDACTED`. One JSON error envelope per **failure**, written last — a non-failure status writes none, so exit `1` (`--assert-changed` no-op) and exits `130`/`143` (graceful shutdown) carry log records only        |
| Exit codes          | Per [ADR 0001](../adr/0001-exit-code-taxonomy.md): `0` success · `1` `--assert-changed` no-op (a result) · `64` usage/unknown command · `65` data (API 4xx, checksum) · `69` API unreachable (transport/5xx) · `70` unclassified · `74` local I/O · `78` config · `run` also exits `130`/`143` on SIGINT/SIGTERM and passes through the daemon's exit code |

---

## Layer 0 — Unit suite (exists; the regression backbone)

**Goal.** Lock the logic of every internal package against fakes.

**Status.** Done. `make build` runs `go vet`, `golangci-lint`, and the full
`go test ./...` (37 test files), then cross-compiles. Coverage includes the
enroll state machine (all §2.4 cells + `--force`), config precedence + env-file
SEC3, checksum fail-closed, exit-code mapping, secret redaction, and the run
lifecycle (compose order, signal→unenroll, exit-code propagation) — all with
`httptest` and the mocked `dnclient.Client`.

**Run.** `make build` (or `make test` / `make test-race`).

**Gap it cannot close.** Everything is faked. It never proves the real flag
parsing, the real config bootstrap, the real subprocess exec, or that the
documented API/dnclient contracts match reality. Layers 1–4 close those.

---

## Layer 0b — In-process golden triple (exists; the contract snapshot)

**Goal.** Freeze the CLI's exact stdout, stderr and exit code per scenario, so
any change to the output or exit contract shows up as a reviewable diff.

**Status.** Done, per [ADR 0006](../adr/0006-cli-testing.md). The suite lives in
`internal/command/golden_test.go` (harness) and
`internal/command/golden_scenarios_test.go` (cases), with goldens under
`internal/command/testdata/<scenario>/{stdout,stderr,exit}`. Scenarios drive the
framework-free delegate — `command.Run(args, deps)` — with buffer streams and a
fake environment, so the suite survives a CLI-framework replacement and is the
safety net for one.

**Run.** `go test ./internal/command`. Regenerate the goldens with
`go test ./internal/command -update`, then review the diff.

**How it differs from the exec-based layers.** It is in process: no built binary,
no subprocess, no container, and no process-level behavior. It reaches the
management API through `httptest` and `dnclient` through a shell stand-in placed
in a temp `DN_CLIENT_BIN_DIR`, and it normalizes every volatile value (temp
paths, ephemeral ports, the build version, the log `run_id` and record
timestamps) to a stable token before comparing.

**Which layer owns which exit code.**

| Exit code                              | Owner                                                                                                      |
| -------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| `0`, `1`, `64`, `65`, `69`, `74`, `78` | Layer 0b, through the CLI, one scenario each at minimum (Layers 1–3 re-verify them on the real binary)     |
| `70` (unclassified)                    | Layer 0 — no CLI invocation can provoke an unclassified error; asserted on a constructed one               |
| `130` / `143` (signal)                 | Layer 0 (`internal/run/signal_test.go`) plus Layer 1b (`e2e/signal_test.go`), which delivers a real signal |
| daemon exit-status passthrough         | Layer 0 (`internal/run/run_test.go`) plus the exec-based container suite (Layer 2)                         |

The three rows Layer 0b cannot reach are exempted **explicitly** in
`exemptFromCoverage` (`internal/command/golden_test.go`), each naming the test
that does cover it; the harness fails if any other declared code goes
unexercised.

---

## Layer 1b — Exec-level signal suite (exists; the process-behavior net)

**Goal.** Cover what an in-process harness structurally cannot: behavior that
needs a real process receiving a real signal.

**Status.** Done, in `e2e/` (`go test ./e2e`, part of `make build`). It builds
the binary, serves a fake management API on loopback, serves a shell stand-in
for `dnclient` as the download `install` verifies and places, drives the full
`install → enroll → daemon` chain, then signals the process.

**What it covers.** On SIGTERM and on SIGINT: the shutdown unenroll runs (the
fake API sees the `DELETE`), the process exits `143` / `130`, stdout stays
empty, and **stderr carries no error envelope and no `ok:false`** — a graceful
stop is not a failure (design Requirement 5, ADR 0005). This is the assertion
whose absence let the graceful-stop error envelope reach real output (dt-nrq1).

**Prerequisites.** Linux on amd64 or arm64 (the `install` platform gate); the
test skips elsewhere, since on another host it would pin the platform rejection
instead.

---

## Layer 1 — CLI contract (offline, runs on any OS incl. this macOS)

**Goal.** Exercise the _real_ binary's command surface, flag parsing, config
bootstrap, output streams, and exit codes — no network, no `dnclient`.

**Prerequisites.** A built binary (`make build` → `bin/dn-tool-<os>-<arch>`).
Nothing else.

**Why it is valuable.** It is the first thing that runs the real `newApp()`
wiring rather than mocks, so it catches gaps like an unwired command (the class
of bug that left `run` as `notImplemented`). It is fast and hermetic.

**What it covers / concrete cases.**

- **Discovery:** `--version` prints the stamped version; `--help` lists exactly
  the six commands; each `<cmd> --help` works; `--force` appears only on
  `enroll`.
- **Unknown command** → exit `64`, an error envelope with
  `"code":"unknown_command"` on stderr and nothing on stdout. A bare `dn-tool`
  behaves the same with `"code":"no_command"`.
- **Missing required config:** `unenroll` with no `DN_API_KEY` → clear
  `DN_API_KEY is required` error, exit `78` (config class), nothing written to
  `/etc/defined`. `enroll` with no `DN_API_KEY`/`DN_NETWORK_ID`/`DN_ROLE_ID` →
  error naming the missing parameter, exit `78`.
- **`write-config`** (fully offline): writes the target file at mode `0600`
  (assert `stat` perms, mask-independent), and the file round-trips back through
  `--env-file` to the same config. Confirms SEC2.
- **`--env-file` precedence + SEC3:** a key set in both the env file and the
  live env resolves to the live env value; a value like
  `DN_TAGS='$(touch /tmp/pwned)'` is treated as literal data (no file created,
  no shell execution).
- **Output streams:** a command's JSON result envelope goes to **stdout** and is
  parseable by `jq` (assert `.schema_version == 1` and `.ok == true` alongside the
  payload); logs go to **stderr**, as JSON when stderr is redirected and as text
  at a terminal, while stdout stays JSON either way.
- **`--assert-changed`:** a no-op command (e.g. `write-config` is always a
  change; use `install` idempotent skip on Linux, or an `enroll`
  already-enrolled no-op on Linux) exits `1` (a result) with its `ok:true`
  result envelope on stdout and **no error envelope on stderr**; a real change
  exits `0`; a failure exits in the sysexits range (`64`-`78`), never `1`.
- **Verbosity:** the default level is `warn`, so a successful command writes
  nothing to stderr; `--log-level debug` opens it up, `--quiet` closes it to
  errors only, and a bad `--log-level` exits `64` while a bad `DN_LOG_LEVEL`
  exits `78`.
- **Secret redaction:** with `DN_LOG_LEVEL=debug`, no stderr line contains the
  `DN_API_KEY` value; it appears only as `REDACTED`.

**Harness.** A `shelltest` / `bats` suite, or a Go `*_test.go` that execs the
built binary. Drive the binary, capture stdout/stderr/exit code, assert with
`jq` on stdout JSON and substring/regex on stderr.

```sh
BIN="bin/dn-tool-$(go env GOOS)-$(go env GOARCH)"

# missing key → clear error, exit 78 (config class)
env -i "$BIN" unenroll; test $? -eq 78

# write-config is 0600 and round-trips
tmp="$(mktemp -d)"
env -i DN_API_KEY=secret DN_NETWORK_ID=net DN_ROLE_ID=role \
  "$BIN" write-config "${tmp}/dn.env"
test "$(stat -f '%Lp' "${tmp}/dn.env" 2>/dev/null || stat -c '%a' "${tmp}/dn.env")" = "600"
env -i "$BIN" --env-file "${tmp}/dn.env" write-config "${tmp}/dn2.env"  # parses, no exec

# stdout is one result envelope opening with the schema_version/ok head
env -i DN_API_KEY=secret DN_NETWORK_ID=net DN_ROLE_ID=role \
  "$BIN" write-config "${tmp}/dn3.env" \
  | jq -e '.schema_version == 1 and .ok == true and .action == "write-config"'

# secret never logged
env -i DN_API_KEY=supersecret DN_LOG_LEVEL=debug "$BIN" unenroll 2>&1 \
  | grep -q supersecret && echo "LEAK" && exit 1
```

**MacOS caveat.** `install`/`enroll`/`run` stop at the Linux OS-gate; the
`write-config`, help/version, env-file, and missing-config cases run fully.

---

## Layer 2 — Fake API + fake `dnclient` (no account; Linux/container for fs paths)

**Goal.** Drive the full `install → enroll → run → unenroll` chain against a
local fake API and a fake `dnclient` executable. This is the highest-value layer
that needs no secrets: it exercises real flag parsing, real config bootstrap,
real subprocess exec, real binary placement, and real `/etc/defined` filesystem
behavior — the things Layer 0's mocks structurally cannot.

**Prerequisites.**

- Linux (the OS gate) — a container is the natural home.
- A config root the test can write: set `DN_CLIENT_CONFIG_DIR` to a temp dir (no
  root needed) and point the fake `dnclient` at the same dir.
- A fake management API server.
- A fake `dnclient` executable served by the fake downloads endpoint.

**The fake API server.** A small HTTP server (Go `net/http`, or any language)
implementing the four endpoints from the contract table. It need not be
stateful, but a tiny in-memory host store makes the orphan/`--force` and
list-and-match paths realistic:

- `GET /v1/downloads` → a downloads table whose `linux-amd64`/`linux-arm64` entry
  URL points back at this server (e.g. `…/dl/dnclient`).
- `GET /dl/dnclient` → the **fake dnclient script bytes** (see below);
  `GET /dl/dnclient.sha256` → that script's lowercase hex SHA-256.
- `POST /v2/host-and-enrollment-code` → `{data:{host:{id,name,…}, enrollmentCode:{code}}}`;
  return `400 ERR_DUPLICATE_VALUE` on `path:name` for a name already in the store
  (exercises orphan detection).
- `GET /v2/hosts?filter.networkID=…` → the store, paginated (exercise ≥2 pages).
- `DELETE /v1/hosts/{id}` → `204`; unknown id → `404` (must be treated as success).

**The fake `dnclient`.** A shell (or Go) script placed/served as the binary. It
mimics just enough of the real CLI contract:

```sh
#!/bin/sh
# fake dnclient: honor the two subcommands dn-tool execs.
case "$1" in
  enroll)  # dnclient enroll -name <net> -code <code>
    # write the local config dn-tool's unenroll/host-id read expects, into the
    # same root (inherited from dn-tool's env; default matches dn-tool's default)
    root="${DN_CLIENT_CONFIG_DIR:-/etc/defined}"
    net=""; for a in "$@"; do [ "$prev" = "-name" ] && net="$a"; prev="$a"; done
    mkdir -p "${root}/${net}"
    printf 'host_id: host-fake-123\n' > "${root}/${net}/dnclient.yml"
    ;;
  run)     # dnclient run -server <url> -name <net> : block until signalled
    trap 'exit 0' TERM INT
    while :; do sleep 1; done
    ;;
  *) echo "fake dnclient: unknown $*" >&2; exit 64 ;;
esac
```

Because `install` downloads this script into `DN_CLIENT_BIN_DIR` and `enroll`/
`run` exec it from there, the **whole chain runs end to end against fakes** —
including the real download→checksum→atomic-place, the real exec, and the real
`dnclient.yml` round-trip that `unenroll` reads back.

**What it covers / concrete cases.**

- **install:** fresh install places an executable at `${DN_CLIENT_BIN_DIR}/dnclient`,
  `Result.changed=true`; re-run is an idempotent skip (`changed=false`, exit `1`
  under `--assert-changed`); a deliberately wrong served `.sha256` → fail-closed,
  no binary placed, exit `65` (data class: checksum mismatch).
- **enroll (state machine, end to end):** absent/absent → create + code + exec
  fake enroll → `dnclient.yml` written, `changed=true`; re-run with local config
  present → no-op `changed=false`; orphan (remote present, local absent) → fail
  with `--force` guidance; `--force` → DELETE then re-enroll. Assert the fake API
  saw the expected method/path/body, and the enrollment code never appears in
  logs.
- **unenroll:** with `dnclient.yml` present → `DELETE /v1/hosts/host-fake-123`
  then local dir removed, `changed=true`; API `403` → local config **retained**,
  exit `65` (data class, a 4xx rejection; the §2.5 invariant still holds);
  already-absent `404` → treated as success.
- **run (lifecycle):** `install → enroll → fake daemon foreground`; send
  `SIGTERM` → fake daemon exits 0, then `unenroll` runs (real DELETE), process
  exits `143` (`128 + SIGTERM`, the graceful-shutdown signal code) with no error
  envelope on stderr (covered by Layer 1b already); make the fake daemon `exit 7`
  with no signal → `dn-tool` exits `7` (daemon exit-code propagation, the
  registry exception) **with** an error envelope, since a daemon that died on
  its own is a real failure.

**Harness options.**

- **Binary-level (preferred for true e2e):** a `docker run` (or `nixos-container`)
  that builds/copies the Linux binary, starts the fake API, runs the chain, and
  asserts on stdout JSON (`jq`), stderr, exit codes, and `/etc/defined` state.
  Wire it as `make e2e` behind a Linux guard.
- **In-process component (faster, no root):** Go tests under a `//go:build e2e`
  tag that call the command cores (`runInstall`, `runEnroll`, `runUnenroll`,
  `runRun`) with an injected config root + `httptest` fake API + a fake dnclient
  in a `t.TempDir()` bin dir. This skips real flag parsing but needs no
  container; it is the cheapest way to cover the install→enroll→unenroll chain
  with a real exec'd fake binary.

**Note on `run`'s shutdown unenroll.** Layer 2 will observe that `run` attempts
the shutdown unenroll even when install/enroll failed early — tracked as the
open decision **dt-577o**. Resolve that before asserting on `run`'s
failed-startup log output, or the assertions will encode behavior that may change.

### Config-root override (available)

Binary-level `enroll`/`unenroll`/`run` tests run without root: set
`DN_CLIENT_CONFIG_DIR` to a temp dir and the binary reads/writes the dnclient
config there instead of `/etc/defined` (delivered in dt-47ab). The fake
`dnclient` above inherits this env var from dn-tool, so both write to the same
root.

---

## Layer 3 — Real defined.net (needs an account; one-time release gate)

**Goal.** Validate the two assumptions no fake can: that the documented API
request/response shapes match the live service, and that the real `dnclient` CLI
contract (args, exit behavior, the `host_id` field in `dnclient.yml`) matches
what the code assumes.

**Prerequisites.** A real `DN_API_KEY` with create/list/delete host scope, a real
`DN_NETWORK_ID` and `DN_ROLE_ID`, and a disposable Linux host (VM/container) with
outbound access to `api.defined.net` and the downloads CDN. Use a staging/test
network — these operations mutate real records.

**Procedure (manual, scripted as a smoke).** On the disposable host:

```sh
export DN_API_KEY="${DN_API_KEY}"
export DN_NETWORK_ID="${DN_NETWORK_ID}"
export DN_ROLE_ID="${DN_ROLE_ID}"
export DN_NETWORK_NAME="${DN_NETWORK_NAME:-defined}"

dn-tool install     # real download + checksum verify from the downloads service
dn-tool enroll      # real create+code, real `dnclient enroll`; expect a dnclient.yml
dn-tool enroll      # idempotent: already-enrolled no-op (exit 0)
dn-tool unenroll    # real DELETE + local removal
```

**What it covers.** Real downloads table/OS-arch keys and the sibling `.sha256`;
the real create/list/delete envelopes and error codes (esp. `ERR_DUPLICATE_VALUE`
on `name`); the real `dnclient.yml` schema (`host_id`); the real `dnclient enroll`
arg contract and exit behavior.

**Cautions.** Mutates live records — always unenroll/clean up; prefer a
throwaway network. Treat any mismatch with
[the API reference](../research/defined-net-api-reference.md) as a doc-vs-reality
bug to reconcile (the reference drove the fakes, so a mismatch invalidates Layer
2 assumptions too).

**When to run.** Once before any release, and whenever the API client, install,
or enroll/unenroll request shapes change. Not in per-commit CI (needs secrets +
mutates state).

---

## Layer 4 — NixOS VM test (lives in `nix-packages`, not this repo)

**Goal.** Validate the `services.dnclient` systemd integration end to end: unit
ordering (install → enroll-on-boot → daemon), `ExecStop` unenroll, the
`TimeoutStopSec > DN_API_TIMEOUT` race (finding D5), and the
**reboot-vs-poweroff** discrimination spike (design §2.7 — the `NEEDS
CLARIFICATION` item).

**Where.** The `nix-packages` repo's `services.dnclient` module, as a
`nixosTest` (design §2.11). This repo's flake ships only the binary; the module
and its VM test are downstream. It can drive the real binary against a fake API
(Layer 2 style) inside the VM to stay hermetic, focusing assertions on the
systemd wiring rather than the API.

**What it covers that Layers 1–3 cannot.** The systemd units, boot/stop
ordering, the timeout race, and the reboot-vs-poweroff policy (`DN_SKIP_UNENROLL`
/ `DN_UNENROLL_ON_REBOOT`) — none of which exist in this repo.

---

## Suggested sequencing

1. **Layer 1 now** — fast, runs on this macOS, zero setup; immediately catches
   unwired/contract regressions. Wire as `make e2e-cli` (or fold into a
   `shelltest`/`bats` suite).
2. **Decide dt-577o**, then build the **Layer 2** harness as the durable
   integration net (`make e2e`, Linux/container-guarded). The binary-level
   variant now runs unprivileged via `DN_CLIENT_CONFIG_DIR` (dt-47ab); the
   in-process component variant injects the root directly.
3. **Layer 3** as a documented, scripted manual gate before releases.
4. **Layer 4** in `nix-packages` alongside the module work.

## Open items this plan depends on

- **dt-577o** — decision: should `run` skip the shutdown unenroll when enroll
  never happened? Affects Layer 2 `run` assertions.
