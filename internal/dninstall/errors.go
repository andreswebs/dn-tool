package dninstall

import (
	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/andreswebs/dn-tool/internal/terr"
)

// ErrUnsupportedPlatform is a host OS or CPU architecture dn-tool cannot serve
// (design §2.1: linux/amd64 and linux/arm64 only). It carries EX_CONFIG (78)
// because the mismatch is between dn-tool and the machine it was deployed on —
// an environment fault the operator resolves by deploying elsewhere, not a bug
// and not a transient failure.
var ErrUnsupportedPlatform = terr.New("unsupported_platform", output.CodeConfig,
	"run dn-tool on linux/amd64 or linux/arm64",
	"unsupported platform")

// ErrNoDownload is a DN_CLIENT_VERSION (or host platform) with no published
// dnclient binary in the downloads table. It carries EX_CONFIG (78): the
// downloads API answered correctly, and what is wrong is the requested version —
// configuration — so it is neither an API data error nor a local I/O failure.
var ErrNoDownload = terr.New("no_download_for_version", output.CodeConfig,
	"set DN_CLIENT_VERSION to a version published for this platform, or leave it unset for the latest",
	"no published dnclient download")

// ErrChecksumMismatch is a downloaded dnclient binary whose SHA-256 does not
// match the published digest. It carries EX_DATAERR (65) because the data
// received is bad; the download is discarded and no binary is placed, so the
// failure is fail-closed by construction.
var ErrChecksumMismatch = terr.New("checksum_mismatch", output.CodeData,
	"the download does not match its published checksum; retry, and treat a repeat as a supply-chain problem",
	"dnclient checksum mismatch")

// ErrChecksumMalformed is a published checksum body that is not a 64-character
// hex digest. It carries EX_DATAERR (65) for the same reason as a mismatch — the
// published data is unusable — while staying a distinct code, because a malformed
// digest points at the publisher, not at a corrupted transfer.
var ErrChecksumMalformed = terr.New("checksum_malformed", output.CodeData,
	"the published checksum is not a SHA-256 digest; retry, and report it if it persists",
	"malformed published dnclient checksum")

// ErrDownloadIO is a failure fetching or streaming the dnclient binary or its
// checksum. It carries EX_IOERR (74) rather than EX_UNAVAILABLE (69): these
// fetches go to the download host over a plain HTTP client, not to the management
// API whose reachability the 69 class describes, and the failure may equally be
// the local write of the stream.
var ErrDownloadIO = terr.New("download_failed", output.CodeIO,
	"check connectivity to the download URL in the message and retry",
	"dnclient download failed")
