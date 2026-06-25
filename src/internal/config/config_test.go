package config

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// emptyEnv is a getenv that returns "" for every key.
func emptyEnv(string) string { return "" }

// mapEnv returns a getenv backed by m.
func mapEnv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// fixedHostname returns a hostname func that always yields name.
func fixedHostname(name string) func() (string, error) {
	return func() (string, error) { return name, nil }
}

func TestLoad_EnvOverrides(t *testing.T) {
	env := map[string]string{
		"DN_API_KEY":            "secret-key",
		"DN_NETWORK_ID":         "net-123",
		"DN_ROLE_ID":            "role-456",
		"DN_NETWORK_NAME":       "mynet",
		"DN_HOSTNAME":           "explicit-host",
		"DN_IP_ADDRESS":         "10.0.0.5",
		"DN_IS_LIGHTHOUSE":      "true",
		"DN_IS_RELAY":           "true",
		"DN_LISTEN_PORT":        "4242",
		"DN_API_URL":            "https://staging.example.net",
		"DN_API_TIMEOUT":        "15s",
		"DN_CLIENT_BIN_DIR":     "/opt/dn/bin",
		"DN_CLIENT_CONFIG_DIR":  "/srv/dn",
		"DN_CLIENT_SOCKET":      "/srv/run/dn.sock",
		"DN_CLIENT_VERSION":     "1.2.3",
		"DN_LOG_LEVEL":          "debug",
		"DN_SKIP_UNENROLL":      "true",
		"DN_UNENROLL_ON_REBOOT": "true",
	}
	cfg, err := load(mapEnv(env), fixedHostname("system-host"))
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}

	tests := []struct {
		name string
		got  any
		want any
	}{
		{"APIKey", cfg.APIKey.Reveal(), "secret-key"},
		{"NetworkID", cfg.NetworkID, "net-123"},
		{"RoleID", cfg.RoleID, "role-456"},
		{"NetworkName", cfg.NetworkName, "mynet"},
		{"Hostname", cfg.Hostname, "explicit-host"},
		{"IPAddress", cfg.IPAddress, "10.0.0.5"},
		{"IsLighthouse", cfg.IsLighthouse, true},
		{"IsRelay", cfg.IsRelay, true},
		{"ListenPort", cfg.ListenPort, 4242},
		{"APIURL", cfg.APIURL, "https://staging.example.net"},
		{"APITimeout", cfg.APITimeout, 15 * time.Second},
		{"ClientBinDir", cfg.ClientBinDir, "/opt/dn/bin"},
		{"ClientConfigDir", cfg.ClientConfigDir, "/srv/dn"},
		{"ClientSocket", cfg.ClientSocket, "/srv/run/dn.sock"},
		{"ClientVersion", cfg.ClientVersion, "1.2.3"},
		{"LogLevel", cfg.LogLevel, "debug"},
		{"SkipUnenroll", cfg.SkipUnenroll, true},
		{"UnenrollOnReboot", cfg.UnenrollOnReboot, true},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
		}
	}
}

func TestLoad_PublicEntryPoint(t *testing.T) {
	cfg, err := Load(emptyEnv)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.APIURL != "https://api.defined.net" {
		t.Errorf("APIURL = %q, want default", cfg.APIURL)
	}
	if cfg.Hostname == "" {
		t.Error("Hostname is empty; expected system hostname fallback")
	}
}

func TestLoad_HostnameFallback(t *testing.T) {
	cfg, err := load(emptyEnv, fixedHostname("auto-detected"))
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}
	if cfg.Hostname != "auto-detected" {
		t.Errorf("Hostname = %q, want %q", cfg.Hostname, "auto-detected")
	}
}

func TestLoad_HostnameErrorPropagates(t *testing.T) {
	failing := func() (string, error) { return "", errors.New("no hostname") }
	_, err := load(emptyEnv, failing)
	if err == nil {
		t.Fatal("load() error = nil, want hostname error")
	}
}

func TestLoad_MalformedValuesError(t *testing.T) {
	for _, key := range []string{"DN_IS_LIGHTHOUSE", "DN_LISTEN_PORT", "DN_API_TIMEOUT"} {
		t.Run(key, func(t *testing.T) {
			cfg, err := load(mapEnv(map[string]string{key: "not-valid"}), fixedHostname("h"))
			if err == nil {
				t.Fatalf("load() error = nil, want error for malformed %s", key)
			}
			if cfg != nil {
				t.Errorf("load() cfg = %v, want nil on error", cfg)
			}
		})
	}
}

