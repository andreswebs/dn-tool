package output

// CommandExitCodes is the declared, per-command exit-code registry required by
// ADR 0001: every code a command can produce is data here, and a conformance
// test (exitcodes_test.go, plus the cmd-level scenario tests) asserts that real
// exits match a declaration rather than trusting prose.
//
// The failure classes {CodeUsage, CodeData, CodeUnavailable, CodeInternal,
// CodeIO, CodeConfig} are common to every command: any command can be misused
// (64), hit a bad config (78), fail local I/O (74), or fall back to the
// unclassified-bug code (70); commands that reach the API can also see 65/69.
// CodeResult (1) is declared only for the result-producing commands, where a
// --assert-changed no-op is a distinct non-error result. run is the long-running
// exception: it has no --assert-changed (no 1), adds the signal codes 130/143,
// and additionally passes through the foreground daemon's own exit status
// (documented below, not enumerable as a fixed set).
var CommandExitCodes = map[string][]int{
	"install":      resultCommandCodes(),
	"enroll":       resultCommandCodes(),
	"unenroll":     resultCommandCodes(),
	"write-config": resultCommandCodes(),
	"run": {
		CodeOK,
		CodeUsage, CodeData, CodeUnavailable, CodeInternal, CodeIO, CodeConfig,
		CodeSIGINT, CodeSIGTERM,
	},
	// The root command dispatches to subcommands; on its own it either prints
	// help (0), rejects an unknown command or a bad global flag (64), or falls
	// back to the unclassified-bug code (70).
	"dn-tool": {CodeOK, CodeUsage, CodeInternal},
}

// RunPassesThroughDaemonExitStatus documents the one registry exception: the run
// command propagates the foreground dnclient daemon's own exit status N verbatim
// (design Req 5), so run may exit with any code the daemon produces in addition
// to the codes declared in CommandExitCodes["run"].
const RunPassesThroughDaemonExitStatus = true

// resultCommandCodes is the code set shared by the four result-producing
// commands (install, enroll, unenroll, write-config): success, the
// --assert-changed no-op result, and every failure class.
func resultCommandCodes() []int {
	return []int{
		CodeOK, CodeResult,
		CodeUsage, CodeData, CodeUnavailable, CodeInternal, CodeIO, CodeConfig,
	}
}
