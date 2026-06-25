package dninstall

import (
	"strings"
	"testing"

	"github.com/andreswebs/dn-tool/internal/api"
)

// binaryName is the placed dnclient executable's file name, asserted on by the
// install tests. Production single-sources it in dnclient.BinaryPath; the tests
// inject a full BinaryPath, so they keep a local copy of the leaf name.
const binaryName = "dnclient"

// sampleDownloads mirrors the API reference §6.1 downloads object: a per-version
// os-arch URL table plus the literal "latest" alias, and versionInfo naming the
// latest concrete version string.
func sampleDownloads() *api.Downloads {
	return &api.Downloads{
		DNClient: map[string]map[string]string{
			"0.9.5": {
				"linux-amd64": "https://dl.defined.net/764f2278/v0.9.5/linux/amd64/dnclient",
				"linux-arm64": "https://dl.defined.net/764f2278/v0.9.5/linux/arm64/dnclient",
			},
			"0.9.4": {
				"linux-amd64": "https://dl.defined.net/764f2278/v0.9.4/linux/amd64/dnclient",
			},
			"latest": {
				"linux-amd64": "https://dl.defined.net/764f2278/v0.9.5/linux/amd64/dnclient",
				"linux-arm64": "https://dl.defined.net/764f2278/v0.9.5/linux/arm64/dnclient",
			},
		},
		VersionInfo: api.VersionInfo{
			Latest: api.LatestVersions{DNClient: "0.9.5"},
		},
	}
}

func TestResolveDownloadLatest(t *testing.T) {
	dl := sampleDownloads()
	p := Platform{OS: "linux", Arch: "amd64"}
	const wantURL = "https://dl.defined.net/764f2278/v0.9.5/linux/amd64/dnclient"

	for _, version := range []string{"", "latest"} {
		t.Run("version="+version, func(t *testing.T) {
			got, err := resolveDownload(dl, p, version)
			if err != nil {
				t.Fatalf("resolveDownload(%q): unexpected error: %v", version, err)
			}
			if got.URL != wantURL {
				t.Errorf("URL = %q, want %q", got.URL, wantURL)
			}
			if got.Version != "0.9.5" {
				t.Errorf("Version = %q, want %q", got.Version, "0.9.5")
			}
		})
	}
}

func TestResolveDownloadExactVersion(t *testing.T) {
	dl := sampleDownloads()
	p := Platform{OS: "linux", Arch: "amd64"}

	got, err := resolveDownload(dl, p, "0.9.4")
	if err != nil {
		t.Fatalf("resolveDownload(0.9.4): unexpected error: %v", err)
	}
	const wantURL = "https://dl.defined.net/764f2278/v0.9.4/linux/amd64/dnclient"
	if got.URL != wantURL {
		t.Errorf("URL = %q, want %q", got.URL, wantURL)
	}
	if got.Version != "0.9.4" {
		t.Errorf("Version = %q, want %q", got.Version, "0.9.4")
	}
}

func TestResolveDownloadUnknownVersionFails(t *testing.T) {
	dl := sampleDownloads()
	p := Platform{OS: "linux", Arch: "amd64"}

	_, err := resolveDownload(dl, p, "1.2.3")
	if err == nil {
		t.Fatal("resolveDownload(1.2.3): want error, got nil")
	}
	if !strings.Contains(err.Error(), "1.2.3") {
		t.Errorf("error %q should name the unknown version %q", err.Error(), "1.2.3")
	}
}

func TestResolveDownloadUnknownPlatformFails(t *testing.T) {
	dl := sampleDownloads()
	// 0.9.4 publishes only linux-amd64, so arm64 has no download at that version.
	p := Platform{OS: "linux", Arch: "arm64"}

	_, err := resolveDownload(dl, p, "0.9.4")
	if err == nil {
		t.Fatal("resolveDownload(0.9.4, arm64): want error, got nil")
	}
	if !strings.Contains(err.Error(), "linux-arm64") {
		t.Errorf("error %q should name the platform key %q", err.Error(), "linux-arm64")
	}
}

func TestResolveDownloadChecksumURLIsSibling(t *testing.T) {
	dl := sampleDownloads()
	p := Platform{OS: "linux", Arch: "amd64"}

	got, err := resolveDownload(dl, p, "latest")
	if err != nil {
		t.Fatalf("resolveDownload: unexpected error: %v", err)
	}
	if got.ChecksumURL != got.URL+".sha256" {
		t.Errorf("ChecksumURL = %q, want %q", got.ChecksumURL, got.URL+".sha256")
	}
}

func TestResolveDownloadNoLatestReportedFails(t *testing.T) {
	dl := sampleDownloads()
	dl.VersionInfo.Latest.DNClient = ""
	p := Platform{OS: "linux", Arch: "amd64"}

	_, err := resolveDownload(dl, p, "")
	if err == nil {
		t.Fatal("resolveDownload with no latest version: want error, got nil")
	}
}
