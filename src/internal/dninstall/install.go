// Package dninstall downloads, verifies, and idempotently places the
// proprietary dnclient binary. It owns the download/checksum/atomic-rename
// machinery; it does not run the binary (dnclient) or read its local state
// (dnstate). The install target path is injected (dnclient.BinaryPath computed
// at the command boundary) so this package never imports dnclient.
package dninstall

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/andreswebs/dn-tool/internal/api"
	"github.com/andreswebs/dn-tool/internal/output"
)

// Downloader fetches the downloads table (GET /v1/downloads). *api.Client
// satisfies it; tests substitute a fake so Install needs no live API.
type Downloader interface {
	ListDownloads(ctx context.Context) (*api.Downloads, error)
}

// InstallDeps are the injected collaborators for Install: the downloads-API
// client, the resilient HTTP client used to fetch the binary + its checksum
// (api.Client.HTTPClient in production), and the resolved host Platform (the
// OS-gate / arch mapping happens at the command boundary so non-install commands
// stay runnable on non-Linux dev hosts — design note, dt-koaf).
type InstallDeps struct {
	API        Downloader
	HTTPClient *http.Client
	Platform   Platform
}

// InstallOptions are the per-invocation knobs: the full path to place the
// binary at (dnclient.BinaryPath, computed from DN_CLIENT_BIN_DIR at the command
// boundary so the same value reaches NewExecClient and Install) and the
// requested version (DN_CLIENT_VERSION; empty or "latest" selects the API's
// latest). Install derives the containing dir from BinaryPath.
type InstallOptions struct {
	BinaryPath string
	Version    string
}

// Install resolves, verifies, and idempotently places the dnclient binary,
// orchestrating resolveDownload → fetch checksum → needsInstall → (on need)
// download+verify+atomic-place. It returns Result{Action:"install"} with
// Changed=true when it wrote a new binary and Changed=false when an up-to-date
// binary was already present (which the exit layer turns into code 2 under
// --assert-changed).
//
// Verification is fail-closed throughout: the binary is only placed after its
// SHA-256 matches the published sibling checksum (downloadAndVerify), and it
// reaches its final path only via an atomic rename of a fully written, verified,
// fsynced temp file — so a download or checksum failure leaves the existing
// binary (if any) untouched and never a partial file.
func Install(ctx context.Context, deps InstallDeps, opts InstallOptions) (output.Result, error) {
	dl, err := deps.API.ListDownloads(ctx)
	if err != nil {
		return output.Result{}, fmt.Errorf("listing dnclient downloads: %w", err)
	}

	res, err := resolveDownload(dl, deps.Platform, opts.Version)
	if err != nil {
		return output.Result{}, err
	}

	expected, err := fetchChecksum(ctx, deps.HTTPClient, res.ChecksumURL)
	if err != nil {
		return output.Result{}, err
	}

	binPath := opts.BinaryPath
	need, reason, err := needsInstall(binPath, expected)
	if err != nil {
		return output.Result{}, err
	}
	slog.Default().Debug("install decision", "path", binPath, "version", res.Version, "needsInstall", need, "reason", reason)
	if !need {
		return output.Result{Action: "install", Changed: false}, nil
	}

	if err := placeBinary(ctx, deps.HTTPClient, res, binPath); err != nil {
		return output.Result{}, err
	}
	return output.Result{Action: "install", Changed: true}, nil
}

// placeBinary downloads and verifies the binary into a temp file in finalPath's
// directory, then atomically renames it onto finalPath with the executable bit
// set. The temp file shares finalPath's directory so the rename stays on one
// filesystem (atomic). It is always cleaned up on any failure before the rename,
// so a download or verify failure never leaves a partial file and finalPath only
// ever changes via the rename of a fully verified binary.
func placeBinary(ctx context.Context, httpClient *http.Client, r resolved, finalPath string) error {
	binDir := filepath.Dir(finalPath)
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("creating bin dir %s: %w", binDir, err)
	}

	tmp, err := os.CreateTemp(binDir, filepath.Base(finalPath)+".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp file in %s: %w", binDir, err)
	}
	tmpPath := tmp.Name()
	// Cleaned up on every failure path before the rename; a no-op (ENOENT) once
	// the rename has consumed tmpPath on success.
	defer func() { _ = os.Remove(tmpPath) }()

	if err := downloadAndVerify(ctx, httpClient, r, tmp); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("syncing %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", tmpPath, err)
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return fmt.Errorf("setting executable bit on %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return fmt.Errorf("placing dnclient at %s: %w", finalPath, err)
	}
	return nil
}

// needsInstall reports whether the dnclient binary at path must be (re)installed
// to match expectedDigest — the published lowercase-hex SHA-256 of the wanted
// version's binary (the same value downloadAndVerify checks the download
// against). need is true when an install is required; it is the changed signal
// the result/exit layer consumes.
//
// It is a pure local-file check with no network I/O: the caller fetches the
// published digest once and passes it in, so fail-closed download verification
// stays single-sourced in downloadAndVerify rather than split across two
// functions. (An earlier draft signature took a resolved + context and would
// have re-fetched the checksum; that conflicts with the requirement that this be
// a network-free decision, and resolved carries only the checksum URL, never the
// digest — so the digest is passed directly.)
//
// Because a given dnclient version has a fixed binary digest, digest identity is
// the combined version+integrity check: a file whose SHA-256 equals
// expectedDigest is the wanted version and is left untouched (need=false); a
// missing file or any digest difference needs a (re)download (need=true). A
// target that exists but cannot be read is a clear error — never a silent skip,
// and need is false so an unchecked error can never be read as "reinstall".
func needsInstall(path, expectedDigest string) (need bool, reason string, err error) {
	actual, err := fileSHA256(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, "no binary at target path", nil
		}
		return false, "", fmt.Errorf("inspecting installed dnclient %s: %w", path, err)
	}
	if actual == strings.ToLower(strings.TrimSpace(expectedDigest)) {
		return false, "installed binary matches expected checksum", nil
	}
	return true, "installed binary checksum differs from expected", nil
}

// fileSHA256 returns the lowercase-hex SHA-256 of the file at path, streaming it
// through the hash so a large binary is never fully buffered in memory.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
