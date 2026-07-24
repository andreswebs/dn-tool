package dninstall

import (
	"fmt"

	"github.com/andreswebs/dn-tool/internal/output"
)

// Platform holds the dnclient download keys for a host: the OS and CPU
// architecture as the downloads API names them (e.g. {OS:"linux", Arch:"amd64"}
// for the linux-amd64 binary). dn-tool is Linux-only, so OS is always "linux".
type Platform struct {
	OS   string
	Arch string
}

// supportedArch maps a Go architecture (runtime.GOARCH) to the dnclient
// downloads arch key. Only the two production server targets are supported
// (design §2.1: x86_64-linux / aarch64-linux); every other arch is rejected so
// dn-tool never installs a binary it cannot vouch for.
var supportedArch = map[string]string{
	"amd64": "amd64",
	"arm64": "arm64",
}

// DetectPlatform maps a host OS and CPU architecture to dnclient download keys.
// goos/goarch are injected (pass runtime.GOOS/runtime.GOARCH in production) so
// the mapping is fully table-testable. It fails clearly on any non-Linux OS and
// on an architecture without a supported dnclient binary, naming the offending
// value. Closes upstream S4 (the bash script read arch(1), not uname -m).
func DetectPlatform(goos, goarch string) (Platform, error) {
	// An unsupported OS or architecture is a configuration error (EX_CONFIG, ADR
	// 0001): the host dn-tool was asked to run on cannot be served.
	if goos != "linux" {
		return Platform{}, output.Classify(fmt.Errorf("unsupported operating system %q: dn-tool requires linux", goos), output.CodeConfig)
	}
	arch, ok := supportedArch[goarch]
	if !ok {
		return Platform{}, output.Classify(fmt.Errorf("unsupported architecture %q: dn-tool supports linux/amd64 and linux/arm64", goarch), output.CodeConfig)
	}
	return Platform{OS: "linux", Arch: arch}, nil
}
