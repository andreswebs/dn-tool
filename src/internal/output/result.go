// Package output defines the command result contract: one structured JSON
// object per command on stdout (design §2.8). The Result schema is the shared
// signal the exit-code layer reads via the Changed field.
package output

import (
	"encoding/json"
	"io"
)

// Result is a single command's machine-readable outcome. The JSON tags fix the
// stdout contract (design §2.8); fields are additive per command and optional
// ones omit when empty. Changed drives --assert-changed / exit 2.
type Result struct {
	Action  string `json:"action"` // "enroll" | "unenroll" | "install" | ...
	Changed bool   `json:"changed"`
	HostID  string `json:"hostId,omitempty"`
	Network string `json:"network,omitempty"`
}

// WriteResult emits r as exactly one JSON object followed by a newline to w
// (stdout in production). It writes nothing else, keeping stdout reserved for
// the single machine-readable result; diagnostics go to stderr via the logger.
func WriteResult(w io.Writer, r Result) error {
	enc := json.NewEncoder(w)
	return enc.Encode(r)
}
