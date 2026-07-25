# 0003: CLI structure

## Context

Tools in this project are agent-first command-line programs built on
`urfave/cli` v3. The framework serves well, but the tool must not be
structurally welded to it: a future maintainer should be able to
replace it (or prefer a plain flag package for a flat tool) without
renegotiating the tool's contract. The structure below separates that
contract, which tests and consumers rely on, from the framework
interior, which is replaceable.

## Decision

### The contract (framework-free)

- `main` is one line and is the only place the real process
  environment is touched:

  ```go
  func main() {
      os.Exit(command.Run(os.Args[1:], command.Deps{
          In: os.Stdin, Out: os.Stdout, Err: os.Stderr,
      }))
  }
  ```

- `internal/command` owns the CLI surface. Its entry point is
  `Run(args []string, deps Deps) int`. No framework type appears in
  this signature or in any exported identifier of the package.
- `Deps` starts lean: `In io.Reader`, `Out, Err io.Writer`. A field is
  added only when a test must fake it (`Getenv`, a clock, terminal
  detection). Arguments are a parameter of `Run`, not a dependency.
- `Run` owns the exit boundary: it emits the error envelope to
  `deps.Err` per the error-handling ADR, chooses the exit code per the
  exit-code taxonomy ADR by resolving the typed-error interface, and
  applies the signal override (130/143) when the tool catches signals
  for graceful shutdown. `main` never inspects errors.
- Command code reads input only from `deps.In`; it never touches
  `os.Stdin`, `os.Stdout`, `os.Stderr`, or `os.Args` directly.
- CLI-level tests drive `Run` with buffers and assert the
  stdout/stderr/exit triple. Because they touch only the contract,
  they are framework-blind: if the framework is ever replaced, the
  entire CLI test suite carries over unchanged.

### The framework interior (replaceable)

- Framework-specific code lives in dedicated files of
  `internal/command` (the command-tree construction, flags, help,
  environment-variable sources, shell completion), conventionally
  `root.go` and `commands*.go`, while the contract lives in `run.go`.
  Replacing the framework replaces those files and keeps `run.go` and
  the tests.
- The no-leak rule: the framework is imported only inside
  `internal/command`; never in domain packages, the typed-error
  package, or the output layer. Framework package globals (custom
  exiters, version printers) are not mutated; the framework's own
  error printing and exit handling are neutralized inside the
  interior, and errors are mapped after the framework returns.
- The sanctioned usage-error classifier (the one string-matching
  carve-out from the error-handling ADR) is framework-coupled by
  definition and therefore lives in the interior files.

### Growth path

When subcommands multiply, generate the framework tree from a
framework-agnostic registry: commands are declared once as data, and
the framework wiring, help, and schema introspection are projections
of that declaration. This deepens the same seam; it is not scaffolded
by default because a new tool with one command does not need it.

### What replacing the framework costs

A replacement re-implements help text (pair it with a drift test tying
help to the registered flags), environment-variable precedence, and
completion. The contract contains these costs; it does not remove
them.

## Consequences

- The framework is a dependency of one package's interior, not of the
  tool's architecture; migrating frameworks is a bounded, test-covered
  operation.
- The whole CLI surface is testable in-process with buffers; no
  framework globals are swapped in tests.
- The no-leak rule is cheap to verify (an import grep or a small lint
  test) but must be kept honest; one leaked framework type in an
  exported signature re-welds the tool to the framework.
- One line of ceremony in `main` and a Deps struct are the total
  overhead paid while the framework is never replaced.
