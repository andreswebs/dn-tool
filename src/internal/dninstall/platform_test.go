package dninstall

import (
	"strings"
	"testing"
)

func TestDetectPlatformSupported(t *testing.T) {
	tests := []struct {
		name          string
		goos, goarch  string
		wantOS, wantA string
	}{
		{"linux amd64", "linux", "amd64", "linux", "amd64"},
		{"linux arm64", "linux", "arm64", "linux", "arm64"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DetectPlatform(tt.goos, tt.goarch)
			if err != nil {
				t.Fatalf("DetectPlatform(%q,%q): unexpected error: %v", tt.goos, tt.goarch, err)
			}
			if got.OS != tt.wantOS || got.Arch != tt.wantA {
				t.Errorf("DetectPlatform(%q,%q) = %+v, want {OS:%q Arch:%q}",
					tt.goos, tt.goarch, got, tt.wantOS, tt.wantA)
			}
		})
	}
}

func TestDetectPlatformNonLinuxFails(t *testing.T) {
	_, err := DetectPlatform("darwin", "arm64")
	if err == nil {
		t.Fatal("DetectPlatform(darwin,arm64): want error, got nil")
	}
	if !strings.Contains(err.Error(), "darwin") {
		t.Errorf("error %q should name the OS %q", err.Error(), "darwin")
	}
}

func TestDetectPlatformUnknownArchFails(t *testing.T) {
	_, err := DetectPlatform("linux", "mips")
	if err == nil {
		t.Fatal("DetectPlatform(linux,mips): want error, got nil")
	}
	if !strings.Contains(err.Error(), "mips") {
		t.Errorf("error %q should name the arch %q", err.Error(), "mips")
	}
}
