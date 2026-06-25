# AI agent instructions

`dn-tool` is a Go control-plane CLI that enrolls and unenrolls Linux hosts in a
[defined.net](https://defined.net) Managed Nebula network — it wraps the
defined.net REST API and orchestrates the proprietary `dnclient` daemon.

The authoritative
specification is [docs/dn-tool-design.md](docs/dn-tool-design.md), and work is
tracked as epics in `.tickets/`. **Read the design doc before implementing
anything.**

## Working in this repo

- **[Build & validation](docs/agents/build-and-validation.md)** — `make`
  targets and the quality gate. After any code change, run `make build` and fix
  any failure before considering the task done.
- **[Ticket workflow](docs/agents/tickets.md)** — managing work with the `tk`
  CLI.
- **[Code conventions & invariants](docs/agents/code-conventions.md)** —
  security rules and project-specific conventions that are easy to get wrong.

## Reference material

- [docs/dn-tool-design.md](docs/dn-tool-design.md) — requirements +
  implementation plan (authoritative).
- [docs/research/](docs/research/) — defined.net API reference, the original
  upstream source, and the upstream bug analysis.
- [docs/research/dnclient-runtime-behavior.md](docs/research/dnclient-runtime-behavior.md)
  — observed behavior of the proprietary `dnclient` binary (enroll/daemon
  ordering, socket, config root, `dnclient.yml` schema, shutdown exit codes,
  required API scopes). Read it before touching the `run` lifecycle, `dnstate`,
  or anything that shells out to `dnclient`.
