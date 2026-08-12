# Build & validation

All commands run from the **project root** via `make`. The Go module
(`github.com/andreswebs/dn-tool`) lives at the repository root, with `go.mod`,
`go.sum`, `.golangci.yml`, `cmd/`, `internal/`, and `e2e/` directly under it.
`e2e/` is the exec-level suite (it builds the binary and drives it as a real
process, per [the e2e plan](../specs/e2e-testing.md)); `make test` runs it with
everything else, and it skips itself on a host the `install` platform gate
rejects.

## The quality gate

**After any code change, run `make build` before considering the task
complete.** `build` runs `validate` (the full gate) and then compiles the
current-platform binary to `bin/dn-tool-<os>-<arch>`. The gate is:

1. `fmt-check` — code must be gofmt-clean
2. `vet` — no suspicious constructs
3. `lint` — no `golangci-lint` violations
4. `test` — all tests pass
5. compile

If any step fails, fix the cause before proceeding. Do **not** silence lint
errors with `_ =` — handle them properly (log, return, or assert in a test).

For a faster inner loop, use the individual targets below, but always finish
with a full `make build`.

## Targets

| Command          | Purpose                                                      |
| ---------------- | ------------------------------------------------------------ |
| `make build`     | Run the gate, then build the local binary (`bin/dn-tool-…`). |
| `make build-all` | Cross-compile every supported OS/arch.                       |
| `make dist`      | Package cross-platform archives + `SHA256SUMS.txt` to `dist/`. |
| `make run`       | Run the CLI directly with `go run`.                          |
| `make validate`  | `fmt-check` + `vet` + `lint` + `test` (no compile).          |
| `make test`      | Run all tests.                                               |
| `make test-race` | Run tests with the race detector.                            |
| `make perf`      | Run performance-budget tests (`-tags perf`).                 |
| `make vet`       | `go vet ./...`                                                |
| `make fmt`       | Format all Go source with `gofmt -w`.                        |
| `make fmt-check` | Fail if any file is not gofmt-clean.                         |
| `make lint`      | Run `golangci-lint`.                                         |
| `make clean`     | Remove build artifacts from `bin/` and `dist/`.              |

Run `make help` for the authoritative, self-documenting list.
