---
id: dt-toaj
status: closed
deps: [dt-zxwf]
links: []
created: 2026-06-06T14:14:57Z
type: task
priority: 1
assignee: Andre Silva
parent: dt-cq78
tags: [output, security]
---
# Secret redaction in logs

Guarantee that secrets never reach logs: the API key (`DN_API_KEY`) and single-use enrollment codes must not appear in any log line or result. Closes upstream **SEC5** (the enrollment response — including the single-use code and host/network/role IDs — was echoed to stdout and captured by journald).

## Approach (structural, not a string filter)

- Redaction is **structural**: never put the secret into a logged struct/attr in the first place. Enrollment codes are used in-memory only and handed straight to `dnclient enroll` ([ENR.create](dt-pe29.md)).
- For the API key, provide a `slog.LogValuer` redacting type (or a config accessor that never logs the raw value) so accidental `slog` logging of the config can't leak it.

```go
// internal/config or internal/output
type Secret string
func (Secret) LogValue() slog.Value { return slog.StringValue("REDACTED") }
func (s Secret) Reveal() string      { return string(s) } // explicit, greppable access
```

## Behaviors (TDD order)

1. **Secret type redacts under slog** — logging a `Secret` yields `REDACTED`, never the value.
2. **Logging the Config does not leak the key** — marshal/log a populated `Config`; assert the API key string is absent from output.
3. **Enrollment code never logged** — a representative enroll log path (with a fake code) produces output that does not contain the code.
4. **`Reveal()` is the only way to get the raw value** — and it's used only where the secret is actually needed (auth header, env-file write).

## Test strategy

Log through a buffer logger and assert the secret substring is absent. Add a focused test that the `Config` log representation omits the key. Treat any appearance of the raw secret as a hard failure.

## Acceptance

- API key and enrollment codes never appear in logs or stdout.
- Raw secret access is explicit (`Reveal()`) and confined to auth + env-file write.

## References

- Design: [Req 7](../docs/dn-tool-design.md#7-output-and-observability), [§2.8 Output contract](../docs/dn-tool-design.md#28-output-contract).
- Closes upstream: [**SEC5**](../docs/research/quickvm-defined-systemd-units-analysis.md#summary-table).
- Security invariant: [docs/agents/code-conventions.md](../docs/agents/code-conventions.md) — "never log secrets".

Parent epic: [dt-cq78](dt-cq78.md).

## Notes

**2026-06-06T20:30:17Z**

Added config.Secret type (internal/config/secret.go): redacts to "REDACTED" structurally across slog (LogValue), fmt %v/%s/%q/%+v/%#v (String/GoString), and encoding/json (MarshalJSON); Reveal() is the sole raw accessor. Wired it in: Config.APIKey is now Secret (behavior 2 — logging Config no longer leaks the key); api.EnrollmentCode.Code is now config.Secret (behavior 3 — logging the create response redacts the code). Secret has NO UnmarshalJSON, so the real code still round-trips IN from the API response (default string unmarshal) while marshal/log redacts OUT — verified by the existing CreateHostAndEnrollmentCode test still asserting the real value. Reveal() used at exactly one prod site: api/client.go bearer header (TestDoSetsBearerAuth confirms the real token still transmits). Handoff: the create cell (dt-pe29) and dnclient enroller (dt-a772) must call EnrollmentCode.Code.Reveal() when handing the code to 'dnclient enroll'; the write-config serializer (dt-cmdg/dt-druh) must call Config.APIKey.Reveal() when writing the env-file. Closes upstream SEC5. Parent epic dt-cq78 now has only dt-0jfp (--log-text) open.
