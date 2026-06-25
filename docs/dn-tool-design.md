# dn-tool — Design

A Go rewrite of the bash `dnctl` control script for managing host enrollment in a
[defined.net](https://defined.net) Managed Nebula network. The tool is pure
control plane: it wraps the defined.net REST API calls and orchestrates the
proprietary `dnclient` daemon. It is designed to run identically under systemd
(NixOS), inside containers, and from CI/automation pipelines.

Background analysis of the upstream implementation and its bugs:
[quickvm-defined-systemd-units-analysis.md](./research/quickvm-defined-systemd-units-analysis.md).
The original upstream source that analysis is based on — the `bin/dnctl` control
script and the `units/*.service` files — is preserved for reference at
[defined-systemd-units.yek.txt](./research/defined-systemd-units.yek.txt).

Finding identifiers used throughout this document (`B#` bugs, `S#` shell issues,
`SEC#` security issues, `D#` design issues) refer to the
[analysis](./research/quickvm-defined-systemd-units-analysis.md)'s summary table.

The defined.net REST API and `dnclient` download details this design relies on —
endpoints, request/response shapes, error codes, token scopes, the OS/arch
download keys, and binary checksum retrieval — are documented in
[defined-net-api-reference.md](./research/defined-net-api-reference.md).

---

## Part 1 — Requirements

### Introduction

`dn-tool` is a standalone command-line tool that enrolls and unenrolls a Linux
host in a defined.net Managed Nebula network. It replaces the upstream bash
`dnctl` script with a single statically-compiled Go binary that runs anywhere
Linux runs — under systemd, inside a container, or from an automation pipeline —
without assuming a systemd context.

The tool is strictly a control plane. The actual Nebula data plane is the
proprietary, closed-source `dnclient` daemon from defined.net, which `dn-tool`
downloads at runtime and invokes. `dn-tool` talks to the defined.net REST API to
create and delete host records and to obtain single-use enrollment codes, then
hands those codes to `dnclient enroll`. It never reimplements Nebula and never
replaces `dnclient`.

The binary is distributed from its own repository (`andreswebs/dn-tool`) with a
Nix flake. The same flake also ships the NixOS integration — the
`services.dnclient` module (systemd units, firewall, configuration plumbing) —
as an independent output (`nixosModules.dnclient`). Co-locating the module with
the binary keeps their tightly-coupled contract (env vars, paths, the `dnclient`
invocation) versioned together. Because the module is a separate, opt-in flake
output, container and pipeline consumers that use only the binary or the image
never evaluate it and pay nothing for it.

### Requirements

#### 1. dnclient binary management

**User Story**: As an operator, I want the tool to obtain the correct `dnclient`
binary at runtime, so that I do not have to package or pre-install a proprietary
binary myself.

**Acceptance Criteria**:

- The system shall download the `dnclient` binary from the URL advertised by the
  defined.net downloads API for the host's OS and CPU architecture.
- The system shall place the downloaded binary under a configurable directory,
  defaulting to `/var/lib/defined/bin`.
- WHERE a binary version is configured, the system shall download that version;
  otherwise the system shall download the version the API designates as `latest`.
- The system shall verify the downloaded binary against the checksum the downloads
  service publishes for it — a sibling `<binary-url>.sha256` file, not a field in
  the downloads API response (see the
  [API reference](./research/defined-net-api-reference.md#63-checksum)).
- IF the published checksum cannot be fetched, THEN the system shall not install
  the binary and shall fail with a clear error.
- WHEN a binary already exists at the target path and matches the expected
  version and checksum, the system shall skip downloading.
- WHEN a binary already exists at the target path but does not match the expected
  version or checksum, the system shall re-download and replace it.
- IF the host operating system is not Linux, THEN the system shall fail with a
  clear error.
- IF the host CPU architecture cannot be mapped to a known `dnclient`
  architecture, THEN the system shall fail with a clear error.
- IF checksum verification fails, THEN the system shall not install the binary
  and shall fail with a clear error.

#### 2. Host enrollment

**User Story**: As an operator, I want to enroll a host into a Nebula network,
so that it can join the overlay and communicate with other hosts.

**Acceptance Criteria**:

- The system shall require a management API key, a network ID, and a role ID
  before attempting enrollment.
- IF any required enrollment parameter is missing or empty, THEN the system shall
  fail with a clear error naming the missing parameter.
- The system shall determine the enrollment hostname from the configured hostname
  when set, and otherwise from the system hostname.
- The system shall request the host's Nebula tun device name to match the
  configured network name.
- WHERE a static Nebula IP address is configured, the system shall request it
  during enrollment.
- WHERE tags are configured, the system shall assign them during enrollment.
- WHEN a local `dnclient` configuration for the network already exists, the
  system shall treat the host as already enrolled and make no changes.
- WHEN no local configuration exists and no remote host record with the
  enrollment hostname exists, the system shall create a remote host record,
  obtain a single-use enrollment code, and run `dnclient enroll` with it.
- IF a remote host record with the enrollment hostname already exists but no
  local configuration is present (an orphaned enrollment), THEN the system shall
  fail with a clear error and instruct the operator to re-run with a force
  option, unless the force option is already set.
- WHERE the force option is set and an orphaned enrollment is detected, the
  system shall delete the stale remote host record and enroll the host afresh.
- IF the management API returns an error response at any enrollment step, THEN
  the system shall fail without running `dnclient enroll`.
- IF `dnclient enroll` exits non-zero, THEN the system shall fail and surface its
  error.

#### 3. Lighthouse and relay enrollment

**User Story**: As an operator, I want to enroll a host as a lighthouse or relay,
so that it can provide discovery or relaying for the network.

**Acceptance Criteria**:

- WHERE the host is configured as a lighthouse, the system shall enroll it with
  the lighthouse role and its configured static addresses.
- WHERE the host is configured as a relay, the system shall enroll it with the
  relay role.
- IF a host is configured as both a lighthouse and a relay, THEN the system shall
  fail with a clear error.
- IF a host is configured as a lighthouse without at least one static address,
  THEN the system shall fail with a clear error.
- IF a host is configured as a lighthouse or relay without a non-zero listen
  port, THEN the system shall fail with a clear error.
- WHERE a listen port is configured, the system shall enroll the host with that
  port; otherwise the system shall request a system-selected port.

#### 4. Host unenrollment

**User Story**: As an operator, I want to unenroll a host, so that it is removed
from the network and leaves no dangling record.

**Acceptance Criteria**:

- The system shall require a management API key before attempting unenrollment.
- The system shall determine the host's remote record identifier by reading it
  from the local `dnclient` configuration for the network.
- IF no local configuration exists, THEN the system shall fail with a clear error
  indicating the host does not appear to be enrolled.
- WHEN unenrolling, the system shall delete the remote host record via the
  management API.
- WHEN the remote delete succeeds, or reports the record is already absent, the
  system shall remove the local configuration directory for the network.
- The system shall complete unenrollment within a bounded, configurable deadline
  that is shorter than the surrounding service-stop timeout.
- IF the remote delete fails within the deadline, THEN the system shall retain
  the local configuration, report the failure, and exit non-zero.
- The system shall keep the local configuration and the remote host record
  consistent: it shall not remove the local configuration while the remote record
  may still exist.

#### 5. Container lifecycle command

**User Story**: As an operator running `dnclient` in a container, I want a single
command that handles the full lifecycle, so that enrollment, running, and
unenrollment work cleanly under a container's process model.

**Acceptance Criteria**:

- WHEN the lifecycle command is invoked, the system shall install the binary,
  start the `dnclient` daemon in the foreground, wait for the daemon's control
  socket, and then enroll the host. `dnclient enroll` connects to the running
  daemon's control socket, so the daemon must be up before enrollment (upstream
  finding: enrollment fails if the daemon is not active first).
- IF the daemon exits or its control socket does not appear before a bounded
  deadline, THEN the system shall fail without enrolling and stop the daemon.
- WHILE the daemon is running, WHEN the process receives a termination signal,
  the system shall unenroll the host and then exit.
- WHEN the daemon terminates on its own, the system shall propagate its
  termination outcome (exit code) to its own exit status.
- WHEN the daemon is stopped as part of a signal-initiated shutdown, the system
  shall treat its signal-kill as expected: a shutdown whose unenroll succeeds
  exits 0, and only an unenroll failure (the host may remain enrolled) yields a
  non-zero status. A clean container stop is not reported as a failure.

#### 6. Configuration interface

**User Story**: As an operator, I want to configure the tool through the
environment or a file, so that it works under systemd, in containers, and in
pipelines without changing how I invoke it.

**Acceptance Criteria**:

- The system shall read all configuration from environment variables.
- WHERE an environment-file path is provided, the system shall load
  key-value pairs from that file as configuration.
- The system shall parse environment files as plain key-value data and shall not
  execute their contents.
- WHEN both an environment file and a live environment variable set the same key,
  the system shall use a single, documented precedence order.
- WHERE a management API base URL is configured, the system shall use it instead
  of the default defined.net API URL.

#### 7. Output and observability

**User Story**: As an automation author, I want machine-readable output, so that
pipelines can consume results and logs without scraping prose.

**Acceptance Criteria**:

- WHEN a command produces a result, the system shall write that result as a
  structured JSON object to standard output.
- The system shall write diagnostic and progress logs to standard error.
- The system shall emit logs as structured JSON by default.
- WHERE human-readable logging is requested, the system shall emit logs as plain
  text to standard error.
- The system shall not write secret values, including the API key, to its logs.
- WHERE a log level is configured, the system shall emit only logs at or above
  that level.

#### 8. Exit status semantics

**User Story**: As an automation author, I want meaningful exit codes, so that
scripts can branch on success, failure, and no-op without parsing output.

**Acceptance Criteria**:

- WHEN a command succeeds, the system shall exit with status zero.
- IF a command fails, THEN the system shall exit with a non-zero error status.
- WHERE change-assertion is requested and a command makes no change, the system
  shall exit with a distinct non-error, non-zero status.

#### 9. Management API resilience

**User Story**: As an operator, I want the tool to tolerate transient network and
server failures, so that boot-time enrollment is reliable on unstable networks.

**Acceptance Criteria**:

- IF a management API call fails with a transient error (a network failure, a
  server error, or a rate-limit response), THEN the system shall retry with
  bounded exponential backoff up to a configurable overall timeout.
- IF a management API call fails with a client error indicating bad input or
  authorization failure, THEN the system shall not retry and shall fail with a
  clear error.
- WHERE an API timeout is configured, the system shall bound all API calls by it.
- The system shall verify the HTTP status of every management API response before
  acting on its body.

#### 10. Configuration file writer

**User Story**: As an operator setting up a host manually, I want to write a
configuration file from the current environment, so that I can persist settings
for later runs.

**Acceptance Criteria**:

- WHEN asked to write a configuration file, the system shall write the configured
  values as key-value pairs to the target path.
- The system shall create the configuration file with owner-only read/write
  permissions from the moment of creation.
- The system shall never write the file with broader permissions and then
  restrict them afterward.

---

## Part 2 — Implementation Plan

### 2.1 Repository and distribution

- **New repo:** `andreswebs/dn-tool`.
- **Go module path:** `github.com/andreswebs/dn-tool`.
- **Binary name:** `dn-tool`.
- **Flake:** exposes `packages.<system>.default` (the binary, built with
  `buildGoModule`). Buildable on `aarch64-darwin` for development; the real
  deployment target is `x86_64-linux` / `aarch64-linux`.
- The flake **also** ships the NixOS integration as independent outputs:
  - `nixosModules.dnclient` (aliased as `nixosModules.default`) — the
    `services.dnclient` module (§2.7), living at `nix/module.nix`.
  - `overlays.default` — adds `pkgs.dn-tool` to a consumer's package set, built
    against the consumer's own nixpkgs. The module's `package` default is
    `pkgs.dn-tool`, so a NixOS consumer applies this overlay (or sets
    `services.dnclient.package`) and gets one nixpkgs in its closure rather than
    a second one dragged in by the flake.
  - `checks.<linux>.module` — the `nixosTest` from §2.11, at `nix/tests/module.nix`.

  The package definition is shared between `packages.default` and the overlay via
  `nix/package.nix`.

**Move from `nix-packages` (historical):** an earlier draft kept the module in a
separate `nix-packages` NUR repo, with `pkgs/dn-tool` re-exporting this flake's
package. That split was reversed — the module now lives here so it versions with
the binary it drives, and `nix-packages` drops the `dnctl` package and `dnclient`
module entirely. Consumers import `dn-tool.nixosModules.dnclient` directly.

This is a complete rewrite only inspired by the upstream QuickVM project; the
README carries an "Acknowledgement" rather than the upstream's MIT
`LICENSE.upstream` (dn-tool is released under the Unlicense).

### 2.2 Command surface

| Command        | Purpose                                                                 |
| -------------- | ----------------------------------------------------------------------- |
| `install`      | Download/verify `dnclient` into `$DN_CLIENT_BIN_DIR`.                   |
| `enroll`       | Create remote host + enrollment code, run `dnclient enroll`.            |
| `unenroll`     | Delete remote host, remove local config.                                |
| `run`          | `install` → daemon, await socket → `enroll`; unenroll on signal.        |
| `write-config` | Persist current env config to a `0600` key-value file.                  |

Dropped from upstream: `start`, `stop`, `restart`, `enable` (systemctl wrappers —
now the module's job), `reenroll` (compose `unenroll` + `enroll` explicitly), and
the `DN_MIRROR_IP` feature and cloud-provider metadata lookups.

Global flags: `--env-file <path>`, `--force` (on `enroll`), `--assert-changed`,
`--log-text`.

### 2.3 Configuration variables

| Variable                | Default                     | Notes                                               |
| ----------------------- | --------------------------- | --------------------------------------------------- |
| `DN_API_KEY`            | —                           | Secret; never logged. Required for enroll/unenroll. |
| `DN_NETWORK_ID`         | —                           | Required for enroll.                                |
| `DN_ROLE_ID`            | —                           | Required for enroll.                                |
| `DN_NETWORK_NAME`       | `defined`                   | Drives tun device + config dir.                     |
| `DN_HOSTNAME`           | system hostname             | Enrollment display name.                            |
| `DN_IP_ADDRESS`         | unset                       | Optional static Nebula IP.                          |
| `DN_TAGS`               | `[]`                        | JSON array.                                         |
| `DN_IS_LIGHTHOUSE`      | `false`                     |                                                     |
| `DN_IS_RELAY`           | `false`                     |                                                     |
| `DN_STATIC_ADDRESSES`   | `[]`                        | JSON array; required for lighthouse.                |
| `DN_LISTEN_PORT`        | unset (→ 0, auto)           | Required non-zero for lighthouse/relay.             |
| `DN_API_URL`            | `https://api.defined.net`   | Override for staging/testing.                       |
| `DN_API_TIMEOUT`        | ~30s enroll / ~10s unenroll | Overall per-command API deadline.                   |
| `DN_CLIENT_BIN_DIR`     | `/var/lib/defined/bin`      | Where `dnclient` is installed.                      |
| `DN_CLIENT_CONFIG_DIR`  | `/var/lib/defined`          | dnclient config root; `<network>/dnclient.yml` here.|
| `DN_CLIENT_SOCKET`      | derived from `<network>`    | dnclient control socket; `run` waits on it.         |
| `DN_CLIENT_VERSION`     | `latest`                    | Module pins a specific version.                     |
| `DN_LOG_LEVEL`          | `info`                      |                                                     |
| `DN_SKIP_UNENROLL`      | `false`                     | Consumed by the module's stop wiring (see §2.7).    |
| `DN_UNENROLL_ON_REBOOT` | `false`                     | Consumed by the module's stop wiring (see §2.7).    |

Precedence: live environment variables override values loaded from `--env-file`.

### 2.4 Enrollment state machine

`enroll` is modeled as a state decision over two truth sources — the **local**
`dnclient` config and the **remote** API host record — rather than a linear
sequence of side effects (addresses upstream bug B4 / finding D7):

| Local config | Remote record       | Action                                                          |
| ------------ | ------------------- | --------------------------------------------------------------- |
| present      | (not checked)       | No-op, "already enrolled". Exit 0 (or 2 w/ `--assert-changed`). |
| absent       | absent              | Create record → get code → `dnclient enroll`.                   |
| absent       | present             | **Orphan.** Fail w/ guidance, unless `--force`.                 |
| absent       | present + `--force` | DELETE stale record → create fresh → enroll.                    |

The orphan-by-default-fails decision (Q8 → C) is deliberate: silently deleting a
remote record could disrupt a host that is legitimately enrolled under the same
name elsewhere. `--force` is the explicit operator override.

**Detecting the remote record.** The API exposes **no `name` filter** on its
host-list endpoints (see the
[API reference](./research/defined-net-api-reference.md#42-list-hosts)), so the
"remote present?" check cannot be a single name query. Two mechanisms are
available:

- **Create-and-detect:** attempt `POST /v2/host-and-enrollment-code` and treat a
  `400 ERR_DUPLICATE_VALUE` on `path: name` as "remote present". One round-trip,
  needs no `hosts:list` scope — but it does not return the existing host's `id`,
  which the `--force` DELETE path requires.
- **List-and-match:** paginate `GET /v2/hosts?filter.networkID=…` and match
  `name` client-side. Costs pagination and a `hosts:list` scope, but yields the
  `id` directly.

The state machine uses list-and-match so the orphan (`absent` / `present`) and
`--force` (DELETE by `id`) cells share one lookup; create-and-detect remains a
fallback signal if the pre-check is skipped. Both endpoints are the **v2** forms
(v1 create/get are deprecated); host deletion is `DELETE /v1/hosts/{hostID}`,
which has no v2 successor.

### 2.5 The unenroll / shutdown tension

An earlier draft had `unenroll` remove the local config even when the remote
DELETE failed. That is precisely what *creates* an orphan: it breaks the
invariant that the local config and the remote record exist together, so the next
boot's `enroll` sees "remote present, local absent" and (by §2.4) fails without
`--force`. The resolution is to stop breaking the invariant rather than to paper
over the orphan with `--force`.

**Invariant:** local config and remote record are removed together. `unenroll`
removes the local config **only after** the remote DELETE succeeds (2xx) or
reports the record already absent (404, treated as idempotent success). On any
other failure within the deadline, the local config is retained, the failure is
logged, and the command exits non-zero.

Why this is correct in every scenario:

| Scenario | DELETE fails → outcome with the invariant |
| --- | --- |
| **Reboot** (host survives) | Local config retained → next boot `enroll` sees local config present → no-op; the daemon resumes with existing credentials and rejoins. No orphan. A later `unenroll` can still complete the delete. |
| **Termination** (disk destroyed) | Disk is gone regardless; retaining vs wiping local is moot. A dangling remote record is unavoidable if the API is unreachable at terminate, and a fleet-level reaper (out of scope) handles truly-abandoned records. |
| **Manual `unenroll`** | The host is still actually enrolled (remote still trusts it); retaining local config reflects the truth. Operator gets a non-zero exit + clear message and retries. |

Consequence to accept consciously: if a manual or scale-in `unenroll` cannot
reach the API, the host stays enrolled — locally and remotely consistent — rather
than half-removed. That honest state is better than a silent orphan.

With this invariant, the §2.4 orphan case (remote present, local absent) only
arises from a **failed enroll** (the B4 path: remote record created, then
`dnclient enroll` failed) — the narrow, genuinely-stale case where `--force`
recovery is clearly safe.

Messaging and recovery:

- The unenroll-failure log states plainly: remote record may persist, local config
  retained, host remains enrolled and will resume on next boot. No `--force`
  surprise, because no orphan is produced.
- The enroll-path orphan (B4) is gated behind `--force` by default to guard
  against hostname collisions. Manual recovery: `dn-tool enroll --force`, or delete
  the host in the defined.net admin.
- Optional module flag `network.forceReenroll` (default off) adds `--force` to the
  boot-time `enroll` for fleets where same-name collisions are impossible and
  automatic healing of enroll-path orphans is preferred.

### 2.6 Host ID retrieval

`unenroll` reads the remote host identifier by parsing
`$DN_CLIENT_CONFIG_DIR/<network>/dnclient.yml` as YAML and reading its
`metadata.host_id` field (Q28 → A). This replaces the upstream `grep | awk`
approach. Dependency: `gopkg.in/yaml.v3`. If the field is absent or the file is
malformed, fail clearly.

> The full set of observed `dnclient` runtime facts (ordering, socket, config
> root, `dnclient.yml` schema, shutdown exit codes, API scopes) is recorded in
> [dnclient-runtime-behavior.md](./research/dnclient-runtime-behavior.md). Two
> facts matter here, both owned by `dnclient` and observed from the current binary
> (0.9.5), not the upstream bash's assumptions. First, the config root is
> `/var/lib/defined` (the `DN_CLIENT_CONFIG_DIR` default), not `/etc/defined` —
> dn-tool passes no config path to `dnclient`, so the default tracks the daemon's
> own built-in location. Second, `host_id` is written twice: under `metadata`
> (with `org_id`/`network_id`, the API host identity) and under `host_key` (with
> the key material). unenroll reads `metadata.host_id`, the identity the remote
> DELETE needs.

### 2.7 NixOS module shape (`services.dnclient`)

The upstream units being replaced — `dnclient@.service` and `dnctl.service` — are
preserved in
[defined-systemd-units.yek.txt](./research/defined-systemd-units.yek.txt); the
fixes below are described as deltas against them.

Stays single-network (not templated). Implemented at `nix/module.nix`. The
options are grouped `network.*` (enrollment) and `client.*` (binary/config
paths), with top-level `environmentFile`/`logLevel`/`package`. Four systemd units:

1. **`dnclient-install.service`** — `Type=oneshot`, `RemainAfterExit=true`,
   `ExecStart=dn-tool install`. Pins `DN_CLIENT_VERSION` via the env file
   (install always verifies the binary against the published `.sha256`; there is
   no `DN_CLIENT_SHA256`). Carries only the non-secret env file — the downloads
   endpoint is unauthenticated, so the API key is withheld (least privilege).

2. **`dnclient@<name>.service`** — the daemon. `Type=notify`, `NotifyAccess=main`,
   `ExecStart=dnclient run -server <apiUrl> -name %i` (the URL is baked from
   `network.apiUrl`; dnclient takes it as a flag, not from env), `Restart=always`,
   `RestartSec=120`. **Fix B1:** `StartLimitIntervalSec=1200` + `StartLimitBurst=10`
   ("10 failures / 20 min"); the upstream `5`/`120`/`10` combination can never
   trip. `requires`/`after` `dnclient-install.service`.

3. **`dnclient-enroll.service`** — `Type=oneshot`, `RemainAfterExit=true`,
   `ExecStart=dn-tool enroll` (`--force` when `network.forceReenroll`). **Fix B2:**
   binds the concrete `dnclient@<name>.service` via `requires=`/`after=` (the
   upstream bare `dnclient.service` reference matched nothing).
   `EnvironmentFile = [ nonSecretEnvFile environmentFile ]`. It has **no `ExecStop`** —
   see the unenroll unit below.

4. **`dnclient-unenroll.service`** — the unenroll, run **only on poweroff/halt**.
   `Type=oneshot`, `ExecStart=dn-tool unenroll`, `EnvironmentFile = [ nonSecretEnvFile environmentFile ]`.
   **Fix D5:** `TimeoutStartSec` (60s) strictly greater than the unenroll
   `DN_API_TIMEOUT` so the DELETE always wins the race. Emitted only when
   `network.skipUnenroll = false`.

**Fix SEC4 (hardening):** the control-plane oneshots (install/enroll/unenroll)
run with `ProtectSystem=strict` (+ `ReadWritePaths=/var/lib/defined`),
`ProtectHome`, `PrivateTmp`, `NoNewPrivileges`, an **empty** capability set
(`CapabilityBoundingSet=""`), a narrow `RestrictAddressFamilies`, and
`SystemCallFilter=@system-service` — they need no privilege beyond network + the
state tree. The daemon keeps `CAP_NET_ADMIN` + `/dev/net/tun` + `AF_NETLINK`
(it programs the tun device) and is otherwise sandboxed the same way.

**Reboot-vs-poweroff — resolved.** Reboot-vs-shutdown discrimination is wholly at
the systemd layer (Q3 → A; `DN_SKIP_UNENROLL`/`DN_UNENROLL_ON_REBOOT` are inert in
the binary). The unenroll's work runs in **`ExecStart` of a dedicated unit pulled
in only by the poweroff/halt transaction**:

```ini
DefaultDependencies=no
After=network-online.target dnclient@<name>.service
Before=poweroff.target halt.target          # + reboot.target iff unenrollOnReboot
WantedBy=poweroff.target halt.target         # + reboot.target iff unenrollOnReboot
```

This is preferred over the earlier `ExecStop=dn-tool unenroll` on the enroll
wrapper, which had a real defect beyond reboot: `nixos-rebuild switch` restarts
changed units, so an `ExecStop` would unenroll/re-enroll on **every rebuild**,
churning the host and its `host_id`. With the separate unit:

| Event | Unenroll runs? |
| --- | --- |
| Normal operation, `systemctl restart`, `nixos-rebuild switch` | No — the unit is never activated. |
| Reboot (default) | No — not `WantedBy` `reboot.target`. |
| Reboot (`unenrollOnReboot=true`) | Yes. |
| Poweroff / halt | Yes (unless `skipUnenroll=true`, which omits the unit). |

The `Before=`/`After=network-online.target` ordering aims to run the DELETE while
the network is still up. If the network is already torn down at that point, the
call fails — and the §2.5 invariant makes that safe: local config is retained, the
host stays consistently enrolled, and resumes on the next boot (or a fleet reaper
clears a truly-dead host). Validated by the `nixosTest` (§2.11, `nix/tests/module.nix`):
boot ordering, enroll-on-boot, daemon-up, the unit's poweroff/halt-not-reboot
wiring, and that a reboot leaves the host enrolled.

### 2.8 Output contract

- **stdout:** one JSON object per command result, e.g.
  `{"action":"enroll","changed":true,"hostId":"host-…","network":"defined"}`.
- **stderr:** one JSON object per error.
- API key and enrollment codes are never logged (fixes upstream SEC5). Enrollment
  codes are used in-memory only.

### 2.9 Internal structure (tentative)

```txt
dn-tool/
  cmd/
    dn-tool/
      main.go                 # urfave/cli wiring, flag/env binding
  internal/
    config/               # env + --env-file loading, precedence, validation
    api/                  # defined.net REST client (status checks, retry/backoff)
    dnclient/             # download/verify/install + subprocess invocation (interface)
    enroll/               # the state machine (§2.4)
    output/               # JSON result writer + slog setup
  go.mod
  go.sum
  flake.nix
  README.md
```

- The `dnclient/` package exposes an interface for the subprocess
  (`Enroll(code) error`, `Run(...) error`) so tests mock it (Q24 → A).
- `api/` is tested against `httptest.Server` covering: success, 4xx (no retry),
  5xx/429 (retry with backoff), timeout, malformed body.

### 2.10 CLI / libraries

- CLI framework: `urfave/cli` (Q22 → C).
- Logging: `log/slog`, JSON handler default (Q23 → A, JSON everywhere).
- YAML: `gopkg.in/yaml.v3` (host-id parsing only).
- HTTP/retry: `hashicorp/go-retryablehttp` wrapping stdlib `net/http`. Its
  default `CheckRetry`/`Backoff` already encode the Requirement 9 policy — retry
  on 5xx/429/connection errors, never on 4xx, exponential backoff — and it honors
  `context.Context` for the `DN_API_TIMEOUT` deadline. Tune `RetryMax`,
  `RetryWaitMin`, `RetryWaitMax`; route its logger through `slog`. Use
  `client.StandardClient()` to hand a plain `*http.Client` to the typed API
  layer. (Pulls in `go-cleanhttp` + `go-hclog`; acceptable.)

### 2.11 Testing

- Unit tests with mocked API (`httptest`) and mocked `dnclient` subprocess
  (interface). Cover the enroll state machine exhaustively (all four cells of the
  §2.4 table, plus `--force`), config precedence, arch mapping, checksum
  verification against the published `.sha256` (match / mismatch / fetch
  failure), and exit-code semantics.
- A NixOS VM test (`nixosTest`) for module integration to validate service
  ordering, enroll-on-boot, and unenroll-on-stop — including the reboot-vs-poweroff
  spike from §2.7.

### 2.12 Build / migration order

1. Scaffold `dn-tool` repo: module, flake, CLI skeleton, CI.
2. Port `install` (download + verify) with tests.
3. Port the API client with retry + status checks.
4. Implement the enroll state machine + `dnclient enroll` invocation.
5. Implement `unenroll` (YAML host-id, bounded deadline).
6. Implement `run` (signal handling) and `write-config`.
7. Ship the NixOS integration in this flake (`nix/module.nix`, `nix/overlay.nix`,
   `nix/package.nix`), applying the B1/B2/D5/SEC4 unit fixes; strip the old
   `dnctl` package and `dnclient` module from `nix-packages`.
8. NixOS VM test (`nix/tests/module.nix`); resolve the reboot/poweroff wiring
   (done: the poweroff-only unenroll unit, §2.7).
