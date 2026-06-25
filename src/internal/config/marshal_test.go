package config

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"
)

// emptyGetenv is a live-environment lookup that returns nothing, so a
// round-tripped config is sourced entirely from the marshaled env-file.
func emptyGetenv(string) string { return "" }

// roundTrip marshals cfg, parses the bytes back through the env-file loader, and
// resolves them with an empty live environment — the inverse path Marshal must
// satisfy.
func roundTrip(t *testing.T, cfg *Config) *Config {
	t.Helper()
	data, err := Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	vars, err := ParseEnvFile(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("ParseEnvFile(Marshal()) error = %v\noutput:\n%s", err, data)
	}
	got, err := Resolve(vars, emptyGetenv)
	if err != nil {
		t.Fatalf("Resolve(ParseEnvFile(Marshal())) error = %v\noutput:\n%s", err, data)
	}
	return got
}

func TestMarshal_RoundTrips(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
	}{
		{
			name: "fully populated",
			cfg: &Config{
				APIKey:           Secret("super-secret-key"),
				NetworkID:        "net-123",
				RoleID:           "role-456",
				NetworkName:      "defined",
				Hostname:         "host-a",
				IPAddress:        "10.0.0.5",
				Tags:             []string{"env:prod", "team:core"},
				IsLighthouse:     true,
				IsRelay:          false,
				StaticAddrs:      []string{"203.0.113.1:4242"},
				ListenPort:       4242,
				APIURL:           "https://api.defined.net",
				APITimeout:       30 * time.Second,
				ClientBinDir:     "/var/lib/defined/bin",
				ClientConfigDir:  "/etc/custom-defined",
				ClientSocket:     "/var/run/defined/dnclient.defined.sock",
				ClientVersion:    "1.2.3",
				LogLevel:         "info",
				SkipUnenroll:     true,
				UnenrollOnReboot: true,
			},
		},
		{
			name: "minimal — defaults that Load would produce",
			cfg: &Config{
				NetworkName:     "defined",
				Hostname:        "host-b",
				APIURL:          "https://api.defined.net",
				ClientBinDir:    "/var/lib/defined/bin",
				ClientConfigDir: "/etc/defined",
				ClientSocket:    "/var/run/defined/dnclient.defined.sock",
				ClientVersion:   "latest",
				LogLevel:        "info",
			},
		},
		{
			name: "relay with auto port and empty arrays",
			cfg: &Config{
				NetworkID:       "net-9",
				RoleID:          "role-9",
				NetworkName:     "defined",
				Hostname:        "relay-1",
				IsRelay:         true,
				ListenPort:      51820,
				APIURL:          "https://staging.example.test",
				ClientBinDir:    "/opt/defined/bin",
				ClientConfigDir: "/etc/defined",
				ClientSocket:    "/var/run/defined/dnclient.defined.sock",
				ClientVersion:   "latest",
				LogLevel:        "debug",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := roundTrip(t, tt.cfg)
			if !reflect.DeepEqual(got, tt.cfg) {
				t.Errorf("round trip mismatch:\n got = %+v\nwant = %+v", got, tt.cfg)
			}
		})
	}
}

func TestMarshal_JSONArraysPreserved(t *testing.T) {
	cfg := &Config{
		NetworkName:   "defined",
		Hostname:      "h",
		APIURL:        "https://api.defined.net",
		ClientBinDir:  "/var/lib/defined/bin",
		ClientVersion: "latest",
		LogLevel:      "info",
		Tags:          []string{"a", "b", "c"},
		StaticAddrs:   []string{"1.2.3.4:5000", "5.6.7.8:6000"},
	}
	got := roundTrip(t, cfg)
	if !reflect.DeepEqual(got.Tags, cfg.Tags) {
		t.Errorf("Tags round trip = %v, want %v", got.Tags, cfg.Tags)
	}
	if !reflect.DeepEqual(got.StaticAddrs, cfg.StaticAddrs) {
		t.Errorf("StaticAddrs round trip = %v, want %v", got.StaticAddrs, cfg.StaticAddrs)
	}
}

func TestMarshal_ValuesWithSpacesSurvive(t *testing.T) {
	cfg := &Config{
		NetworkName:   "defined",
		Hostname:      "my host name",
		APIURL:        "https://api.defined.net",
		ClientBinDir:  "/path/with a space/bin",
		ClientVersion: "latest",
		LogLevel:      "info",
		Tags:          []string{"owner:Jane Doe"},
	}
	got := roundTrip(t, cfg)
	if got.Hostname != cfg.Hostname {
		t.Errorf("Hostname round trip = %q, want %q", got.Hostname, cfg.Hostname)
	}
	if got.ClientBinDir != cfg.ClientBinDir {
		t.Errorf("ClientBinDir round trip = %q, want %q", got.ClientBinDir, cfg.ClientBinDir)
	}
	if !reflect.DeepEqual(got.Tags, cfg.Tags) {
		t.Errorf("Tags round trip = %v, want %v", got.Tags, cfg.Tags)
	}
}

// TestMarshal_APIKeyWritten verifies the documented decision (dt-meg1): the API
// key IS persisted to the env-file (protected later by 0600), so it must be
// revealed into the output and survive the round trip — not redacted.
func TestMarshal_APIKeyWritten(t *testing.T) {
	const key = "dn-key-abc123"
	cfg := &Config{
		APIKey:        Secret(key),
		NetworkName:   "defined",
		Hostname:      "h",
		APIURL:        "https://api.defined.net",
		ClientBinDir:  "/var/lib/defined/bin",
		ClientVersion: "latest",
		LogLevel:      "info",
	}
	data, err := Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if !strings.Contains(string(data), key) {
		t.Errorf("Marshal() output does not contain the raw API key; got:\n%s", data)
	}
	if strings.Contains(string(data), redactedMarker) {
		t.Errorf("Marshal() output contains %q — the secret was redacted, breaking round trip:\n%s", redactedMarker, data)
	}
	if got := roundTrip(t, cfg); got.APIKey.Reveal() != key {
		t.Errorf("APIKey round trip = %q, want %q", got.APIKey.Reveal(), key)
	}
}

// TestMarshal_SelfContained checks the §2.3 / behavior-4 decision: resolved
// values are emitted so the file stands alone — a known set of DN_* keys is
// always present.
func TestMarshal_SelfContained(t *testing.T) {
	cfg := &Config{
		NetworkName:   "defined",
		Hostname:      "h",
		APIURL:        "https://api.defined.net",
		ClientBinDir:  "/var/lib/defined/bin",
		ClientVersion: "latest",
		LogLevel:      "info",
	}
	data, err := Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	vars, err := ParseEnvFile(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("ParseEnvFile() error = %v", err)
	}
	for _, key := range []string{
		"DN_NETWORK_NAME", "DN_HOSTNAME", "DN_API_URL", "DN_CLIENT_BIN_DIR",
		"DN_CLIENT_CONFIG_DIR", "DN_CLIENT_VERSION", "DN_LOG_LEVEL", "DN_IS_LIGHTHOUSE",
		"DN_IS_RELAY", "DN_LISTEN_PORT", "DN_TAGS", "DN_STATIC_ADDRESSES",
	} {
		if _, ok := vars[key]; !ok {
			t.Errorf("Marshal() output missing self-contained key %q", key)
		}
	}
}
