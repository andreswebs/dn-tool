package output

import (
	"encoding/json"
	"sort"

	"github.com/andreswebs/dn-tool/internal/terr"
)

// SchemaEnvelope is the runtime self-description contract (ADR 0005): the tool's
// command surface, its flags, the declared exit codes, and the error inventory,
// all as one stdout envelope headed like every other. Every field is a projection
// of a declaration that already drives behavior — the command tree,
// CommandExitCodes, and terr's registry — so it is never hand-maintained and
// cannot drift from the real surface.
type SchemaEnvelope struct {
	Head
	Tool     string          `json:"tool"`
	Version  string          `json:"version"`
	Commands []SchemaCommand `json:"commands"`
	// GlobalFlags are the root command's flags, which apply to every subcommand.
	// They are reported separately because the root is a dispatcher, not an
	// invocable command, and so has no entry in Commands.
	GlobalFlags []SchemaFlag `json:"global_flags"`
	// ExitCodes is the tool-level union of every command's declared codes, sorted
	// ascending; each command's own set is on its SchemaCommand.
	ExitCodes []int         `json:"exit_codes"`
	Errors    []SchemaError `json:"errors"`
	// Statuses are the coded values that carry an exit code without being
	// failures (the --assert-changed no-op, a graceful signal shutdown). They are
	// a list of their own rather than entries under Errors: dn-tool writes no
	// error envelope for them, so reporting them as errors would repeat the
	// category confusion the envelope exists to remove, while dropping them
	// would lose the code-to-meaning mapping a consumer wants.
	Statuses []SchemaStatus `json:"statuses"`
}

// SchemaCommand is one invocable command's projected surface.
type SchemaCommand struct {
	Name  string       `json:"name"`
	Usage string       `json:"usage"`
	Args  string       `json:"args,omitempty"`
	Flags []SchemaFlag `json:"flags"`
	// ExitCodes is this command's declared set from CommandExitCodes.
	ExitCodes []int `json:"exit_codes"`
	// PassesThroughChildExitStatus reports the registry exception as data: the
	// command may exit with a status outside its declared set, because it
	// propagates a child process's status verbatim. It is emitted on every
	// command, false included, so a consumer reads it without a presence check.
	PassesThroughChildExitStatus bool `json:"passes_through_child_exit_status"`
}

// SchemaFlag is one flag as the consumer invokes it: Name carries the leading
// dashes, so a caller can use it verbatim.
type SchemaFlag struct {
	Name  string `json:"name"`
	Usage string `json:"usage"`
}

// SchemaError is one registered error sentinel, the projection ADR 0002 requires
// so the documented error surface cannot drift from the real one.
type SchemaError struct {
	Code     string `json:"code"`
	ExitCode int    `json:"exit_code"`
	Hint     string `json:"hint,omitempty"`
}

// SchemaStatus is one registered non-failure status (terr.Status). Meaning is the
// sentinel's own message, which is what a consumer reads the code against; the
// error inventory carries that text in the envelope instead, and a status has no
// envelope. Hint is optional, because a status such as a signal shutdown has
// nothing to remediate.
type SchemaStatus struct {
	Code     string `json:"code"`
	ExitCode int    `json:"exit_code"`
	Meaning  string `json:"meaning"`
	Hint     string `json:"hint,omitempty"`
}

// MarshalJSON renders the envelope with its collections coalesced: an absent list
// serializes as [] and never as null (ADR 0005), enforced here rather than by
// call-site discipline. The local alias type sheds this method, so the embedded
// Head still marshals first and the recursion terminates.
func (e SchemaEnvelope) MarshalJSON() ([]byte, error) {
	type alias SchemaEnvelope
	a := alias(e)
	if a.Commands == nil {
		a.Commands = []SchemaCommand{}
	}
	if a.GlobalFlags == nil {
		a.GlobalFlags = []SchemaFlag{}
	}
	if a.ExitCodes == nil {
		a.ExitCodes = []int{}
	}
	if a.Errors == nil {
		a.Errors = []SchemaError{}
	}
	if a.Statuses == nil {
		a.Statuses = []SchemaStatus{}
	}
	return json.Marshal(a)
}

// MarshalJSON coalesces a command's own collections, for the same reason and by
// the same means as the envelope's.
func (c SchemaCommand) MarshalJSON() ([]byte, error) {
	type alias SchemaCommand
	a := alias(c)
	if a.Flags == nil {
		a.Flags = []SchemaFlag{}
	}
	if a.ExitCodes == nil {
		a.ExitCodes = []int{}
	}
	return json.Marshal(a)
}

// AllExitCodes is the union of every code in CommandExitCodes, sorted ascending
// and de-duplicated: the tool-level answer to "what can dn-tool exit with?",
// derived from the same registry the exit boundary reads.
func AllExitCodes() []int {
	seen := map[int]bool{}
	var codes []int
	for _, declared := range CommandExitCodes {
		for _, c := range declared {
			if !seen[c] {
				seen[c] = true
				codes = append(codes, c)
			}
		}
	}
	sort.Ints(codes)
	return codes
}

// ErrorInventory projects the failure half of terr's registry in registration
// order. It reports only registered sentinels, which is the surface a consumer can
// encounter as a stable code; a per-invocation terr.Newf instance shares a
// sentinel's code and so is already covered by its entry. The non-failure statuses
// are projected by StatusInventory, so a consumer reading this list sees failures
// and only failures.
func ErrorInventory() []SchemaError {
	sentinels := terr.All()
	inventory := make([]SchemaError, 0, len(sentinels))
	for _, s := range sentinels {
		if s.NonFailure() {
			continue
		}
		inventory = append(inventory, SchemaError{
			Code:     s.Code(),
			ExitCode: s.ExitCode(),
			Hint:     s.Hint(),
		})
	}
	return inventory
}

// StatusInventory projects the non-failure half of terr's registry in registration
// order: the codes a consumer can see as an exit status without a failure having
// occurred, each with what it means.
func StatusInventory() []SchemaStatus {
	sentinels := terr.All()
	inventory := make([]SchemaStatus, 0, len(sentinels))
	for _, s := range sentinels {
		if !s.NonFailure() {
			continue
		}
		inventory = append(inventory, SchemaStatus{
			Code:     s.Code(),
			ExitCode: s.ExitCode(),
			Meaning:  s.Error(),
			Hint:     s.Hint(),
		})
	}
	return inventory
}
