# Code conventions & invariants

Project-specific rules that are easy to get wrong. General coding hygiene is
assumed; this file covers only what is particular to `dn-tool`.

## Security invariants (non-negotiable)

These derive from the design requirements and the upstream bug analysis. Hold
them in any code that touches secrets or config:

- **Never log secrets.** The API key (`DN_API_KEY`) and single-use enrollment
  codes must never reach logs or stdout. Enrollment codes are in-memory only.
- **Env files are data, not code.** Parse `--env-file` as plain key-value
  pairs; never source, exec, or shell-expand their contents.
- **Config files are `0600` from creation.** Create files owner-only
  read/write at the moment of creation — never write world-readable and
  restrict afterward.

## Authoritative spec

[docs/dn-tool-design.md](../dn-tool-design.md) is the source of truth for
requirements, the command surface, config variables, and the enrollment state
machine. Implement against it; if code and the design disagree, reconcile the
design first.

## Finding-ID convention

The design and tickets reference findings from the upstream analysis by ID:
`B#` (bugs), `S#` (shell issues), `SEC#` (security), `D#` (design). The
mapping lives in
[docs/research/quickvm-defined-systemd-units-analysis.md](../research/quickvm-defined-systemd-units-analysis.md).
Cite the relevant ID when a change addresses one.
