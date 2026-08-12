package output

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/andreswebs/dn-tool/internal/terr"
)

// ADR 0005: absent collections serialize as [] and never as null, enforced by the
// envelope's own MarshalJSON rather than by call-site discipline. Marshaling a
// zero envelope is the worst case: every collection is nil.
func TestSchemaEnvelope_NilCollectionsMarshalAsEmptyArrays(t *testing.T) {
	data, err := json.Marshal(SchemaEnvelope{})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got := string(data)
	for _, field := range []string{
		`"commands":[]`, `"global_flags":[]`, `"exit_codes":[]`, `"errors":[]`,
		`"statuses":[]`,
	} {
		if !strings.Contains(got, field) {
			t.Errorf("SchemaEnvelope{} = %s, want it to contain %s", got, field)
		}
	}
	if strings.Contains(got, "null") {
		t.Errorf("SchemaEnvelope{} = %s, want no null collection", got)
	}
}

// The same rule applies one level down: a command with no flags and no declared
// exit codes still renders both as [].
func TestSchemaCommand_NilCollectionsMarshalAsEmptyArrays(t *testing.T) {
	data, err := json.Marshal(SchemaCommand{Name: "install"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got := string(data)
	for _, field := range []string{`"flags":[]`, `"exit_codes":[]`} {
		if !strings.Contains(got, field) {
			t.Errorf("SchemaCommand{} = %s, want it to contain %s", got, field)
		}
	}
	if strings.Contains(got, "null") {
		t.Errorf("SchemaCommand{} = %s, want no null collection", got)
	}
}

// The head still opens the envelope: MarshalJSON must not reorder or drop the
// embedded Head (the type-alias trick preserves field order).
func TestSchemaEnvelope_OpensWithHead(t *testing.T) {
	data, err := json.Marshal(SchemaEnvelope{Head: OKHead(), Tool: "dn-tool"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	const head = `{"schema_version":1,"ok":true,`
	if !bytes.HasPrefix(data, []byte(head)) {
		t.Errorf("SchemaEnvelope = %s, want it to open with %s", data, head)
	}
}

// Args is a synopsis some commands do not have, so it omits rather than
// rendering as an empty string; the passthrough flag is uniform on every command
// so a consumer can read it without a presence check.
func TestSchemaCommand_OmitsEmptyArgsAndAlwaysReportsPassthrough(t *testing.T) {
	data, err := json.Marshal(SchemaCommand{Name: "install"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), `"args"`) {
		t.Errorf("SchemaCommand without args = %s, want no args field", data)
	}
	if !strings.Contains(string(data), `"passes_through_child_exit_status":false`) {
		t.Errorf("SchemaCommand = %s, want the passthrough flag reported as false", data)
	}
}

// AllExitCodes is the tool-level union the schema envelope reports: every code
// any command declares, sorted ascending and de-duplicated.
func TestAllExitCodes_IsTheSortedDeduplicatedUnion(t *testing.T) {
	got := AllExitCodes()

	if !sort.IntsAreSorted(got) {
		t.Errorf("AllExitCodes() = %v, want ascending order", got)
	}
	seen := map[int]bool{}
	for _, c := range got {
		if seen[c] {
			t.Errorf("AllExitCodes() = %v, contains %d twice", got, c)
		}
		seen[c] = true
	}
	for cmd, codes := range CommandExitCodes {
		for _, c := range codes {
			if !seen[c] {
				t.Errorf("AllExitCodes() = %v, missing %d declared by %q", got, c, cmd)
			}
		}
	}
	for _, c := range got {
		if !declaredByAnyCommand(c) {
			t.Errorf("AllExitCodes() = %v, contains %d which no command declares", got, c)
		}
	}
}

// ErrorInventory projects terr's registry, which is the whole point of the ADR
// 0002 pairing: the documented error surface cannot drift from the real one. It
// reports the failures; the non-failure statuses are their own list, so the
// inventory never claims a graceful stop or a no-op was a failure.
func TestErrorInventory_ProjectsEveryRegisteredFailureSentinel(t *testing.T) {
	sentinels := failureSentinels()
	got := ErrorInventory()

	if len(got) != len(sentinels) {
		t.Fatalf("ErrorInventory() has %d entries, want %d (terr.All() failures)", len(got), len(sentinels))
	}
	for i, want := range sentinels {
		if got[i].Code != want.Code() {
			t.Errorf("entry %d code = %q, want %q", i, got[i].Code, want.Code())
		}
		if got[i].ExitCode != want.ExitCode() {
			t.Errorf("entry %q exit code = %d, want %d", got[i].Code, got[i].ExitCode, want.ExitCode())
		}
		if got[i].Hint != want.Hint() {
			t.Errorf("entry %q hint = %q, want %q", got[i].Code, got[i].Hint, want.Hint())
		}
	}
}

func declaredByAnyCommand(code int) bool {
	for _, codes := range CommandExitCodes {
		if contains(codes, code) {
			return true
		}
	}
	return false
}

// The two inventories partition the registry: every registered sentinel appears
// in exactly one of them, so option (a) of dt-nrq1 (a parallel statuses list)
// keeps the code-to-meaning mapping without losing a code.
func TestInventoriesPartitionTheRegistry(t *testing.T) {
	status := terr.NewStatus("t_inventory_status", 1, "drop the flag", "nothing changed")
	failure := terr.New("t_inventory_failure", 74, "check the path", "write failed")

	inFailures, inStatuses := map[string]bool{}, map[string]bool{}
	for _, e := range ErrorInventory() {
		inFailures[e.Code] = true
	}
	for _, s := range StatusInventory() {
		inStatuses[s.Code] = true
	}

	for _, e := range terr.All() {
		if inFailures[e.Code()] == inStatuses[e.Code()] {
			t.Errorf("sentinel %q is in both inventories or in neither", e.Code())
		}
	}
	if !inStatuses[status.Code()] {
		t.Errorf("status %q is missing from StatusInventory()", status.Code())
	}
	if !inFailures[failure.Code()] {
		t.Errorf("failure %q is missing from ErrorInventory()", failure.Code())
	}
}

// A status entry carries the mapping a consumer branches on: the code, the exit
// code it produces, and what it means. The hint stays optional, because a status
// like a signal shutdown has nothing to remediate.
func TestStatusInventory_CarriesCodeExitAndMeaning(t *testing.T) {
	s := terr.NewStatus("t_status_projection", 143, "", "terminated by signal")

	var got SchemaStatus
	for _, entry := range StatusInventory() {
		if entry.Code == s.Code() {
			got = entry
		}
	}
	if got.Code == "" {
		t.Fatalf("StatusInventory() has no entry for %q", s.Code())
	}
	if got.ExitCode != 143 {
		t.Errorf("exit code = %d, want 143", got.ExitCode)
	}
	if got.Meaning != "terminated by signal" {
		t.Errorf("meaning = %q, want %q", got.Meaning, "terminated by signal")
	}
	if got.Hint != "" {
		t.Errorf("hint = %q, want empty for a status with nothing to remediate", got.Hint)
	}
}

// failureSentinels is the registry's failure half, in registration order.
func failureSentinels() []*terr.E {
	var out []*terr.E
	for _, e := range terr.All() {
		if !e.NonFailure() {
			out = append(out, e)
		}
	}
	return out
}
