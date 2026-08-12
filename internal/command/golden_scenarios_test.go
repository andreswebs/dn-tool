package command_test

import (
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
)

// scenarios is the golden suite's case list: at minimum every declared exit code
// that is reachable in process (the exemptions live in exemptFromCoverage), each
// command's success path, and the ADR 0005 stdout/stderr split on both sides of
// a failure. The API-backed and filesystem-backed cases are what exercise the
// data (65), unavailable (69) and I/O (74) classes through the CLI rather than on
// constructed errors.
func scenarios() []scenario {
	return []scenario{
		installSuccess(),
		installNoOpAssertChanged(),
		installChecksumMismatch(),
		enrollSuccess(),
		enrollAPIRejected(),
		enrollAPIUnavailable(),
		enrollAPIMalformedResponse(),
		enrollOrphaned(),
		enrollDNClientFailure(),
		unenrollSuccess(),
		unenrollMissingAPIKey(),
		writeConfigSuccess(),
		writeConfigMissingPath(),
		writeConfigIOFailure(),
		writeConfigMalformedEnv(),
		runSuccess(),
		unknownCommand(),
		badGlobalFlag(),
		badSubcommandFlag(),
		bareInvocation(),
		badLogLevel(),
		schemaCommand(),
		helpFlag(),
		versionFlag(),
	}
}

// installHandler serves the three endpoints install reads: the downloads table,
// the binary, and its published sibling checksum. publishedDigest is what the
// checksum endpoint returns, so a caller can publish a digest that does not match
// binary and drive the fail-closed verification path. Any other path falls
// through to next (the run lifecycle's enroll and delete endpoints), or is
// reported as unexpected when next is nil.
func installHandler(t *testing.T, binaryPath, binary, publishedDigest string, downloads func() string, next http.HandlerFunc) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/downloads":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(downloads()))
		case binaryPath:
			_, _ = w.Write([]byte(binary))
		case binaryPath + ".sha256":
			_, _ = w.Write([]byte(publishedDigest))
		default:
			if next == nil {
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
				return
			}
			next(w, r)
		}
	}
}

// installFixture wires an install-backed scenario: a masked bin dir, a server
// publishing binary under publishedDigest (with next serving any non-install
// endpoint), and DN_CLIENT_*. It returns the bin dir so a caller can pre-place a
// binary there.
//
// It owns the host-platform gate, because every caller needs it by definition:
// install rejects a non-Linux host, so a scenario reaching here on one would pin
// that rejection instead of its own behavior. Leaving the call to each caller
// once cost a whole platform's quality gate (dt-xyp4).
func installFixture(t *testing.T, f *fixture, binary, publishedDigest string, next http.HandlerFunc) string {
	t.Helper()
	f.installBacked = true
	requireLinuxHost(t)
	binDir := f.tempDir(t, tokenBinDir)
	const binaryPath = "/dnclient"
	var binURL string
	srv := apiServer(t, f, installHandler(t, binaryPath, binary, publishedDigest, func() string {
		return downloadsJSON(t, binURL)
	}, next))
	binURL = srv.URL + binaryPath
	f.setenv("DN_API_KEY", fixtureAPIKey)
	f.setenv("DN_CLIENT_BIN_DIR", binDir)
	f.setenv("DN_CLIENT_VERSION", fixtureVersion)
	return binDir
}

// install downloads, verifies and places the binary: exit 0 with the result
// envelope reporting the change.
func installSuccess() scenario {
	return scenario{
		name:    "install-success",
		command: "install",
		build: func(t *testing.T, f *fixture) []string {
			const binary = "#!/bin/sh\nexit 0\n"
			installFixture(t, f, binary, digestOf(binary), nil)
			return []string{"install"}
		},
	}
}

// A binary already present with the published digest is a no-op, and
// --assert-changed turns that into the distinct result class (exit 1) while the
// result envelope still goes to stdout.
func installNoOpAssertChanged() scenario {
	return scenario{
		name:    "install-noop-assert-changed",
		command: "install",
		build: func(t *testing.T, f *fixture) []string {
			const binary = "#!/bin/sh\nexit 0\n"
			binDir := installFixture(t, f, binary, digestOf(binary), nil)
			writeFakeDNClient(t, binDir, binary)
			return []string{"--assert-changed", "install"}
		},
	}
}

