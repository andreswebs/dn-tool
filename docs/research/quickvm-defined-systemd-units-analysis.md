# quickvm/defined-systemd-units — Analysis

Source: <https://github.com/quickvm/defined-systemd-units>

This document describes the behavior of the original implementation and records findings from a thorough review: bugs, misconfigurations, design gaps, and suggestions for improvement. This is the basis for the Go rewrite of `dnctl`.

---

## Repository Structure

```txt
bin/dnctl              — the main control script
units/dnclient@.service — template unit for the dnclient nebula process
units/dnctl.service    — oneshot enrollment/unenrollment service
install                — imperative bootstrap script
```

---

## Behavioral Description

### `install`

A one-shot bootstrap script meant to be run from the repo root as root. It:

1. Detects CPU architecture (`arch` command) and maps to the dnclient download arch string.
2. Fetches the dnclient download URL from `https://api.defined.net/v1/downloads` using `jq` to select by arch and version.
3. Creates `/etc/defined/` if missing.
4. Downloads `dnclient` to `/usr/local/bin/dnclient` (skips if already present).
5. Uses `envsubst` to substitute `${BIN_DIR}` into the unit template files and installs them to `/etc/systemd/system/`.
6. Installs `bin/dnctl` to `/usr/local/bin/dnctl`.
7. Runs `systemctl daemon-reload`.

After install the operator must create `/etc/defined/dnctl`, then run `dnctl enable` and `dnctl start`.

### `dnclient@.service` (template unit)

A systemd template unit. Instantiated as `dnclient@<network-name>.service` where `<network-name>` becomes `%i` inside the unit.

| Directive                   | Value                                                   | Purpose                         |
| --------------------------- | ------------------------------------------------------- | ------------------------------- |
| `Type`                      | `notify`                                                | dnclient uses `sd_notify`       |
| `ConditionFileIsExecutable` | `${BIN_DIR}/dnclient`                                   | Silently skips if binary absent |
| `After`                     | `network-online.target dnclient-install.service`        | Ordering                        |
| `Wants`                     | `network-online.target`                                 | Soft network dep                |
| `ExecStart`                 | `dnclient run -server https://api.defined.net -name %i` | Run nebula client               |
| `Restart`                   | `always`                                                | Always restart on failure       |
| `StartLimitInterval`        | `5`                                                     | Rate limit window (seconds)     |
| `StartLimitBurst`           | `10`                                                    | Restarts allowed in window      |
| `RestartSec`                | `120`                                                   | Delay between restarts          |
| `WantedBy`                  | `multi-user.target`                                     | Enable target                   |

### `dnctl.service`

A `Type=oneshot RemainAfterExit=yes` unit that wraps enrollment/unenrollment as a service lifecycle.

| Directive         | Value                                    |
| ----------------- | ---------------------------------------- |
| `EnvironmentFile` | `/etc/defined/dnctl`                     |
| `ExecStart`       | `dnctl enroll`                           |
| `ExecReload`      | `dnctl reenroll`                         |
| `ExecStop`        | `dnctl unenroll`                         |
| `After`           | `network-online.target dnclient.service` |
| `Wants`           | `network-online.target dnclient.service` |

The `RemainAfterExit=yes` means the service reports "active" after `ExecStart` exits 0, and `ExecStop` runs when the unit is stopped (or during system shutdown).

### `bin/dnctl`

The main control script. Reads configuration from `/etc/defined/dnctl` (sourced as shell) and environment variables. All operations run as root.

**Commands:**

| Command        | Behavior                                                |
| -------------- | ------------------------------------------------------- |
| `install`      | Download dnclient binary to `${BIN_DIR}`                |
| `start`        | `systemctl start dnclient@${DN_NETWORK_NAME}.service`   |
| `stop`         | `systemctl stop dnclient@${DN_NETWORK_NAME}.service`    |
| `restart`      | `systemctl restart dnclient@${DN_NETWORK_NAME}.service` |
| `enable`       | `systemctl enable dnclient@${DN_NETWORK_NAME}.service`  |
| `write_config` | Write env vars to `/etc/defined/dnctl`                  |
| `enroll`       | Create host in defined.net API + run `dnclient enroll`  |
| `unenroll`     | Delete host from defined.net API + remove local config  |
| `reenroll`     | `unenroll` then `enroll`                                |

