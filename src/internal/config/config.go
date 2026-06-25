// Package config defines the dn-tool configuration struct and loads it from
// the process environment with the §2.3 defaults.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Config holds every DN_* setting. Field defaults follow design §2.3.
type Config struct {
	APIKey          Secret        // DN_API_KEY (secret; never logged — see secret.go)
	NetworkID       string        // DN_NETWORK_ID
	RoleID          string        // DN_ROLE_ID
	NetworkName     string        // DN_NETWORK_NAME, default "defined"
	Hostname        string        // DN_HOSTNAME, default system hostname
	IPAddress       string        // DN_IP_ADDRESS, optional
	Tags            []string      // DN_TAGS (JSON string array)
	IsLighthouse    bool          // DN_IS_LIGHTHOUSE (strconv.ParseBool forms)
	IsRelay         bool          // DN_IS_RELAY (strconv.ParseBool forms)
	StaticAddrs     []string      // DN_STATIC_ADDRESSES (JSON string array)
	ListenPort      int           // DN_LISTEN_PORT, unset -> 0
	APIURL          string        // DN_API_URL, default "https://api.defined.net"
	APITimeout      time.Duration // DN_API_TIMEOUT, unset -> 0 (command picks default)
	ClientBinDir    string        // DN_CLIENT_BIN_DIR, default "/var/lib/defined/bin"
	ClientConfigDir string        // DN_CLIENT_CONFIG_DIR, default "/var/lib/defined" (dnclient config root; <network>/dnclient.yml lives here)
	ClientSocket    string        // DN_CLIENT_SOCKET, default "/var/run/defined/dnclient.<network>.sock" (dnclient control socket; run polls it before enroll)
	ClientVersion   string        // DN_CLIENT_VERSION, default "latest"
	LogLevel        string        // DN_LOG_LEVEL, default "info"

	// Module-only knobs: loaded for completeness but inert in the binary; the
	// NixOS module's stop wiring consumes them (design §2.7).
	SkipUnenroll     bool // DN_SKIP_UNENROLL
	UnenrollOnReboot bool // DN_UNENROLL_ON_REBOOT
}

// Timeout returns DN_API_TIMEOUT when set, else fallback. A zero APITimeout
// means unset, so each command supplies its own per-command default (design
// §2.3): the API-only commands a short bound, install a longer one to cover the
// binary download.
func (c *Config) Timeout(fallback time.Duration) time.Duration {
	if c.APITimeout > 0 {
		return c.APITimeout
	}
	return fallback
}

// Load reads every DN_* variable from getenv and applies the §2.3 defaults.
// getenv is injected so tests need not touch the real environment.
func Load(getenv func(string) string) (*Config, error) {
	return load(getenv, os.Hostname)
}

// load is the testable core: hostname is injected so the system-hostname
// fallback can be exercised without depending on the host.
func load(getenv func(string) string, hostname func() (string, error)) (*Config, error) {
	cfg := &Config{
		APIKey:          Secret(getenv("DN_API_KEY")),
		NetworkID:       getenv("DN_NETWORK_ID"),
		RoleID:          getenv("DN_ROLE_ID"),
		NetworkName:     orDefault(getenv("DN_NETWORK_NAME"), "defined"),
		IPAddress:       getenv("DN_IP_ADDRESS"),
		APIURL:          orDefault(getenv("DN_API_URL"), "https://api.defined.net"),
		ClientBinDir:    orDefault(getenv("DN_CLIENT_BIN_DIR"), "/var/lib/defined/bin"),
		ClientConfigDir: orDefault(getenv("DN_CLIENT_CONFIG_DIR"), "/var/lib/defined"),
		ClientVersion:   orDefault(getenv("DN_CLIENT_VERSION"), "latest"),
		LogLevel:        orDefault(getenv("DN_LOG_LEVEL"), "info"),
	}
	cfg.ClientSocket = orDefault(getenv("DN_CLIENT_SOCKET"), defaultClientSocket(cfg.NetworkName))

	h, err := resolveHostname(getenv("DN_HOSTNAME"), hostname)
	if err != nil {
		return nil, err
	}
	cfg.Hostname = h

	if cfg.IsLighthouse, err = parseBool(getenv("DN_IS_LIGHTHOUSE")); err != nil {
		return nil, fmt.Errorf("DN_IS_LIGHTHOUSE: %w", err)
	}
	if cfg.IsRelay, err = parseBool(getenv("DN_IS_RELAY")); err != nil {
		return nil, fmt.Errorf("DN_IS_RELAY: %w", err)
	}
	if cfg.SkipUnenroll, err = parseBool(getenv("DN_SKIP_UNENROLL")); err != nil {
		return nil, fmt.Errorf("DN_SKIP_UNENROLL: %w", err)
	}
	if cfg.UnenrollOnReboot, err = parseBool(getenv("DN_UNENROLL_ON_REBOOT")); err != nil {
		return nil, fmt.Errorf("DN_UNENROLL_ON_REBOOT: %w", err)
	}

	if cfg.Tags, err = parseJSONArray(getenv("DN_TAGS")); err != nil {
		return nil, fmt.Errorf("DN_TAGS: %w", err)
	}
	if cfg.StaticAddrs, err = parseJSONArray(getenv("DN_STATIC_ADDRESSES")); err != nil {
		return nil, fmt.Errorf("DN_STATIC_ADDRESSES: %w", err)
	}

	if cfg.ListenPort, err = parsePort(getenv("DN_LISTEN_PORT")); err != nil {
		return nil, fmt.Errorf("DN_LISTEN_PORT: %w", err)
	}
	if cfg.APITimeout, err = parseTimeout(getenv("DN_API_TIMEOUT")); err != nil {
		return nil, fmt.Errorf("DN_API_TIMEOUT: %w", err)
	}

	return cfg, nil
}

// defaultClientSocket is dnclient's control-socket path for networkName,
// following the daemon's convention (/var/run/defined/dnclient.<network>.sock).
// run polls this path to know the daemon is up before invoking `dnclient enroll`,
// which connects to it. DN_CLIENT_SOCKET overrides it when the daemon is run with
// a non-default socket location.
func defaultClientSocket(networkName string) string {
	return filepath.Join("/var/run/defined", "dnclient."+networkName+".sock")
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func resolveHostname(configured string, hostname func() (string, error)) (string, error) {
	if configured != "" {
		return configured, nil
	}
	h, err := hostname()
	if err != nil {
		return "", fmt.Errorf("resolving system hostname: %w", err)
	}
	return h, nil
}

func parseBool(v string) (bool, error) {
	if v == "" {
		return false, nil
	}
	return strconv.ParseBool(v)
}

func parseJSONArray(v string) ([]string, error) {
	if v == "" {
		return nil, nil
	}
	var out []string
	if err := json.Unmarshal([]byte(v), &out); err != nil {
		return nil, fmt.Errorf("not a JSON string array: %w", err)
	}
	return out, nil
}

func parsePort(v string) (int, error) {
	if v == "" {
		return 0, nil
	}
	p, err := strconv.Atoi(v)
	if err != nil {
		return 0, err
	}
	if p < 0 || p > 65535 {
		return 0, fmt.Errorf("port %d out of range 0-65535", p)
	}
	return p, nil
}

func parseTimeout(v string) (time.Duration, error) {
	if v == "" {
		return 0, nil
	}
	return time.ParseDuration(v)
}
