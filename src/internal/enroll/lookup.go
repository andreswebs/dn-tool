package enroll

import (
	"context"
	"fmt"

	"github.com/andreswebs/dn-tool/internal/api"
)

// findRemoteRecord answers the §2.4 "remote present?" truth source, hiding the
// no-name-filter list-and-match workaround (reference §4.2): the host-list
// endpoint has no name filter, so the enrollment name is matched client-side. A
// nil host means absent. A list error leaves presence unknown, so it is returned
// for the caller to abort on rather than risk creating a duplicate.
//
// It does lookup only — the orphan/force decision (§2.4 rows 3-4) stays in the
// state machine, which inspects the returned record.
func findRemoteRecord(ctx context.Context, client API, networkID, name string) (*api.Host, error) {
	hosts, err := client.ListHosts(ctx, networkID)
	if err != nil {
		return nil, fmt.Errorf("listing hosts for network %s: %w", networkID, err)
	}
	for i := range hosts {
		if hosts[i].Name == name {
			return &hosts[i], nil
		}
	}
	return nil, nil
}
