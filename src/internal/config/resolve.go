package config

import (
	"fmt"
	"os"
)

// LoadWithEnvFile builds the Config from the live environment, optionally
// overlaying an env-file. When envFilePath is empty, no file is read and the
// configuration is taken solely from the live environment (getenv) and the
// §2.3 defaults. When envFilePath is set, the file is parsed as plain
// key-value data — never executed (closes upstream SEC3) — and merged beneath
// the live environment: a live variable overrides an env-file value, which in
// turn overrides a built-in default (design §2.3 precedence).
//
// A configured env-file that cannot be opened or parsed is a clear error;
// callers pass an empty path when no --env-file flag was given.
func LoadWithEnvFile(envFilePath string, getenv func(string) string) (*Config, error) {
	var fileVars map[string]string
	if envFilePath != "" {
		f, err := os.Open(envFilePath)
		if err != nil {
			return nil, fmt.Errorf("opening env-file %q: %w", envFilePath, err)
		}
		defer func() { _ = f.Close() }()
		if fileVars, err = ParseEnvFile(f); err != nil {
			return nil, fmt.Errorf("parsing env-file %q: %w", envFilePath, err)
		}
	}
	return Resolve(fileVars, getenv)
}

// Resolve merges an env-file's key-value pairs with the live process environment
// and produces the final Config. Precedence is, per design §2.3, strictly:
//
//	live environment variable > env-file value > built-in default
//
// fileVars is typically the output of ParseEnvFile; getenv is the live
// environment lookup (os.Getenv in production). The same defaulting and typing
// logic as Load is reused — Resolve only chooses the source per key.
//
// Empty-value edge case: a getenv signature of func(string) string cannot tell a
// variable explicitly set to empty (DN_X=) apart from an unset one, since both
// yield "". Resolve therefore treats an empty live value as "not set" and falls
// through to the env-file (then the default). A live variable only wins when it
// is non-empty.
func Resolve(fileVars map[string]string, getenv func(string) string) (*Config, error) {
	return resolve(fileVars, getenv, os.Hostname)
}

// resolve is the testable core: the system-hostname source is injected so the
// fallback can be exercised without depending on the host.
func resolve(fileVars map[string]string, getenv func(string) string, hostname func() (string, error)) (*Config, error) {
	merged := func(key string) string {
		if v := getenv(key); v != "" {
			return v
		}
		return fileVars[key]
	}
	return load(merged, hostname)
}
