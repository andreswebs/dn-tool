// Package output defines the command result contract: one structured JSON
// envelope per command on stdout (ADR 0005, design §2.8), opening with the
// schema_version/ok head. The Result payload is the shared signal the exit-code
// layer reads via the Changed field.
package output

import (
	"bytes"
	"encoding/json"
	"io"
)

// SchemaVersion is the version of dn-tool's machine-readable output contract,
// bumped on a breaking shape change (ADR 0005). It heads every envelope, so a
// consumer can detect a contract change instead of discovering it as a parse
// failure.
const SchemaVersion = 1

// Head opens every envelope, result and error alike (ADR 0005), so a consumer can
// read the contract version and success flag before it knows the payload shape.
// Embed it as the first field of a command's envelope struct: encoding/json
// inlines an embedded struct at its declaration position, which puts the head
// first.
type Head struct {
	SchemaVersion int  `json:"schema_version"`
	OK            bool `json:"ok"`
}

// OKHead returns the head every successful result carries.
func OKHead() Head {
	return Head{SchemaVersion: SchemaVersion, OK: true}
}

// Result is a single command's machine-readable outcome, the payload half of the
// envelope. The JSON tags fix the stdout contract; fields are additive per command
// and optional ones omit when empty. Changed drives --assert-changed / exit 1.
type Result struct {
	Action  string `json:"action"` // "enroll" | "unenroll" | "install" | ...
	Changed bool   `json:"changed"`
	HostID  string `json:"host_id,omitempty"`
	Network string `json:"network,omitempty"`
}

// ResultEnvelope is the stdout envelope shared by every command that reports an
// outcome (all but schema, which has its own SchemaEnvelope): they emit the same
// payload, so per-command envelope types would be five identical types.
// Split it when payloads actually diverge. It carries no collection, so it needs
// no null-coalescing MarshalJSON.
type ResultEnvelope struct {
	Head
	Result
}

// WriteResult emits r wrapped in its envelope as exactly one JSON object followed
// by a newline to w (stdout in production). The wrapping happens here so no caller
// can emit a headless result. It writes nothing else, keeping stdout reserved for
// the single machine-readable result; diagnostics go to stderr via the logger.
func WriteResult(w io.Writer, r Result) error {
	return EmitJSON(w, ResultEnvelope{Head: OKHead(), Result: r})
}

// EmitJSON marshals v and writes it to w as one newline-terminated line. It is the
// shared emit path, so the result and error envelopes cannot drift in framing or
// escaping.
func EmitJSON(w io.Writer, v any) error {
	data, err := marshalLine(v)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// marshalLine renders v as one newline-terminated line with HTML escaping off, so
// a value containing <, > or & reads as itself rather than as &lt; on a terminal.
// No envelope is ever embedded in HTML, so the escaping buys nothing.
func marshalLine(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