**Enrollment flow:**

1. Validate required vars (`DN_API_KEY`, `DN_NETWORK_ID`, `DN_ROLE_ID`).
2. Validate lighthouse/relay mutual exclusion and port requirements.
3. If `DN_MIRROR_IP=true`: query the default-route IP, take its last octet, fetch the network CIDR from the API, construct `DN_IP_ADDRESS` and persist it to `/etc/defined/dnctl`.
4. Check if local config (`/etc/defined/${DN_NETWORK_NAME}/dnclient.yml`) exists — if so, skip (already enrolled locally).
5. Check if `dnclient@${DN_NETWORK_NAME}.service` is active — if not, exit 1.
6. Determine enrollment hostname: `DN_HOSTNAME` or `hostname`, optionally append cloud instance ID.
7. Query defined.net API for existing host by name — if found, skip (already enrolled remotely).
8. Build JSON enrollment payload with `jq --null-input`.
9. POST to `https://api.defined.net/v1/host-and-enrollment-code`.
10. Extract enrollment code from response.
11. Call `dnclient enroll -name "${DN_NETWORK_NAME}" -code "${ENROLLMENT_CODE}"`.

**Unenrollment flow:**

1. Validate `DN_API_KEY`.
2. If `DN_SKIP_UNENROLL=true`, exit 0.
3. If `is_rebooting` and `DN_UNENROLL_ON_REBOOT != true`, exit 0 (preserve host across reboots).
4. Read `host_id` from `/etc/defined/${DN_NETWORK_NAME}/dnclient.yml`.
5. DELETE `https://api.defined.net/v1/hosts/${host_id}`.
6. If `is_shutting_down`: just `rm -rf /etc/defined/${DN_NETWORK_NAME}`.
7. Otherwise: `systemctl stop`, `rm -rf`, `systemctl start`.

**Shutdown/reboot detection:**

- `is_rebooting`: checks `systemctl list-jobs` for `reboot.target`; falls back to `/run/systemd/shutdown/scheduled` file.
- `is_shutting_down`: checks `systemctl is-system-running` for `stopping`; also scans job queue for shutdown/poweroff/halt/reboot/final targets.

---

## Findings

### Bugs

#### B1 — `dnclient@.service` restart rate-limiter is impossible to trigger

`StartLimitInterval=5` (5-second window) combined with `RestartSec=120` (120-second restart delay) means you cannot get 10 failures within 5 seconds — restarts are spaced 120 seconds apart. The rate limiter never fires. `Restart=always` with this config causes infinite restarts regardless of failure count.

**Intent** appears to be "give up after 10 failures." The correct config for "10 failures within 20 minutes":

```ini
StartLimitIntervalSec=1200
StartLimitBurst=10
RestartSec=120
```

#### B2 — `dnctl.service` dependency references non-existent unit

```ini
After=network-online.target dnclient.service
Wants=network-online.target dnclient.service
```

The actual running unit is `dnclient@<name>.service` (a template instance). `dnclient.service` (bare, non-instantiated) does not exist. systemd silently ignores ordering and want dependencies on non-existent units.

**Consequence:** `dnctl.service` can start before any `dnclient@*.service` is active. The `enroll` function checks `systemctl is-active "$SERVICE_NAME"` and exits 1 if not running, so enrollment will fail on a fresh boot unless the operator manually starts `dnclient@<name>.service` first.

The correct dependency cannot be expressed as a static string without knowing the network name at unit-install time. One solution: make `dnctl.service` itself a template (`dnctl@.service`) so it can reference `dnclient@%i.service`.

#### B3 — `unenroll` proceeds on API DELETE failure

The `curl -X DELETE` call has no HTTP status checking:

