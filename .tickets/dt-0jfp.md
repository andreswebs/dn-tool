---
id: dt-0jfp
status: closed
deps: [dt-zxwf]
links: []
created: 2026-06-06T14:14:57Z
type: task
priority: 1
assignee: Andre Silva
parent: dt-cq78
tags: [output, logging]
---
# Plain-text logging (--log-text)

Add the plain-text logging mode selected by the `--log-text` global flag: when set, `NewLogger` uses a text handler instead of JSON (still to stderr, still level-filtered). For humans at a terminal; JSON remains the default for machines.

## Public interface

Extends `NewLogger` (OUT.slog / dt-zxwf): when `LogOptions.Text` is true, use `slog.NewTextHandler` instead of `slog.NewJSONHandler`. No new exported symbol.

## Behaviors (TDD order)

1. **`Text=true` emits plain text** — output is `slog` text format (`key=value`), not JSON.
2. **`Text=false` stays JSON** — default unchanged (regression guard).
3. **Level filtering still applies** — text mode honors `DN_LOG_LEVEL` identically.
4. **`--log-text` flag wired** — the CLI flag sets `LogOptions.Text` (verified via the command wiring).

## Test strategy

`NewLogger(&buf, LogOptions{Text:true})`; assert output is not valid JSON and matches the text-handler shape; repeat with `Text:false` to confirm JSON.

## Acceptance

- `--log-text` produces plain-text stderr logs; default remains JSON; level filter unaffected.

## References

- Design: [Req 7](../docs/dn-tool-design.md#7-output-and-observability) ("WHERE human-readable logging is requested…"), [§2.10 libraries](../docs/dn-tool-design.md#210-cli--libraries).

Parent epic: [dt-cq78](dt-cq78.md).

## Notes

**2026-06-06T20:41:23Z**

Plain-text logging (--log-text) implemented. NewLogger now branches on opts.Text: slog.NewTextHandler vs NewJSONHandler (still stderr, still level-filtered). Flag wired via a root cli Before hook calling slog.SetDefault(NewLogger(os.Stderr, logOptions(cmd))) — logOptions(cmd) reads cmd.Bool("log-text") + DN_LOG_LEVEL from live env. main() keeps a bootstrap JSON logger for pre-flag-parse failures; Before reconfigures once --log-text is known. Tests: TextMode + TextModeLevelFiltering (output pkg, behaviors 1/3), TestLogText_FlagWiredToLogOptions drives the real app swapping install's action to capture logOptions after parse (behavior 4). This was the last open child of epic dt-cq78 — that epic plus dt-zwgc (all children closed) can now be verified/closed to unblock the P0 install/enroll/unenroll commands.