// A binary whose SHA-256 does not match the published checksum is rejected as a
// data error (65) and never placed.
func installChecksumMismatch() scenario {
	return scenario{
		name:    "install-checksum-mismatch",
		command: "install",
		build: func(t *testing.T, f *fixture) []string {
			installFixture(t, f, "the-served-binary", digestOf("a-different-binary"), nil)
			return []string{"install"}
		},
	}
}

// enrollHandler serves the §2.4 create path: the host list the orphan pre-check
// reads, and the create call that yields the host record and enrollment code.
// hosts is the raw JSON array the list returns.
func enrollHandler(t *testing.T, hosts string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/hosts":
			_, _ = fmt.Fprintf(w, `{"data":%s,"metadata":{"hasNextPage":false}}`, hosts)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/host-and-enrollment-code":
			_, _ = fmt.Fprintf(w,
				`{"data":{"host":{"id":%q,"name":%q},"enrollmentCode":{"code":"ENROLL-CODE","lifetimeSeconds":300}}}`,
				fixtureHostID, fixtureHostname)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

// lifecycleHandler serves the API side of the run lifecycle: the enroll create
// path plus the shutdown delete of the record enroll created.
func lifecycleHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	enroll := enrollHandler(t, "[]")
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/v1/hosts/"+fixtureHostID {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		enroll(w, r)
	}
}

// enrollFixture wires an enroll scenario against handler, with the dnclient
// stand-in script placed in the bin dir.
func enrollFixture(t *testing.T, f *fixture, handler http.HandlerFunc, script string) {
	t.Helper()
	configRoot := f.tempDir(t, tokenConfigRoot)
	binDir := f.tempDir(t, tokenBinDir)
	writeFakeDNClient(t, binDir, script)
	apiServer(t, f, handler)
	enrollEnv(f, configRoot, binDir)
}

// The create cell: no local config, no remote record, so enroll creates the
// record and hands the code to `dnclient enroll`.
func enrollSuccess() scenario {
	return scenario{
		name:    "enroll-success",
		command: "enroll",
		build: func(t *testing.T, f *fixture) []string {
			enrollFixture(t, f, enrollHandler(t, "[]"), "#!/bin/sh\nexit 0\n")
			return []string{"enroll"}
		},
	}
}

// An API 4xx is a rejected request: the data class (65), through the CLI.
func enrollAPIRejected() scenario {
	return scenario{
		name:    "enroll-api-rejected",
		command: "enroll",
		build: func(t *testing.T, f *fixture) []string {
			enrollFixture(t, f, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"errors":[{"code":"ERR_PERMISSION_DENIED","message":"insufficient scope","path":""}]}`))
			}, "#!/bin/sh\nexit 0\n")
			return []string{"enroll"}
		},
	}
}

// An API 5xx is the dependency failing on its side: the unavailable class (69).
// DN_API_TIMEOUT is short so the bounded retry (1s minimum backoff) is cut off by
// the context deadline rather than sleeping through four attempts — the same
// classification, in half a second.
func enrollAPIUnavailable() scenario {
	return scenario{
		name:    "enroll-api-unavailable",
		command: "enroll",
		build: func(t *testing.T, f *fixture) []string {
			enrollFixture(t, f, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}, "#!/bin/sh\nexit 0\n")
			f.setenv("DN_API_TIMEOUT", "500ms")
			return []string{"enroll"}
		},
	}
}

// A 2xx body that does not match the documented shape is malformed published
// data, not an unclassified bug: the data class (65).
func enrollAPIMalformedResponse() scenario {
	return scenario{
		name:    "enroll-api-malformed-response",
		command: "enroll",
		build: func(t *testing.T, f *fixture) []string {
			enrollFixture(t, f, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"data":"not-a-list","metadata":{"hasNextPage":false}}`))
			}, "#!/bin/sh\nexit 0\n")
			return []string{"enroll"}
		},
	}
}

