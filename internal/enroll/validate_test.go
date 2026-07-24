package enroll

import (
	"context"
	"strings"
	"testing"

	"github.com/andreswebs/dn-tool/internal/config"
)

// TestValidateRoles exercises the lighthouse/relay validation gates (Req 3) as a
// pure function: mutual exclusion, lighthouse needs ≥1 static address,
// lighthouse/relay need a non-zero listen port; valid combinations pass.
func TestValidateRoles(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*config.Config)
		wantErr string // substring the error must contain; "" means expect nil
	}{
		{
			name: "both lighthouse and relay → mutually exclusive",
			mutate: func(c *config.Config) {
				c.IsLighthouse = true
				c.IsRelay = true
				c.StaticAddrs = []string{"203.0.113.1:4242"}
				c.ListenPort = 4242
			},
			wantErr: "mutually exclusive",
		},
		{
			name: "lighthouse without static address → fail",
			mutate: func(c *config.Config) {
				c.IsLighthouse = true
				c.ListenPort = 4242
			},
			wantErr: "static address",
		},
		{
			name: "lighthouse without non-zero port → fail",
			mutate: func(c *config.Config) {
				c.IsLighthouse = true
				c.StaticAddrs = []string{"203.0.113.1:4242"}
			},
			wantErr: "listen port",
		},
		{
			name: "relay without non-zero port → fail",
			mutate: func(c *config.Config) {
				c.IsRelay = true
			},
			wantErr: "listen port",
		},
		{
			name: "valid lighthouse passes",
			mutate: func(c *config.Config) {
				c.IsLighthouse = true
				c.StaticAddrs = []string{"203.0.113.1:4242"}
				c.ListenPort = 4242
			},
			wantErr: "",
		},
		{
			name: "valid relay passes",
			mutate: func(c *config.Config) {
				c.IsRelay = true
				c.ListenPort = 4242
			},
			wantErr: "",
		},
		{
			name:    "plain host (neither) passes",
			mutate:  func(_ *config.Config) {},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.mutate(cfg)

			err := validateRoles(cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateRoles returned error for valid config: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// Behavior 6: the validation gates run before any remote API call. An invalid
// role config (both lighthouse and relay) must fail without touching the API —
// failingAPI fatals on any call, and no local config is written, so without the
// gate the state machine would proceed to ListHosts.
func TestEnrollValidatesRolesBeforeRemoteCalls(t *testing.T) {
	root := t.TempDir()

	cfg := validConfig()
	cfg.NetworkName = "defined"
	cfg.IsLighthouse = true
	cfg.IsRelay = true
	cfg.StaticAddrs = []string{"203.0.113.1:4242"}
	cfg.ListenPort = 4242

	deps := Deps{API: failingAPI{t: t}, ConfigRoot: root}

	_, err := Enroll(context.Background(), cfg, deps)
	if err == nil {
		t.Fatalf("expected Enroll to fail on invalid role config")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("error %q does not name the role conflict", err.Error())
	}
}
