package dnstate

import (
	"os"
	"path/filepath"
)

// configPath returns the location of the per-network dnclient configuration,
// <configRoot>/<networkName>/dnclient.yml. configRoot is injected (default
// /etc/defined) so tests can supply a temp dir.
func configPath(configRoot, networkName string) string {
	return filepath.Join(configRoot, networkName, "dnclient.yml")
}

// ConfigExists reports whether a local dnclient configuration is present for the
// network — i.e. <configRoot>/<networkName>/dnclient.yml exists. enroll uses it
// as the "already enrolled" signal for the §2.4 state machine (row 1): when the
// config is present the host is treated as enrolled and no changes are made.
//
// This is a plain presence probe, not a parse; recovering the host_id is
// ReadHostID's job. A non-not-exist stat error (e.g. a permission failure on a
// present path) is reported as not-present, matching the probe's intent: enroll
// then proceeds down the create path rather than masking the condition here.
func ConfigExists(configRoot, networkName string) bool {
	_, err := os.Stat(configPath(configRoot, networkName))
	return err == nil
}
