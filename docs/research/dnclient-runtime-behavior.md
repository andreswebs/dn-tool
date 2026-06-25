# dnclient runtime behavior (observed)

Source: empirical — running the proprietary `dnclient` binary (version **0.9.5**)
under the container lifecycle (`dn-tool run`) and inspecting what it does on disk
and over its control socket.

`dnclient` is closed-source, so its runtime contract is not in any spec we
control. The upstream `dnctl` bash and our own first-cut design encoded several
assumptions about it that turned out to be **wrong for this version**. This
document records what the binary actually does, so the next person does not have
to rediscover it by reading shutdown error messages. Each fact notes how it was
observed and what in `dn-tool` depends on it.

> Caveat: these are observations of one version (0.9.5). `dnclient`'s socket
> path, config location, and `dnclient.yml` schema are all owned by `dnclient`,
> not by any API or by `dn-tool`. Treat them as version-specific and re-verify
> after a `dnclient` upgrade. `dn-tool` keeps the version-sensitive values
> behind config knobs (`DN_CLIENT_CONFIG_DIR`, `DN_CLIENT_SOCKET`) precisely so a
> drift can be corrected without a code change.

---

## 1. `dnclient enroll` requires the daemon to be running first

`dnclient enroll -name <network> -code <code>` does **not** enroll standalone. It
is a client of the running `dnclient run` daemon: it connects to the daemon's
unix control socket and hands it the code; the daemon then fetches the cert and
writes the config. If the daemon is not up, enroll fails immediately with:

```text
dial unix /var/run/defined/dnclient.<network>.sock: connect: no such file or directory
```

**Observed:** a fresh `dn-tool run` that ran `install → enroll → dnclient run`
(enroll before the daemon) failed with the dial error above; reordering to start
the daemon first fixed it.

**Corroborated by:** the upstream systemd units order the enroll wrapper
`After=dnclient@<name>.service` and the `Type=notify` daemon, and the upstream
`dnctl enroll` explicitly checks the daemon is `is-active` first
([analysis §"dnctl.service"](./quickvm-defined-systemd-units-analysis.md), B2).
Also the official defined.net flow: install (which *starts* the service), then
`sudo dnclient enroll -code …`.

**`dn-tool` depends on it:** the `run` lifecycle is ordered
`install → start daemon → wait for socket → enroll → block on daemon`
(design §2.2 / Req 5; `internal/run`). The systemd module path already had the
correct ordering (daemon is its own unit, enroll wrapper ordered after it).

## 2. The daemon's readiness signal is its control socket

The daemon (`dnclient run`) is `Type=notify` under systemd (`sd_notify(READY=1)`),
but with no systemd present (the container path) the observable readiness signal
is the **control socket file appearing**:

```text
/var/run/defined/dnclient.<network>.sock   # srwx------, created once the daemon is listening
```

The daemon starts and opens this socket **while still unenrolled** — it logs
`"Config file does not yet exist, awaiting enrollment"` and waits to be enrolled
over the socket. So "daemon up" and "host enrolled" are independent states, and
polling for the socket is a sound readiness gate for the enroll step.

**`dn-tool` depends on it:** `dnclient.WaitForSocket` polls this path; the path is
`DN_CLIENT_SOCKET` (default `/var/run/defined/dnclient.<network>.sock`, derived
from the network name since `dn-tool` passes no socket flag to `dnclient`).

## 3. Config root is `/var/lib/defined`, not `/etc/defined`

This `dnclient` writes its per-network state under **`/var/lib/defined/<network>/`**:

```text
/var/lib/defined/<network>/config.yml     # nebula config (pki, firewall, …)
/var/lib/defined/<network>/dnclient.yml   # enrollment state, incl. host_id
```

The upstream bash and our first design assumed `/etc/defined/<network>/…` (the
upstream even commented "dnclient expects its config to be in /etc/defined!").
For 0.9.5 that is false. `dn-tool` passes **no** config-dir flag to `dnclient`, so
the location is `dnclient`'s own built-in default; `dn-tool`'s view of it
(`DN_CLIENT_CONFIG_DIR`) must simply match.

