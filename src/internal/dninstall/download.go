package dninstall

import (
	"fmt"

	"github.com/andreswebs/dn-tool/internal/api"
)

// resolved is a dnclient binary download resolved from the downloads API: the
// binary URL, the sibling checksum URL to verify it against, and the concrete
// version selected.
type resolved struct {
	URL         string // binary URL
	ChecksumURL string // sibling <URL>.sha256 (API reference §6.3)
	Version     string // concrete version selected (never the "latest" alias)
}

// downloadKey is the downloads-API os-arch table key for the platform, e.g.
// "linux-amd64" (API reference §6.2). The key construction lives here rather
// than on Platform so the data holder stays a pure mapping (see learnings
// dt-ewgz).
func (p Platform) downloadKey() string {
	return p.OS + "-" + p.Arch
}

// resolveDownload resolves the dnclient download for platform p from the
// already-fetched downloads object dl (api.ListDownloads). When version is empty
// or "latest", it selects the version the downloads API designates as latest
// (versionInfo.latest.dnclient), resolving it to a concrete version string so
// callers can compare against an installed binary; otherwise it selects the
// exact configured version. It fails clearly when the requested version has no
// entry, when the platform has no published binary for that version, or when the
// API reports no latest version. The checksum URL is the sibling <URL>.sha256
// (API reference §6.3), derived here rather than read from the downloads JSON.
//
// It performs no I/O — dl is already fetched — so it takes no context.
func resolveDownload(dl *api.Downloads, p Platform, version string) (resolved, error) {
	resolvedVersion := version
	if resolvedVersion == "" || resolvedVersion == "latest" {
		resolvedVersion = dl.VersionInfo.Latest.DNClient
		if resolvedVersion == "" {
			return resolved{}, fmt.Errorf("downloads API reported no latest dnclient version")
		}
	}

	archURLs, ok := dl.DNClient[resolvedVersion]
	if !ok {
		return resolved{}, fmt.Errorf("no dnclient download for version %q", resolvedVersion)
	}

	key := p.downloadKey()
	url, ok := archURLs[key]
	if !ok {
		return resolved{}, fmt.Errorf("no dnclient download for platform %q at version %q", key, resolvedVersion)
	}

	return resolved{
		URL:         url,
		ChecksumURL: url + ".sha256",
		Version:     resolvedVersion,
	}, nil
}
