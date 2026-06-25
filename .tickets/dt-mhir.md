---
id: dt-mhir
status: closed
deps: [dt-ecf9]
links: []
created: 2026-06-06T14:14:57Z
type: task
priority: 0
assignee: Andre Silva
parent: dt-uzx6
tags: [config, env]
---
# Config struct: env loading + defaults + DN_API_URL

Define the `Config` struct and a loader that reads every `DN_*` variable from the process environment and applies the §2.3 defaults, including the `DN_API_URL` override. This is the type imported by `api`, `dnclient`, `enroll`, `unenroll`, and `write-config`, so get the field set and defaults right.

## Public interface

```go
// internal/config
type Config struct {
    APIKey        string   // DN_API_KEY (secret)
    NetworkID     string   // DN_NETWORK_ID
    RoleID        string   // DN_ROLE_ID
    NetworkName   string   // DN_NETWORK_NAME, default "defined"
    Hostname      string   // DN_HOSTNAME, default system hostname
    IPAddress     string   // DN_IP_ADDRESS, optional
    Tags          []string // DN_TAGS (JSON array) — parsing in CFG.types
    IsLighthouse  bool     // DN_IS_LIGHTHOUSE
    IsRelay       bool     // DN_IS_RELAY
    StaticAddrs   []string // DN_STATIC_ADDRESSES (JSON array) — CFG.types
    ListenPort    int      // DN_LISTEN_PORT, unset -> 0
    APIURL        string   // DN_API_URL, default "https://api.defined.net"
    APITimeout    time.Duration // DN_API_TIMEOUT
    ClientBinDir  string   // DN_CLIENT_BIN_DIR, default "/var/lib/defined/bin"
    ClientVersion string   // DN_CLIENT_VERSION, default "latest"
    LogLevel      string   // DN_LOG_LEVEL, default "info"
    SkipUnenroll      bool // DN_SKIP_UNENROLL  — module-only, inert here
    UnenrollOnReboot  bool // DN_UNENROLL_ON_REBOOT — module-only, inert here
}

func Load(getenv func(string) string) (*Config, error)  // inject getenv for testability
```

Inject the environment lookup (e.g. `func(string) string`) so tests don't touch real `os.Getenv`. Scope this task to **env-only** loading + defaults; `--env-file` (CFG.envfile / dt-kiqk), precedence (CFG.precedence / dt-9x3y), and rich typed parsing/validation (CFG.types / dt-toqi) layer on top.

## Behaviors (TDD order)

1. **Defaults applied when env is empty** — table test per defaulted field (`NetworkName="defined"`, `APIURL="https://api.defined.net"`, `ClientBinDir="/var/lib/defined/bin"`, `ClientVersion="latest"`, `LogLevel="info"`, `ListenPort=0`).
2. **Env overrides default** — setting a var changes the field.
3. **`DN_HOSTNAME` falls back to system hostname** — unset → `os.Hostname()` (inject for testability).
4. **`DN_API_URL` override honored** — custom value used verbatim.
5. **Module-only vars load but are inert** — `SkipUnenroll`/`UnenrollOnReboot` populate the struct; no behavior branches on them here.

## Test strategy

Table-driven over `(env map) -> expected Config`. Inject `getenv`/hostname funcs. Assert through the returned `*Config` only.

## Acceptance

- Every `DN_*` in §2.3 maps to a field with the documented default.
- `Load` is pure w.r.t. injected env; no global state.

## References

- Design: [Req 6](../docs/dn-tool-design.md#6-configuration-interface), [§2.3 Configuration variables](../docs/dn-tool-design.md#23-configuration-variables).

Parent epic: [dt-uzx6](dt-uzx6.md).

## Notes

**2026-06-06T18:06:35Z**

Implemented internal/config: Config struct + Load(getenv) covering all 18 §2.3 DN_* vars with defaults (NetworkName=defined, APIURL=https://api.defined.net, ClientBinDir=/var/lib/defined/bin, ClientVersion=latest, LogLevel=info). Public Load(getenv) delegates to unexported load(getenv, hostname) so the system-hostname fallback is testable without touching the host (matches the one-param signature in the ticket). Bools/port/duration are parsed and return wrapped errors on malformed input (returns nil cfg + err). Tags/StaticAddrs fields exist but JSON-array parsing is deferred to dt-toqi; APITimeout defaults to 0 (no single default since enroll~30s/unenroll~10s differ per-command, so the command layer picks). No global state. Table-driven tests: defaults, env overrides, hostname fallback + error propagation, malformed-value errors, public entrypoint. make build green.