**Observed:** after a successful enroll, the daemon refreshed certs "from disk"
but `unenroll` failed with `/etc/defined/<network>/dnclient.yml: host does not
appear to be enrolled` — the file was at `/var/lib/defined/<network>/` instead.

**`dn-tool` depends on it:** `DN_CLIENT_CONFIG_DIR` defaults to `/var/lib/defined`;
both `dnstate.ConfigExists` (enroll's already-enrolled probe) and
`dnstate.ReadHostID` (unenroll) read from it.

## 4. `dnclient.yml` nests `host_id` under `metadata`

`host_id` is **not** a top-level key. It appears in two places:

```yaml
host_key:
  host_ed_key: …
  host_p256_key: …
  host_id: host-…        # copy alongside the key material
  counter: …
  trusted_keys: …
metadata:
  org_id: …
  org_name: …
  network_id: …
  network_name: …
  host_id: host-…        # the API host identity — what the remote DELETE needs
  host_name: …
  host_ip_addresses: …
```

`unenroll`'s remote `DELETE /v1/hosts/{id}` needs the API host identity, which is
**`metadata.host_id`** (the copy sitting with `org_id`/`network_id`). The
`host_key.host_id` copy held the same value in observation, but `metadata` is the
semantically correct, intentional location.

**Observed:** with the path fixed (§3), `unenroll` then failed with
`missing host_id field` because the parser decoded `host_id` at the root; the key
dump showed it indented under `metadata` (and `host_key`).

**`dn-tool` depends on it:** `dnstate.ReadHostID` decodes `metadata.host_id`
(design §2.6).

## 5. A signal-killed daemon during shutdown is not a failure

On `docker compose down` / SIGTERM, the foreground daemon is killed and
`dnclient run` returns `signal: killed` (an `*exec.ExitError` with
`ExitCode() == -1`). This is the **expected** way the daemon stops when `dn-tool`
itself initiates shutdown — it must not be reported as a failure.

`dn-tool` distinguishes the two cases by the run context's cancellation state:

- **ctx cancelled** (SIGTERM/SIGINT initiated the stop) → the daemon's signal-kill
  is expected; exit status is governed by unenroll alone (clean unenroll → exit 0).
- **ctx not cancelled** (daemon died on its own — crash, bad config, OOM) →
  propagate the daemon's exit code (Req 5 / dt-r2ks).

**Observed:** a clean enroll → run → SIGTERM → successful unenroll cycle exited 1
because the signal-kill mapped to `CodeError`; gating on `ctx.Err()` makes the
graceful path exit 0. (The unit-test mock had masked this by returning `nil` on
cancel, where the real binary returns a signal-kill error.)

**`dn-tool` depends on it:** `internal/run.runAndUnenroll` gates daemon-error
propagation on `ctx.Err() == nil` (design Req 5 acceptance criteria).

## 6. API key scopes required

`dn-tool`'s enroll runs a remote orphan pre-check (`ListHosts`,
`GET /v2/hosts?filter.networkID=…`) on **every** enroll, not just under `--force`
([enroll/state.go](../../src/internal/enroll/state.go)). So the key needs the
list/read scope in addition to the obvious ones:

| Scope | Used by |
| ----- | ------- |
| `hosts:create` | create the host record (`POST /v2/host-and-enrollment-code`) |
| `hosts:enroll` | obtain the single-use enrollment code (same call) |
| `hosts:list` (or `hosts:read`) | orphan pre-check lists hosts on every enroll |
| `hosts:delete` | `unenroll`'s `DELETE /v1/hosts/{id}`; `--force` orphan cleanup |

`/v1/downloads` (install) needs no scope. The upstream README listed only
create/delete/enroll because its bash matched existing hosts by a different route.
See also the [API reference §"Implementation notes"](./defined-net-api-reference.md).
