package config

import "testing"

// TestResolve_Precedence exercises the live-env > env-file > default order,
// covering all four cells of the precedence table plus the empty-live edge case.
func TestResolve_Precedence(t *testing.T) {
	tests := []struct {
		name     string
		fileVars map[string]string
		envVars  map[string]string
		want     string
	}{
		{
			name:     "key only in env-file",
			fileVars: map[string]string{"DN_NETWORK_ID": "file-net"},
			envVars:  nil,
			want:     "file-net",
		},
		{
			name:     "key only in live env",
			fileVars: nil,
			envVars:  map[string]string{"DN_NETWORK_ID": "env-net"},
			want:     "env-net",
		},
		{
			name:     "key in both - live env wins",
			fileVars: map[string]string{"DN_NETWORK_ID": "file-net"},
			envVars:  map[string]string{"DN_NETWORK_ID": "env-net"},
			want:     "env-net",
		},
		{
			name:     "key in neither - default applies",
			fileVars: nil,
			envVars:  nil,
			want:     "", // NetworkID has no default; stays empty
		},
		{
			name:     "empty live value falls through to env-file",
			fileVars: map[string]string{"DN_NETWORK_ID": "file-net"},
			envVars:  map[string]string{"DN_NETWORK_ID": ""},
			want:     "file-net",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := resolve(tt.fileVars, mapEnv(tt.envVars), fixedHostname("system-host"))
			if err != nil {
				t.Fatalf("resolve() error = %v", err)
			}
			if cfg.NetworkID != tt.want {
				t.Errorf("NetworkID = %q, want %q", cfg.NetworkID, tt.want)
			}
		})
	}
}

// TestResolve_DefaultedKey confirms the merge still routes through the shared
// defaulting logic: an unset, file-absent key receives its §2.3 default.
func TestResolve_DefaultedKey(t *testing.T) {
	cfg, err := resolve(nil, emptyEnv, fixedHostname("system-host"))
	if err != nil {
		t.Fatalf("resolve() error = %v", err)
	}
	if cfg.NetworkName != "defined" {
		t.Errorf("NetworkName = %q, want default %q", cfg.NetworkName, "defined")
	}
}

// TestResolve_EnvFileDefaultInteraction checks that a file value defeats the
// default while live env (unset here) defers to the file.
func TestResolve_EnvFileDefaultInteraction(t *testing.T) {
	cfg, err := resolve(
		map[string]string{"DN_NETWORK_NAME": "file-net-name"},
		emptyEnv,
		fixedHostname("system-host"),
	)
	if err != nil {
		t.Fatalf("resolve() error = %v", err)
	}
	if cfg.NetworkName != "file-net-name" {
		t.Errorf("NetworkName = %q, want %q", cfg.NetworkName, "file-net-name")
	}
}
