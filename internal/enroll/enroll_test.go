package enroll

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/andreswebs/dn-tool/internal/config"
)

// validConfig returns a Config with every required enrollment parameter set,
// so individual tests can blank out one field to exercise a validation path.
func validConfig() *config.Config {
	return &config.Config{
		APIKey:      "dnkey-secret",
		NetworkID:   "network-1",
		RoleID:      "role-1",
		NetworkName: "defined",
		Hostname:    "host-a",
	}
}

// newEnrollInput carries the network name alongside the mapped request, so the
// create path no longer reaches back into cfg for it. buildCreateRequest never
// returned the network name (the v2 body omits it), so this is the new seam's
// added responsibility.
func TestNewEnrollInputCarriesNetworkName(t *testing.T) {
	cfg := validConfig()
	cfg.NetworkName = "corpnet"

	in, err := newEnrollInput(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.NetworkName != "corpnet" {
		t.Errorf("NetworkName = %q, want %q", in.NetworkName, "corpnet")
	}
	if in.Request.Name != cfg.Hostname {
		t.Errorf("Request.Name = %q, want %q (from cfg.Hostname)", in.Request.Name, cfg.Hostname)
	}
}

func TestNewEnrollInputRequiredParams(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*config.Config)
		wantNamed string // substring the error must contain
	}{
		{
			name:      "missing API key",
			mutate:    func(c *config.Config) { c.APIKey = "" },
			wantNamed: "DN_API_KEY",
		},
		{
			name:      "missing network ID",
			mutate:    func(c *config.Config) { c.NetworkID = "" },
			wantNamed: "DN_NETWORK_ID",
		},
		{
			name:      "missing role ID",
			mutate:    func(c *config.Config) { c.RoleID = "" },
			wantNamed: "DN_ROLE_ID",
		},
		{
			name: "all missing names the first one",
			mutate: func(c *config.Config) {
				c.APIKey = ""
				c.NetworkID = ""
				c.RoleID = ""
			},
			wantNamed: "DN_API_KEY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.mutate(cfg)

			_, err := newEnrollInput(cfg)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantNamed) {
				t.Fatalf("error %q does not name %q", err.Error(), tt.wantNamed)
			}
		})
	}
}

func TestNewEnrollInputMapsFields(t *testing.T) {
	cfg := validConfig()
	cfg.Hostname = "lighthouse-7"

	in, err := newEnrollInput(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	req := in.Request

	if req.Name != "lighthouse-7" {
		t.Errorf("Name = %q, want %q (from cfg.Hostname)", req.Name, "lighthouse-7")
	}
	if req.NetworkID != "network-1" {
		t.Errorf("NetworkID = %q, want %q", req.NetworkID, "network-1")
	}
	if req.RoleID != "role-1" {
		t.Errorf("RoleID = %q, want %q", req.RoleID, "role-1")
	}
}

// The v2 create-host body has no tun/device field; the network name drives the
// tun device at `dnclient enroll` time, not in this POST. Lock that invariant so
// a future change cannot smuggle DN_NETWORK_NAME into the request body.
func TestNewEnrollInputOmitsTunField(t *testing.T) {
	cfg := validConfig()
	cfg.NetworkName = "corpnet"

	in, err := newEnrollInput(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	req := in.Request

	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}

	for _, banned := range []string{"tun", "tunName", "deviceName", "networkName"} {
		if _, ok := fields[banned]; ok {
			t.Errorf("request body unexpectedly carries %q field", banned)
		}
	}
	if strings.Contains(string(b), "corpnet") {
		t.Errorf("request body leaks network name: %s", b)
	}
}

func TestNewEnrollInputStaticIPAndTags(t *testing.T) {
	t.Run("included when set", func(t *testing.T) {
		cfg := validConfig()
		cfg.IPAddress = "10.0.0.5"
		cfg.Tags = []string{"env:prod", "team:net"}

		in, err := newEnrollInput(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		req := in.Request

		if len(req.IPAddresses) != 1 || req.IPAddresses[0] != "10.0.0.5" {
			t.Errorf("IPAddresses = %v, want [10.0.0.5]", req.IPAddresses)
		}
		if len(req.Tags) != 2 {
			t.Errorf("Tags = %v, want 2 entries", req.Tags)
		}
	})

	t.Run("omitted when unset", func(t *testing.T) {
		cfg := validConfig()

		in, err := newEnrollInput(cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		req := in.Request

		if req.IPAddresses != nil {
			t.Errorf("IPAddresses = %v, want nil", req.IPAddresses)
		}
		if req.Tags != nil {
			t.Errorf("Tags = %v, want nil", req.Tags)
		}

		b, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		for _, omitted := range []string{"ipAddresses", "tags"} {
			if strings.Contains(string(b), omitted) {
				t.Errorf("unset %q should be omitted from body: %s", omitted, b)
			}
		}
	})
}