```bash
curl -H user-agent:defined-systemd-units -sSL -X DELETE "https://api.defined.net/v1/hosts/${DN_HOST_ID}" \
    -H "Accept: application/json" \
    -H "Authorization: Bearer ${DN_API_KEY}"
```

If the API returns 401, 404, 500, or if the network is unreachable, the script continues and deletes the local config directory. The host then exists in defined.net with no local config. On the next `enroll` run, the remote check finds the host by name and skips enrollment — the machine can never re-enroll without manual API cleanup.

#### B4 — Enrollment orphan on `dnclient enroll` failure

If the API call to `POST /v1/host-and-enrollment-code` succeeds but `dnclient enroll` subsequently fails (crash, disk full, network drop), the host exists in defined.net with no local config. On the next run:

- Local check: no config → proceed
- Remote check: host found by name → "already enrolled, skipping"

The machine is permanently stuck. There is no recovery path without manually deleting the host from defined.net.

#### B5 — README incorrectly states `dnclient.service` has `Wants=dnctl.service`

> "You only need to start `dnclient.service` since it has a systemd.unit `Wants=dnctl.service` directive it will cause `dnctl.service` to start afterwards."

The actual `dnclient@.service` unit has no such directive. `Wants=network-online.target` is all that's there. Both units must be explicitly enabled and started.

#### B6 — `reenroll` with `DN_SKIP_UNENROLL=true` silently does nothing

`reenroll` calls `unenroll` then `enroll`. If `DN_SKIP_UNENROLL=true`, `unenroll` exits 0 immediately. Then `enroll` finds the existing host and also exits 0. The overall effect is a no-op with exit 0 and no clear indication that reenrollment was skipped.

---

### Shell Scripting Issues

#### S1 — `set -e` only; missing `set -u` and `set -o pipefail`

Without `set -u`, referencing an unset variable silently expands to empty string. Without `set -o pipefail`, a pipeline failure in the middle (e.g., `curl` returning malformed data) is invisible if the last command exits 0.

Example: `curl -sL ... | jq -r '.data.cidr'` — if `curl` exits 1 but `jq` processes empty input and exits 0, the pipeline succeeds and `DN_NETWORK_CIDR` is empty. The script continues silently.

#### S2 — `: ${VAR:-}` does not set defaults

The lines at the top of the script:

```bash
: ${DN_API_KEY:-}
: ${DN_NETWORK_ID:-}
```

