package enroll

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/andreswebs/dn-tool/internal/api"
)

// findRemoteRecord isolates the no-name-filter list-and-match lookup (reference
// §4.2) from the orphan/force policy: a nil host means absent, and a list error
// leaves remote presence unknown so it must propagate for the caller to abort
// rather than risk creating a duplicate.
func TestFindRemoteRecord(t *testing.T) {
	t.Run("nil when the network has no hosts", func(t *testing.T) {
		got, err := findRemoteRecord(context.Background(), &scriptedAPI{t: t}, "network-1", "host-a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Errorf("got %+v, want nil for an empty network", got)
		}
	})

	t.Run("nil when no host name matches", func(t *testing.T) {
		apiMock := &scriptedAPI{t: t, hosts: []api.Host{
			{ID: "other-1", Name: "host-b"},
			{ID: "other-2", Name: "host-c"},
		}}
		got, err := findRemoteRecord(context.Background(), apiMock, "network-1", "host-a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Errorf("got %+v, want nil when no name matches", got)
		}
	})

	t.Run("returns the matching host among many", func(t *testing.T) {
		apiMock := &scriptedAPI{t: t, hosts: []api.Host{
			{ID: "other-1", Name: "host-b"},
			{ID: "match-1", Name: "host-a"},
		}}
		got, err := findRemoteRecord(context.Background(), apiMock, "network-1", "host-a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil || got.ID != "match-1" {
			t.Errorf("got %+v, want the host named host-a (id match-1)", got)
		}
	})

	t.Run("propagates a list error", func(t *testing.T) {
		apiMock := &scriptedAPI{t: t, listErr: &api.APIError{StatusCode: http.StatusForbidden}}
		got, err := findRemoteRecord(context.Background(), apiMock, "network-1", "host-a")
		if err == nil {
			t.Fatal("expected error when ListHosts fails, got nil")
		}
		var apiErr *api.APIError
		if !errors.As(err, &apiErr) {
			t.Errorf("error %v does not wrap the underlying *api.APIError", err)
		}
		if got != nil {
			t.Errorf("got %+v, want nil on list error", got)
		}
	})
}
