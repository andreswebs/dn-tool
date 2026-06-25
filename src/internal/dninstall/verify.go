package dninstall

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// downloadAndVerify downloads the dnclient binary described by r and verifies it
// against the published sibling checksum (r.ChecksumURL, the <URL>.sha256 of
// API reference §6.3). Verification is mandatory and fail-closed: it fetches the
// expected digest first, so a checksum that cannot be fetched fails before the
// binary is ever downloaded, and a digest computed over the actual binary stream
// that does not match the published value returns an error. On any error the
// caller must not install whatever was written to dest.
//
// The binary is streamed into dest while its SHA-256 is computed, so the whole
// binary is never buffered in memory. Pass the resilient HTTP client
// (api.Client's StandardClient) so transient binary fetch failures are retried.
func downloadAndVerify(ctx context.Context, httpClient *http.Client, r resolved, dest io.Writer) error {
	expected, err := fetchChecksum(ctx, httpClient, r.ChecksumURL)
	if err != nil {
		return err
	}

	resp, err := get(ctx, httpClient, r.URL)
	if err != nil {
		return fmt.Errorf("downloading dnclient binary: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading dnclient binary %s: unexpected status %d", r.URL, resp.StatusCode)
	}

	hasher := sha256.New()
	if _, err := io.Copy(dest, io.TeeReader(resp.Body, hasher)); err != nil {
		return fmt.Errorf("streaming dnclient binary: %w", err)
	}

	actual := hex.EncodeToString(hasher.Sum(nil))
	if actual != expected {
		return fmt.Errorf("dnclient checksum mismatch: published %s, computed %s", expected, actual)
	}
	return nil
}

// fetchChecksum retrieves and validates the published SHA-256 digest from url.
// The body is a single lowercase 64-char hex digest (§6.3); surrounding
// whitespace is tolerated. Any fetch, read, or format failure is an error so the
// caller never proceeds to download an unverifiable binary.
func fetchChecksum(ctx context.Context, httpClient *http.Client, url string) (string, error) {
	resp, err := get(ctx, httpClient, url)
	if err != nil {
		return "", fmt.Errorf("fetching dnclient checksum: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching dnclient checksum %s: unexpected status %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading dnclient checksum: %w", err)
	}

	digest := strings.ToLower(strings.TrimSpace(string(body)))
	if _, err := hex.DecodeString(digest); err != nil || len(digest) != sha256.Size*2 {
		return "", fmt.Errorf("malformed dnclient checksum %q from %s", digest, url)
	}
	return digest, nil
}

func get(ctx context.Context, httpClient *http.Client, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return httpClient.Do(req)
}
