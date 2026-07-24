package enroll

import (
	"testing"

	"github.com/andreswebs/dn-tool/internal/config"
)

// TestNewEnrollInputRoleFields locks the lighthouse/relay/port/static-address
// mapping onto the create-host body (design Req 3). Validation (mutual exclusion,
// lighthouse-needs-address, non-zero-port) is guaranteed by validateRoles, so
// these cases assume already-valid input and only assert the request shape.
func TestNewEnrollInputRoleFields(t *testing.T) {
	tests := []struct {
		name            string
		mutate          func(*config.Config)
		wantLighthouse  bool
		wantRelay       bool
		wantStaticAddrs []string
		wantListenPort  int
	}{
		{
			name: "lighthouse carries role, static addresses and listen port",
			mutate: func(c *config.Config) {
				c.IsLighthouse = true
				c.StaticAddrs = []string{"203.0.113.1:4242"}
				c.ListenPort = 4242
			},
			wantLighthouse:  true,
			wantStaticAddrs: []string{"203.0.113.1:4242"},
			wantListenPort:  4242,
		},
		{
			name: "relay carries relay role and listen port, no static addresses",
			mutate: func(c *config.Config) {
				c.IsRelay = true
				c.ListenPort = 4242
			},
			wantRelay:      true,
			wantListenPort: 4242,
		},
		{
			name: "configured port on a plain host is passed through",
			mutate: func(c *config.Config) {
				c.ListenPort = 5151
			},
			wantListenPort: 5151,
		},
		{
			name:           "plain host with no port requests the auto-selected port",
			mutate:         func(_ *config.Config) {},
			wantListenPort: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.mutate(cfg)

			in, err := newEnrollInput(cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			req := in.Request

			if req.IsLighthouse != tt.wantLighthouse {
				t.Errorf("IsLighthouse = %v, want %v", req.IsLighthouse, tt.wantLighthouse)
			}
			if req.IsRelay != tt.wantRelay {
				t.Errorf("IsRelay = %v, want %v", req.IsRelay, tt.wantRelay)
			}
			if req.ListenPort != tt.wantListenPort {
				t.Errorf("ListenPort = %d, want %d", req.ListenPort, tt.wantListenPort)
			}
			if len(req.StaticAddresses) != len(tt.wantStaticAddrs) {
				t.Fatalf("StaticAddresses = %v, want %v", req.StaticAddresses, tt.wantStaticAddrs)
			}
			for i, want := range tt.wantStaticAddrs {
				if req.StaticAddresses[i] != want {
					t.Errorf("StaticAddresses[%d] = %q, want %q", i, req.StaticAddresses[i], want)
				}
			}
		})
	}
}

// A plain host (neither role) must not carry the lighthouse/relay fields or any
// static address — a regression guard so the role mapping never bleeds into the
// common host path.
func TestNewEnrollInputPlainHostHasNoRoleFields(t *testing.T) {
	cfg := validConfig()
	cfg.StaticAddrs = []string{"203.0.113.1:4242"} // present in config but not a lighthouse

	in, err := newEnrollInput(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	req := in.Request

	if req.IsLighthouse {
		t.Error("plain host request has IsLighthouse set")
	}
	if req.IsRelay {
		t.Error("plain host request has IsRelay set")
	}
	if req.StaticAddresses != nil {
		t.Errorf("plain host request has StaticAddresses = %v, want nil", req.StaticAddresses)
	}
}
