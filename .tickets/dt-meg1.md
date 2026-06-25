---
id: dt-meg1
status: closed
deps: [dt-druh, dt-ccmn, dt-icq8]
links: []
created: 2026-06-06T14:14:58Z
type: task
priority: 2
assignee: Andre Silva
parent: dt-cmdg
tags: [config, write-config, security]
---
# write-config 0600-at-creation

Implement the `write-config` command: write the serialized config ([WC.serialize](dt-druh.md)) to the target path, creating the file with owner-only `0600` permissions **from the moment of creation** — never create broader then `chmod`. Closes upstream **SEC2** (the bash `write_config` created the API-key file with umask perms ~0644 then `chmod 0640`, leaving a world-readable window).

## Public interface

```go
// internal/config or cmd
func WriteConfigFile(path string, cfg *config.Config) (output.Result, error)
//   os.OpenFile(path, O_CREATE|O_WRONLY|O_TRUNC, 0600) — mode at creation, no chmod-after
//   account for umask; verify resulting mode
```

**Decision (per plan):** `DN_API_KEY` **is** written to the file, guarded by `0600`. This is documented behavior — the file is a secret artifact; the 0600 guarantee is the protection. (If we later want to omit it, that's a follow-up.)

## Behaviors (TDD order)

1. **File created with 0600** — after `WriteConfigFile`, `os.Stat(path).Mode().Perm() == 0600`, independent of the process umask.
2. **No widen-then-restrict** — there is no moment the file exists with broader perms (structural: single `OpenFile` with the mode; test asserts final mode and that creation used the mode).
3. **Content round-trips** — the written file parses back via the loader to the same config.
4. **API key present but file is 0600** — the secret is in the file; perms protect it.
5. **Result/exit** — emits `Result{Action:"write-config", Changed:true}`.

## Test strategy

`t.TempDir()`; set a non-restrictive umask in the test, write, and assert the final mode is exactly `0600` (proving the mode was applied at creation, not widened). Round-trip the content through the loader.

## Acceptance

- File is `0600` from creation, umask-independent; never widened-then-restricted; content round-trips; API-key handling documented.

## References

- Design: [Req 10](../docs/dn-tool-design.md#10-configuration-file-writer), [§2.2 Command surface](../docs/dn-tool-design.md#22-command-surface).
- Closes upstream: [**SEC2**](../docs/research/quickvm-defined-systemd-units-analysis.md#summary-table).
- Security invariant: [docs/agents/code-conventions.md](../docs/agents/code-conventions.md) — "config files are 0600 from creation".

Parent epic: [dt-cmdg](dt-cmdg.md).

## Notes

**2026-06-06T22:34:41Z**

Implemented write-config. Core file-writer lives in internal/config.WriteConfigFile(path, cfg) returning error (NOT output.Result) — kept the config->output edge out, so config stays free of the output dependency; the cmd layer builds the Result. Single os.OpenFile(O_CREATE|O_WRONLY|O_TRUNC, 0o600) — mode applied at creation, no chmod-after (SEC2 closed structurally). Note: 0600 is umask-independent because it has no group/other bits and umask only clears bits; tests still set umask(0) to prove the mode came from the open flag, not the umask (a default-mode create under umask 0 would be 0666). Command: cmd/dn-tool/write_config.go — path is the positional arg (ArgsUsage <path>), required (errMissingWriteConfigPath); thin writeConfigAction -> testable runWriteConfig(cfg, path). Round-trip tested through the real loader (WriteConfigFile -> ParseEnvFile -> Resolve). API key persisted cleartext, protected by 0600 (documented decision). Updated main_test stub list (write-config now wired; only 'run' remains notImplemented).
