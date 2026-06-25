package config

import (
	"fmt"
	"os"
)

// WriteConfigFile serializes cfg to the env-file KEY=VALUE format (Marshal) and
// writes it to path, creating the file with owner-only (0600) permissions from
// the moment of creation.
//
// The 0600 mode is supplied to the single os.OpenFile call, so the file is never
// created at a broader mode and then narrowed — closing upstream SEC2, where the
// API-key file briefly existed group/world-readable between create and chmod.
// 0600 has no group or other bits, and the process umask can only clear bits, so
// the created file is exactly 0600 for any normal umask (no chmod-after needed,
// and none performed: a chmod-after is precisely the pattern SEC2 warns against).
//
// The API key is persisted in cleartext (Marshal reveals it); the file's 0600
// mode is its protection (the documented dt-meg1 decision).
func WriteConfigFile(path string, cfg *Config) error {
	data, err := Marshal(cfg)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("creating config file %s: %w", path, err)
	}

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("writing config file %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing config file %s: %w", path, err)
	}
	return nil
}
