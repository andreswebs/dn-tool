# 0006: CLI testing

## Context

The preceding ADRs impose conformance obligations: declared exit codes
must match real exits (0001), the error contract must hold (0002), CLI
tests must be framework-blind (0003), and the output contract must not
drift (0005). This ADR fixes how those obligations are verified, so
the enforcement layer is as uniform as the contracts it enforces.

## Decision

### In-process golden triple (required)

- The test suite carries a golden-triple harness: scenarios invoke the
  delegate (`Run(args, deps)` per the CLI-structure ADR) with buffer
  streams and compare three artifacts per scenario against golden
  files: stdout, stderr, and the exit code.
- The harness supports an `-update` flag that regenerates golden
  files, making every contract change a reviewable diff instead of a
  hand-edit.
- Golden content is normalized for determinism: working-directory
  paths, ephemeral ports, and similar volatile values are replaced
  with stable tokens before comparison.
- Conformance obligations are expressed through this harness: every
  code in the exit-code registry is exercised by at least one
  scenario, every observed exit is a member of the registry, and
  error scenarios cover the envelope shape.
- Because scenarios touch only the delegate contract, the suite is
  framework-blind: it survives a CLI-framework replacement unchanged
  and is the safety net for one.

### Exec-based end-to-end tests (conditional)

- When the tool has process-level behavior that in-process tests
  cannot reach (signal handling and 130/143 exits, subprocess
  lifecycles, child exit-status passthrough), it also carries an
  end-to-end suite that builds the real binary once, drives it as a
  process, and applies the same golden-triple discipline to its
  streams and exit status.
- Absent such behavior, an end-to-end suite is optional.

### Unit tests

- Domain packages keep conventional table-driven unit tests; the
  golden harness covers the CLI surface, not the domain logic behind
  it. Help-text or other hand-maintained surfaces that mirror
  declarations carry a drift test tying the two together.

## Consequences

- Contract changes (exit codes, envelopes, error shapes) become
  mechanical: change the code, run with `-update`, review the golden
  diff, and the diff is the release note's evidence.
- The golden suite doubles as executable documentation of the tool's
  exact behavior per scenario.
- Normalization discipline is load-bearing: an unnormalized volatile
  value makes goldens flaky, which is the harness's main failure
  mode.
- Framework migrations and refactors of the CLI interior are guarded
  by a suite that does not mention the framework.
