---
id: dt-kiqk
status: closed
deps: [dt-mhir]
links: []
created: 2026-06-06T14:14:57Z
type: task
priority: 0
assignee: Andre Silva
parent: dt-uzx6
tags: [config, env-file]
---
# env-file parser (plain data, never executed)

Parse the `--env-file` path as **plain key-value data** and merge it into the config source. This closes upstream **SEC3** (the bash version `source`d the config as shell, allowing arbitrary code execution). The parser must never source, exec, or shell-expand file contents.

## Public interface

```go
// internal/config
func ParseEnvFile(r io.Reader) (map[string]string, error)  // pure: reader -> KV map
```

Take an `io.Reader` (not a path) so tests use `strings.NewReader`. The command layer opens the file and passes the reader. Loading then becomes: parse file → overlay live env (precedence in dt-9x3y).

## Format rules

- `KEY=VALUE` per line; trim surrounding whitespace around `KEY`.
- Blank lines and lines beginning with `#` are ignored.
- A value may be optionally wrapped in single or double quotes; strip one layer of matching quotes. **No** variable interpolation, **no** command substitution.
- Malformed lines (no `=`, empty key) → return a clear error naming the line.

## Behaviors (TDD order)

1. **Simple `KEY=VALUE` parsed** — one pair returned.
2. **Comments and blank lines ignored** — `#…` and empty lines skipped.
3. **Quoted values unwrapped** — `K="a b"` → `a b`; `K='x'` → `x`.
4. **Shell metacharacters are literal data** — `K=$(rm -rf /)` and `` K=`id` `` and `K=$OTHER` return the literal string; nothing is executed or expanded (the SEC3 guard — make this an explicit test).
5. **Malformed line errors clearly** — a line without `=` names the offending line.

## Test strategy

Table-driven over `(file text) -> (map | error)` via `strings.NewReader`. The SEC3 test asserts the literal value is preserved byte-for-byte.

## Acceptance

- Valid files parse to the expected map; comments/blanks ignored; quotes handled.
- No code path executes, sources, or expands file content.
- Malformed input yields a clear, line-referencing error.

## References

- Design: [Req 6](../docs/dn-tool-design.md#6-configuration-interface).
- Closes upstream: [**SEC3**](../docs/research/quickvm-defined-systemd-units-analysis.md#summary-table) (config sourced as shell).
- Security invariant: [docs/agents/code-conventions.md](../docs/agents/code-conventions.md) — "env files are data, not code".

Parent epic: [dt-uzx6](dt-uzx6.md).

## Notes

**2026-06-06T18:44:16Z**

Added internal/config/envfile.go: ParseEnvFile(io.Reader) (map[string]string, error), a pure reader->KV parser (command layer opens the file). Format: KEY=VALUE; key trimmed; value trimmed then one layer of matching single/double quotes stripped (quotes protect interior whitespace); blank lines and #-comment lines (after trim) skipped; no '=' or empty key -> nil map + line-naming error. SEC3 closed: zero expansion — $(...), backticks, $VAR, ;/&& all preserved byte-for-byte (explicit test). Unbalanced/mismatched quotes left literal. Value-trimming was a spec gap (ticket only said trim KEY); chose to trim value too since the quote rule exists precisely to protect intentional whitespace. Next: dt-9x3y overlays live env (precedence) and dt-druh serializes config back to this format.