The `:-` operator returns the default if unset/empty but does **not** assign it. These lines are no-ops (they would only suppress `set -u` errors, which aren't enabled). Only lines using `:=` (e.g., `: ${DN_SKIP_UNENROLL:="false"}`) actually set defaults.

#### S3 — Unquoted variable expansions

Multiple locations fail to quote variables, enabling word splitting and glob expansion:

```bash
echo $DEFAULT_ROUTE_IP | cut -d. -f4           # word splitting
curl -sSL ${DN_DOWNLOAD_URL} -o ${BIN_DIR}/dnclient  # both unquoted
export OSARCH="$(echo ${OS_TYPE}|sed ...)"     # OS_TYPE unquoted in subshell
if [[ -f ${BIN_DIR}/dnclient ]]; then          # unquoted
```

#### S4 — `arch` instead of `uname -m`

`arch` is less universally available than `uname -m` and may differ in multiarch/container environments. `uname -m` is the POSIX-blessed approach and what the install script already effectively calls.

#### S5 — Global variable side-effects inside `install_dnclient` function

`BIN_DIR`, `DEFINEDPATH`, `DN_VERSION`, `OS_ARCH`, `OS_TYPE`, `ARCH`, `OSARCH`, `DN_DOWNLOAD_URL` are all set without `local` inside the function. They pollute global scope on every call.

#### S6 — Redundant `-v` check in `enroll`

```bash
[[ -v DN_API_KEY ]] || { echo "DN_API_KEY is unset!"; exit 1; }
[[ ${DN_API_KEY} ]] || { echo "DN_API_KEY is empty!"; exit 1; }
```

After sourcing `/etc/defined/dnctl` and running the `: ${VAR:-}` lines at the top, all variables are technically "set" (to whatever the config says, or empty). The `-v` check always passes. Only the non-empty check is meaningful.

---

### Security Issues

#### SEC1 — No integrity verification of the downloaded binary

`dnclient` is downloaded from a URL retrieved from the defined.net API. The binary is made executable with no checksum verification. While the API is fetched over TLS (MITM requires a valid cert), there is no content hash check. A compromised CDN or API could serve a malicious binary.

#### SEC2 — `write_config` permission race

```bash
cat << EOF > /etc/defined/dnctl
DN_API_KEY=...
...
EOF
chmod 0640 /etc/defined/dnctl
```

The file is created first with umask-derived permissions (often `0644`, world-readable), then `chmod` is applied. There is a brief window where the file containing the API key is readable by non-root users. The correct approach is to create the file with restricted permissions atomically, e.g. using `install -m 0640` or `(umask 0137; cat > ...)`.

#### SEC3 — Config file sourced as shell code

`source /etc/defined/dnctl` executes the file's contents as shell code. If an attacker with root write access (or a misconfigured sudoer) can modify the file, they gain arbitrary code execution in the context of subsequent `dnctl` invocations. The `0640 root:root` permissions mitigate this for non-root users, but it is worth being aware of as a design constraint.

#### SEC4 — No systemd security hardening

Neither service unit uses any systemd sandboxing directives:

- No `ProtectSystem`, `ProtectHome`, `PrivateTmp`
- No `NoNewPrivileges`
- No `CapabilityBoundingSet` / `AmbientCapabilities`
- No `RestrictAddressFamilies`, `SystemCallFilter`

`dnclient` needs `CAP_NET_ADMIN` (TUN device). `dnctl` needs root for config writes and `systemctl` calls. Both run with full root capability. Hardening is possible for `dnclient` at minimum.

#### SEC5 — Sensitive data written to journal

```bash
echo "Enrollment Data: ${ENROLLMENT_DATA}"
echo "Enrollment Response: ${ENROLLMENT_RESPONSE}"
echo "Enrollment Code: ${ENROLLMENT_CODE}"
```

The enrollment response contains the enrollment code, host ID, network ID, and role ID. These are written to stdout (captured by journald). Enrollment codes are single-use but this is still excessive journal exposure.

---

### Design Issues

#### D1 — `install` has dead code after `mkdir -p`

```bash
mkdir -p "${DEFINEDPATH}"
[[ -d ${DEFINEDPATH} ]] || { echo "The ${DEFINEDPATH} directory does not exist!"; exit 1; }
```

If `mkdir -p` succeeds, the directory exists — the check always passes. If `mkdir -p` fails (permission denied), `set -e` exits the script before the check is reached. The check is unreachable dead code.

#### D2 — `envsubst` in `install` substitutes all env vars

```bash
install -m 0644 <(envsubst < ${UNIT_FILES}/dnclient@.service) ${UNIT_DIR}/dnclient@.service
```

`envsubst` without a variable list substitutes every `${VAR}` found in the file against the entire environment. Any future addition of `${...}` expressions to the unit templates (e.g., for comments or other directives) would be silently mangled. The safe form is:

```bash
envsubst '$BIN_DIR' < ${UNIT_FILES}/dnclient@.service
```

#### D3 — `After=dnclient-install.service` is vestigial

`dnclient@.service` has `After=network-online.target dnclient-install.service`. There is no `dnclient-install.service` in this repo or referenced anywhere. systemd silently ignores ordering on non-existent units. This is a dead artifact from a prior design.

#### D4 — No API error handling

None of the `curl` calls that hit the defined.net API check the HTTP response status. Errors from the API (401 unauthorized, 422 validation error, 500 server error) are silently consumed by `jq` as whatever the error body parses to. The script should use `curl --fail` or check the response for an error field before proceeding.

#### D5 — No `TimeoutStopSec` on `dnctl.service`

`ExecStop` makes a network API call. If defined.net is unreachable, `curl` blocks until its default timeout. Without `TimeoutStopSec`, systemd uses the system default (typically 90 seconds) before sending SIGKILL. For reliable shutdown sequences, an explicit `TimeoutStopSec` and `--max-time` in the curl call is recommended.

#### D6 — Multi-network support requires multiple installs

`dnctl.service` is not a template. It hard-codes one `DN_NETWORK_NAME`. Managing multiple Nebula networks on the same host requires duplicating the entire service configuration. A `dnctl@.service` template would make this composable.

#### D7 — Non-atomic state: enrollment can desync

There are two sources of enrollment truth: the local dnclient config at `/etc/defined/<name>/dnclient.yml` and the defined.net API. They are checked and updated separately in sequence with no rollback. Any failure mid-enrollment leaves them inconsistent without a reconciliation path.

---

## Summary Table

| ID   | Severity | Category | Short Description                                                                      |
| ---- | -------- | -------- | -------------------------------------------------------------------------------------- |
| B1   | High     | Bug      | Restart rate limiter impossible to trigger (`StartLimitInterval=5` + `RestartSec=120`) |
| B2   | High     | Bug      | `dnctl.service` depends on non-existent `dnclient.service` unit                        |
| B3   | High     | Bug      | `unenroll` deletes local config even when API DELETE fails                             |
| B4   | Medium   | Bug      | Enrollment orphan when `dnclient enroll` fails after API call succeeds                 |
| B5   | Low      | Bug      | README claims `dnclient.service` has `Wants=dnctl.service` (it doesn't)                |
| B6   | Low      | Bug      | `reenroll` silently does nothing when `DN_SKIP_UNENROLL=true`                          |
| S1   | High     | Shell    | `set -e` only; missing `set -u` and `set -o pipefail`                                  |
| S2   | Medium   | Shell    | `: ${VAR:-}` does not assign defaults (needs `:=`)                                     |
| S3   | Medium   | Shell    | Unquoted variable expansions throughout                                                |
| S4   | Low      | Shell    | `arch` instead of `uname -m`                                                           |
| S5   | Low      | Shell    | `install_dnclient` sets global vars without `local`                                    |
| S6   | Low      | Shell    | Redundant `-v` check before non-empty check in `enroll`                                |
| SEC1 | Medium   | Security | Downloaded binary has no checksum verification                                         |
| SEC2 | Medium   | Security | `write_config` permission race (0644 → 0640)                                           |
| SEC3 | Low      | Security | Config file sourced as shell code                                                      |
| SEC4 | Low      | Security | No systemd unit hardening                                                              |
| SEC5 | Low      | Security | Enrollment response (including code) written to journal                                |
| D1   | Low      | Design   | Dead code after `mkdir -p` in `install`                                                |
| D2   | Low      | Design   | `envsubst` without variable list is fragile                                            |
| D3   | Low      | Design   | `After=dnclient-install.service` is vestigial dead reference                           |
| D4   | High     | Design   | No API HTTP error handling on any `curl` call                                          |
| D5   | Low      | Design   | No `TimeoutStopSec` on `dnctl.service`                                                 |
| D6   | Low      | Design   | `dnctl.service` not a template; multi-network requires duplication                     |
| D7   | Medium   | Design   | Non-atomic enrollment state; no reconciliation on partial failure                      |

---

## Notes for the Go Rewrite

- B3 and D4 are the most important to fix: all API calls should check HTTP status and surface clear errors before mutating local state.
- B4 (orphan enrollment) requires a deliberate recovery strategy — either re-attempt the `dnclient enroll` if the host already exists remotely with no local config, or delete the remote host and re-create it.
- B1 and B2 are unit-file issues, not script logic — they need addressing in the NixOS module that replaces the unit templates.
- D7 motivates treating enrollment as a state machine rather than a linear sequence of side effects.
- SEC5 argues for structured logging where sensitive fields are either omitted or log-level-gated, not unconditionally printed.
