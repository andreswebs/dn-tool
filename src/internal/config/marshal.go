package config

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Marshal serializes a resolved Config to the KEY=VALUE env-file format that
// ParseEnvFile consumes: ParseEnvFile(Marshal(cfg)) reproduces the same
// effective config. It is the inverse of the env-file loader.
//
// Every §2.3 DN_* variable is emitted with its resolved value, so the written
// file is self-contained — a later run needs no other source to reproduce the
// configuration. JSON-array fields (DN_TAGS, DN_STATIC_ADDRESSES) are written
// as the JSON array string the loader parses back. Values that the loader would
// otherwise alter (whitespace trimmed, or a leading/trailing quote stripped) are
// wrapped in double quotes so they round-trip byte-for-byte.
//
// The API key is written in cleartext (revealed): persisting it is the
// documented behavior (dt-meg1), and the file's 0600 permissions — enforced by
// the writer, not here — are its protection. This is one of the few sanctioned
// Secret.Reveal call sites (see secret.go).
func Marshal(cfg *Config) ([]byte, error) {
	tags, err := marshalJSONArray(cfg.Tags)
	if err != nil {
		return nil, fmt.Errorf("DN_TAGS: %w", err)
	}
	staticAddrs, err := marshalJSONArray(cfg.StaticAddrs)
	if err != nil {
		return nil, fmt.Errorf("DN_STATIC_ADDRESSES: %w", err)
	}

	pairs := []struct{ key, value string }{
		{"DN_API_KEY", cfg.APIKey.Reveal()},
		{"DN_NETWORK_ID", cfg.NetworkID},
		{"DN_ROLE_ID", cfg.RoleID},
		{"DN_NETWORK_NAME", cfg.NetworkName},
		{"DN_HOSTNAME", cfg.Hostname},
		{"DN_IP_ADDRESS", cfg.IPAddress},
		{"DN_TAGS", tags},
		{"DN_IS_LIGHTHOUSE", strconv.FormatBool(cfg.IsLighthouse)},
		{"DN_IS_RELAY", strconv.FormatBool(cfg.IsRelay)},
		{"DN_STATIC_ADDRESSES", staticAddrs},
		{"DN_LISTEN_PORT", strconv.Itoa(cfg.ListenPort)},
		{"DN_API_URL", cfg.APIURL},
		{"DN_API_TIMEOUT", cfg.APITimeout.String()},
		{"DN_CLIENT_BIN_DIR", cfg.ClientBinDir},
		{"DN_CLIENT_CONFIG_DIR", cfg.ClientConfigDir},
		{"DN_CLIENT_SOCKET", cfg.ClientSocket},
		{"DN_CLIENT_VERSION", cfg.ClientVersion},
		{"DN_LOG_LEVEL", cfg.LogLevel},
		{"DN_SKIP_UNENROLL", strconv.FormatBool(cfg.SkipUnenroll)},
		{"DN_UNENROLL_ON_REBOOT", strconv.FormatBool(cfg.UnenrollOnReboot)},
	}

	var b strings.Builder
	for _, p := range pairs {
		b.WriteString(p.key)
		b.WriteByte('=')
		b.WriteString(quote(p.value))
		b.WriteByte('\n')
	}
	return []byte(b.String()), nil
}

// marshalJSONArray renders a string slice as the JSON array form the loader's
// parseJSONArray reads back. An empty/nil slice serializes to the empty string,
// which parseJSONArray maps back to a nil slice — matching a Config whose array
// field was never set (parseJSONArray("[]") would instead yield a non-nil empty
// slice, breaking the round trip).
func marshalJSONArray(v []string) (string, error) {
	if len(v) == 0 {
		return "", nil
	}
	out, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// quote wraps a value in double quotes when ParseEnvFile would otherwise not
// recover it verbatim: it trims surrounding whitespace before unquoting, and
// strips one layer of matching leading/trailing quotes. unquote removes only the
// single outermost layer, so wrapping is safe even when the value contains
// interior quotes (e.g. a JSON array of strings).
func quote(v string) string {
	if !needsQuote(v) {
		return v
	}
	return `"` + v + `"`
}

func needsQuote(v string) bool {
	if v == "" {
		return false
	}
	if strings.ContainsAny(v, " \t") {
		return true
	}
	if c := v[0]; (c == '"' || c == '\'') && v[len(v)-1] == c {
		return true
	}
	return false
}
