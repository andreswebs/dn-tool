// Package dnstate reads local dnclient state written by the proprietary
// dnclient daemon: ConfigExists probes whether a per-network config is present
// (the enroll "already enrolled" signal), and ReadHostID recovers the remote
// host identifier unenroll needs from the per-network dnclient.yml. It is
// read-only state inspection — running the binary is dnclient, installing it is
// dninstall.
package dnstate

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ReadHostID returns the remote host identifier dnclient recorded at
// metadata.host_id in <configRoot>/<networkName>/dnclient.yml. configRoot is
// injected (default /var/lib/defined) so tests can supply a temp dir.
//
// dnclient writes host_id in two places: under metadata (alongside org_id /
// network_id — the API host identity) and under host_key (with the key
// material). metadata.host_id is the one the remote DELETE in unenroll needs, so
// it is read explicitly rather than the host_key copy.
func ReadHostID(configRoot, networkName string) (string, error) {
	path := configPath(configRoot, networkName)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%s: %w", path, ErrNotEnrolled)
		}
		return "", fmt.Errorf("reading %s: %w", path, err)
	}

	var doc struct {
		Metadata struct {
			HostID string `yaml:"host_id"`
		} `yaml:"metadata"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return "", fmt.Errorf("parsing %s: %w", path, err)
	}
	if doc.Metadata.HostID == "" {
		return "", fmt.Errorf("%s: missing metadata.host_id field", path)
	}
	return doc.Metadata.HostID, nil
}
