---
id: dt-prt6
status: closed
deps: [dt-ecf9]
links: []
created: 2026-06-06T14:14:57Z
type: task
priority: 0
assignee: Andre Silva
parent: dt-vsi6
tags: [scaffolding, lint, build]
---
# golangci-lint config + green validate gate

Add a `golangci-lint` configuration and make `make validate` (fmt-check → vet → lint → test) pass on the skeleton. Establishes the quality gate every later task must keep green.

## Public interface

A `.golangci.yml` (or `.golangci.toml`) under `src/` (the lint target runs `cd src && golangci-lint run ./...`). Enable a sensible, non-noisy linter set; do not enable everything.

Suggested baseline: the defaults plus `errcheck`, `govet`, `staticcheck`, `revive` (or `gofmt`/`goimports` formatting check). Tune to the project — see the [golang skill linting guidance].

## Behaviors / verification

1. **`make lint` runs clean** on the current skeleton (CLI stub + `internal/version`).
2. **`make validate` green** end to end (fmt-check, vet, lint, test).
3. **Lint catches real issues** — e.g. an unchecked error is flagged (sanity-check the config isn't a no-op).
4. **No lint-silencing convention** — the gate forbids `_ =` to mute errors (per `docs/agents/build-and-validation.md`); reviewers handle errors properly instead.

## Acceptance

- `make validate` exits 0 on the skeleton.
- `.golangci.yml` is committed and the enabled linters are intentional (documented if non-obvious).

## References

- Project gate: [docs/agents/build-and-validation.md](../docs/agents/build-and-validation.md).
- Design: [§2.12 step 1](../docs/dn-tool-design.md#212-build--migration-order).

Parent epic: [dt-vsi6](dt-vsi6.md).

## Notes

**2026-06-06T18:21:36Z**

Added src/.golangci.yml (golangci-lint v2 schema — toolchain is v2.12.2, NOT the v1 format in the golang skill). Config: 'default: standard' (errcheck/govet/ineffassign/staticcheck/unused) + revive for style. Scoped exclusion suppresses revive's stutter warning on api.APIError/APIErrorItem only (internal/api/) — those names are deliberate per docs/learnings.md (enroll's orphan check uses errors.As(err,&apiErr)); renaming is the api package's call, not the gate's. Behavior 4 (no '_ =' error-muting) is a documented review convention in the config header + build-and-validation.md, NOT errcheck check-blank — the project's own committed code uses '_ = resp.Body.Close()' (client.go:125) and '_, _ = w.Write' in httptest handlers, so check-blank would have broken 'make validate' and contradicted the project's idioms. Verified config is not a no-op (temp unchecked os.Setenv was flagged by errcheck). Fixed genuinely-missing doc/package comments revive flagged: internal/version (package + Override + Current) and cmd/dn-tool/main.go (command package comment). make validate exits 0.
