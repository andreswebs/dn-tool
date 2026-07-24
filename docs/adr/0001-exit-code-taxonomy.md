# 0001: Exit-code taxonomy

## Context

Tools in this project are agent-first: exit codes are part of the machine contract, alongside JSON output on stdout and diagnostics on stderr.

## Decision

Exit codes follow this table:

| Code    | Meaning                                                                                                                         |
| ------- | ------------------------------------------------------------------------------------------------------------------------------- |
| 0       | success, clean                                                                                                                  |
| 1       | recoverable result: the command completed and the output demands action (violations found, partial success, assert-style no-op) |
| 2-63    | optional tool-specific result sub-codes (see rules)                                                                             |
| 64      | usage error: the CLI surface was misused (EX_USAGE)                                                                             |
| 65      | data error: payload or input data rejected (EX_DATAERR)                                                                         |
| 70      | internal error, a bug (EX_SOFTWARE)                                                                                             |
| 74      | input/output failure (EX_IOERR)                                                                                                 |
| 78      | configuration error (EX_CONFIG)                                                                                                 |
| 130/143 | terminated by SIGINT/SIGTERM (128 plus the signal number)                                                                       |

Failure classes use the BSD `sysexits.h` range. The five codes above (64, 65, 70, 74, 78) are the mandatory core. Other `sysexits.h` codes (66 EX_NOINPUT, 69 EX_UNAVAILABLE, 75 EX_TEMPFAIL, 77 EX_NOPERM, and so on) are permitted when their semantics fit exactly.

Rules:

- Exit 1 and the 2-63 range are result classes only: the command completed, and the exit code summarizes the result (prior art: `grep` and `diff`, which exit 1 for a negative result and 2 for trouble). Failures never use this range; failures live in 64-78.

- Every exit code the tool can produce is declared as data in code (a per-command registry, or a single table for flat CLIs), and at least one test asserts that real exits match the declaration. Prose documentation alone is not acceptable.

- A tool that catches SIGINT or SIGTERM for graceful shutdown must exit 128 plus the signal number (130 or 143) after cleanup, overriding the interrupted work's exit code. Tools that do not catch signals need no signal handling: default signal death already produces the correct value.

- Long-running servers adopt the applicable slice only: the signal rule plus the startup failure codes (64 usage, 78 configuration).

## Consequences

- Consumers, including AI agents and shell scripts, can branch on exit class without parsing output, and knowledge transfers across every tool in the family.

- `set -e` scripts treat exit 1 (recoverable result) as failure; that is intended, since a result demanding action usually warrants stopping a pipeline. Scripts that only care about hard failures should test for codes 64 and above.

- The taxonomy diverges from tools that use exit 1 for generic failure; the registry-as-data rule plus its conformance test is the guard against drift into improvised codes.
