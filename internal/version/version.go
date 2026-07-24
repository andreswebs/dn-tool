// Package version reports the dn-tool build version, preferring a value
// stamped in at build time over the Go module build info.
package version

import "runtime/debug"

// Override is set at build time via -ldflags "-X .../version.Override=..." and
// takes precedence over the module build info when non-empty.
var Override = ""

// Current returns the build version: the ldflags Override when set, otherwise
// the module version from build info, falling back to "dev" for a bare build.
func Current() string {
	if Override != "" {
		return Override
	}
	// BuildInfo.Main.Version is populated by `go install ...@vX.Y.Z` but
	// reports "(devel)" for bare `go build`.
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}