// A remote record under the enrollment name with no local config is an orphan:
// enroll refuses and points at --force (the config class, 78).
func enrollOrphaned() scenario {
	return scenario{
		name:    "enroll-orphaned",
		command: "enroll",
		build: func(t *testing.T, f *fixture) []string {
			hosts := fmt.Sprintf(`[{"id":%q,"name":%q}]`, fixtureHostID, fixtureHostname)
			enrollFixture(t, f, enrollHandler(t, hosts), "#!/bin/sh\nexit 0\n")
			return []string{"enroll"}
		},
	}
}

// A dnclient child that exits non-zero is a local I/O failure (74) carrying the
// child's status as structured detail — never the child's status as dn-tool's own
// exit code, which no command declares.
func enrollDNClientFailure() scenario {
	return scenario{
		name:    "enroll-dnclient-failure",
		command: "enroll",
		build: func(t *testing.T, f *fixture) []string {
			enrollFixture(t, f, enrollHandler(t, "[]"), "#!/bin/sh\nexit 3\n")
			return []string{"enroll"}
		},
	}
}

// unenroll deletes the remote record read from the local config, then removes the
// local config: exit 0 with the changed result.
func unenrollSuccess() scenario {
	return scenario{
		name:    "unenroll-success",
		command: "unenroll",
		build: func(t *testing.T, f *fixture) []string {
			configRoot := f.tempDir(t, tokenConfigRoot)
			writeDNClientConfig(t, configRoot, fixtureNetwork, fixtureHostID)
			apiServer(t, f, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodDelete || r.URL.Path != "/v1/hosts/"+fixtureHostID {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				w.WriteHeader(http.StatusNoContent)
			})
			enrollEnv(f, configRoot, f.tempDir(t, tokenBinDir))
			return []string{"unenroll"}
		},
	}
}

// unenroll requires the API key up front: a missing one is a configuration error
// (78) that touches neither the filesystem nor the network.
func unenrollMissingAPIKey() scenario {
	return scenario{
		name:    "unenroll-missing-api-key",
		command: "unenroll",
		build: func(t *testing.T, f *fixture) []string {
			configRoot := f.tempDir(t, tokenConfigRoot)
			writeDNClientConfig(t, configRoot, fixtureNetwork, fixtureHostID)
			f.setenv("DN_NETWORK_NAME", fixtureNetwork)
			f.setenv("DN_CLIENT_CONFIG_DIR", configRoot)
			return []string{"unenroll"}
		},
	}
}

// write-config persists the resolved configuration to the positional path.
func writeConfigSuccess() scenario {
	return scenario{
		name:    "write-config-success",
		command: "write-config",
		build: func(t *testing.T, f *fixture) []string {
			f.setenv("DN_NETWORK_ID", fixtureNetworkID)
			f.setenv("DN_HOSTNAME", fixtureHostname)
			return []string{"write-config", filepath.Join(f.tempDir(t, tokenTmpDir), "dn.env")}
		},
	}
}

// The positional path is required: its absence is a command-line fault (64), not
// the unclassified fallback it used to take.
func writeConfigMissingPath() scenario {
	return scenario{
		name:    "write-config-missing-path",
		command: "write-config",
		build: func(_ *testing.T, f *fixture) []string {
			f.setenv("DN_HOSTNAME", fixtureHostname)
			return []string{"write-config"}
		},
	}
}

// A path under a directory that does not exist is a local I/O failure (74).
func writeConfigIOFailure() scenario {
	return scenario{
		name:    "write-config-io-failure",
		command: "write-config",
		build: func(t *testing.T, f *fixture) []string {
			f.setenv("DN_HOSTNAME", fixtureHostname)
			return []string{"write-config", filepath.Join(f.tempDir(t, tokenTmpDir), "absent-dir", "dn.env")}
		},
	}
}

// A malformed DN_* value fails configuration resolution before the command
// touches the filesystem or the API: the config class (78).
func writeConfigMalformedEnv() scenario {
	return scenario{
		name:    "write-config-malformed-env",
		command: "write-config",
		build: func(t *testing.T, f *fixture) []string {
			f.setenv("DN_HOSTNAME", fixtureHostname)
			f.setenv("DN_LISTEN_PORT", "not-a-number")
			return []string{"write-config", filepath.Join(f.tempDir(t, tokenTmpDir), "dn.env")}
		},
	}
}