func TestLoad_Defaults(t *testing.T) {
	cfg, err := load(emptyEnv, fixedHostname("host.example"))
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}

	tests := []struct {
		name string
		got  any
		want any
	}{
		{"NetworkName", cfg.NetworkName, "defined"},
		{"APIURL", cfg.APIURL, "https://api.defined.net"},
		{"ClientBinDir", cfg.ClientBinDir, "/var/lib/defined/bin"},
		{"ClientConfigDir", cfg.ClientConfigDir, "/var/lib/defined"},
		{"ClientSocket", cfg.ClientSocket, "/var/run/defined/dnclient.defined.sock"},
		{"ClientVersion", cfg.ClientVersion, "latest"},
		{"LogLevel", cfg.LogLevel, "info"},
		{"ListenPort", cfg.ListenPort, 0},
		{"Hostname", cfg.Hostname, "host.example"},
		{"APITimeout", cfg.APITimeout, time.Duration(0)},
		{"IsLighthouse", cfg.IsLighthouse, false},
		{"IsRelay", cfg.IsRelay, false},
		{"SkipUnenroll", cfg.SkipUnenroll, false},
		{"UnenrollOnReboot", cfg.UnenrollOnReboot, false},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
		}
	}
}

func TestLoad_JSONArraysParse(t *testing.T) {
	env := map[string]string{
		"DN_TAGS":             `["env:prod","team:net"]`,
		"DN_STATIC_ADDRESSES": `["1.2.3.4:4242"]`,
	}
	cfg, err := load(mapEnv(env), fixedHostname("h"))
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}
	if len(cfg.Tags) != 2 || cfg.Tags[0] != "env:prod" || cfg.Tags[1] != "team:net" {
		t.Errorf("Tags = %v, want [env:prod team:net]", cfg.Tags)
	}
	if len(cfg.StaticAddrs) != 1 || cfg.StaticAddrs[0] != "1.2.3.4:4242" {
		t.Errorf("StaticAddrs = %v, want [1.2.3.4:4242]", cfg.StaticAddrs)
	}
}

func TestLoad_JSONArrayInvalidErrorsNamingVar(t *testing.T) {
	for _, key := range []string{"DN_TAGS", "DN_STATIC_ADDRESSES"} {
		t.Run(key, func(t *testing.T) {
			cfg, err := load(mapEnv(map[string]string{key: "not json"}), fixedHostname("h"))
			if err == nil {
				t.Fatalf("load() error = nil, want error for malformed %s", key)
			}
			if !strings.Contains(err.Error(), key) {
				t.Errorf("error = %q, want it to name %q", err, key)
			}
			if cfg != nil {
				t.Errorf("cfg = %v, want nil on error", cfg)
			}
		})
	}
}

func TestLoad_JSONArrayEmptyIsNil(t *testing.T) {
	cfg, err := load(emptyEnv, fixedHostname("h"))
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}
	if cfg.Tags != nil {
		t.Errorf("Tags = %v, want nil when unset", cfg.Tags)
	}
	if cfg.StaticAddrs != nil {
		t.Errorf("StaticAddrs = %v, want nil when unset", cfg.StaticAddrs)
	}
}

func TestLoad_ClientSocketDefaultDerivesFromNetworkName(t *testing.T) {
	cfg, err := load(mapEnv(map[string]string{"DN_NETWORK_NAME": "corpnet"}), fixedHostname("h"))
	if err != nil {
		t.Fatalf("load() error = %v", err)
	}
	want := "/var/run/defined/dnclient.corpnet.sock"
	if cfg.ClientSocket != want {
		t.Errorf("ClientSocket = %q, want %q", cfg.ClientSocket, want)
	}
}

func TestLoad_PortBounds(t *testing.T) {
	tests := []struct {
		name    string
		val     string
		want    int
		wantErr bool
	}{
		{"valid mid", "4242", 4242, false},
		{"zero is auto", "0", 0, false},
		{"max", "65535", 65535, false},
		{"too high", "65536", 0, true},
		{"negative", "-1", 0, true},
		{"non-numeric", "abc", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := load(mapEnv(map[string]string{"DN_LISTEN_PORT": tt.val}), fixedHostname("h"))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("load() error = nil, want error for port %q", tt.val)
				}
				if !strings.Contains(err.Error(), "DN_LISTEN_PORT") {
					t.Errorf("error = %q, want it to name DN_LISTEN_PORT", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("load() error = %v", err)
			}
			if cfg.ListenPort != tt.want {
				t.Errorf("ListenPort = %d, want %d", cfg.ListenPort, tt.want)
			}
		})
	}
}

// Timeout returns the configured DN_API_TIMEOUT when set, and the caller's
// per-command fallback when it is unset (zero) — the "zero means unset"
// convention the field documents (§2.3).
func TestConfigTimeout(t *testing.T) {
	const fallback = 30 * time.Second

	t.Run("returns DN_API_TIMEOUT when set", func(t *testing.T) {
		cfg := &Config{APITimeout: 5 * time.Second}
		if got := cfg.Timeout(fallback); got != 5*time.Second {
			t.Errorf("Timeout = %v, want the configured 5s", got)
		}
	})

	t.Run("returns the fallback when unset", func(t *testing.T) {
		cfg := &Config{} // APITimeout is the zero value
		if got := cfg.Timeout(fallback); got != fallback {
			t.Errorf("Timeout = %v, want the %v fallback", got, fallback)
		}
	})
}