// The full lifecycle: install (already satisfied), daemon, readiness, enroll,
// then the daemon's own clean exit and the shutdown unenroll. run emits no result
// envelope — its outcome is the daemon's termination — so its success triple is
// exit 0 with both streams empty.
//
// The dnclient stand-in plays both roles: `run` creates the control socket and
// blocks until enrollment marks itself done, `enroll` writes the local config the
// shutdown unenroll reads the host id from. The wait is bounded so a broken
// fixture fails the test rather than hanging it.
func runSuccess() scenario {
	return scenario{
		name:    "run",
		command: "run",
		build: func(t *testing.T, f *fixture) []string {
			work := f.tempDir(t, tokenTmpDir)
			socket := filepath.Join(work, "dnclient.sock")
			marker := filepath.Join(work, "enrolled")
			configRoot := f.tempDir(t, tokenConfigRoot)
			netDir := filepath.Join(configRoot, fixtureNetwork)

			script := fmt.Sprintf(`#!/bin/sh
case "$1" in
run)
	: > %[1]q
	i=0
	while [ ! -f %[2]q ] && [ "$i" -lt 600 ]; do
		sleep 0.05
		i=$((i+1))
	done
	;;
enroll)
	mkdir -p %[3]q
	printf 'metadata:\n  host_id: %%s\n' %[4]q > %[3]q/dnclient.yml
	: > %[2]q
	;;
esac
exit 0
`, socket, marker, netDir, fixtureHostID)

			binDir := installFixture(t, f, script, digestOf(script), lifecycleHandler(t))
			writeFakeDNClient(t, binDir, script)
			enrollEnv(f, configRoot, binDir)
			f.setenv("DN_CLIENT_VERSION", fixtureVersion)
			f.setenv("DN_CLIENT_SOCKET", socket)
			return []string{"run"}
		},
	}
}

// A first argument that names no command is a usage error (64) with the envelope
// on stderr — never the framework's own banner on stdout.
func unknownCommand() scenario {
	return scenario{
		name:    "unknown-command",
		command: "dn-tool",
		build: func(_ *testing.T, _ *fixture) []string {
			return []string{"bogus"}
		},
	}
}

func badGlobalFlag() scenario {
	return scenario{
		name:    "bad-global-flag",
		command: "dn-tool",
		build: func(_ *testing.T, _ *fixture) []string {
			return []string{"--nonexistent"}
		},
	}
}

func badSubcommandFlag() scenario {
	return scenario{
		name:    "bad-subcommand-flag",
		command: "enroll",
		build: func(_ *testing.T, _ *fixture) []string {
			return []string{"enroll", "--nonexistent"}
		},
	}
}

// A bare invocation is an incomplete command line, not a help request: exit 64
// with the envelope on stderr, because ADR 0005 keeps the usage banner off
// stdout.
func bareInvocation() scenario {
	return scenario{
		name:    "bare-invocation",
		command: "dn-tool",
		build: func(_ *testing.T, _ *fixture) []string {
			return nil
		},
	}
}

// An unknown --log-level is rejected at parse time (64) rather than falling back
// to some other verbosity.
func badLogLevel() scenario {
	return scenario{
		name:    "bad-log-level",
		command: "dn-tool",
		build: func(_ *testing.T, _ *fixture) []string {
			return []string{"--log-level", "chatty", "schema"}
		},
	}
}

// schema projects the declarations already in memory, so its golden is the
// machine-readable surface itself: a contract change shows up here as a diff.
func schemaCommand() scenario {
	return scenario{
		name:    "schema",
		command: "schema",
		build: func(_ *testing.T, _ *fixture) []string {
			return []string{"schema"}
		},
	}
}

// The pinned ADR 0005 exemption: a consumer asking for help or the version is not
// asking for a result, so both write to stdout and exit 0.
func helpFlag() scenario {
	return scenario{
		name:    "help",
		command: "dn-tool",
		build: func(_ *testing.T, _ *fixture) []string {
			return []string{"--help"}
		},
	}
}

func versionFlag() scenario {
	return scenario{
		name:    "version",
		command: "dn-tool",
		build: func(_ *testing.T, _ *fixture) []string {
			return []string{"--version"}
		},
	}
}
