package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"github.com/andreswebs/dn-tool/internal/config"
)

// CreateHostRequest is the body of POST /v2/host-and-enrollment-code (reference
// §4.1). The enroll command (the orphan-aware state machine, design §2.4) maps
// the DN_* config into this; DN_IP_ADDRESS becomes a single IPAddresses entry on
// the v2 endpoint, which superseded the deprecated v1 scalar ipAddress shape.
type CreateHostRequest struct {
	Name            string   `json:"name"`
	NetworkID       string   `json:"networkID"`
	RoleID          string   `json:"roleID,omitempty"`
	IPAddresses     []string `json:"ipAddresses,omitempty"`
	StaticAddresses []string `json:"staticAddresses,omitempty"`
	ListenPort      int      `json:"listenPort"`
	IsLighthouse    bool     `json:"isLighthouse,omitempty"`
	IsRelay         bool     `json:"isRelay,omitempty"`
	Tags            []string `json:"tags,omitempty"`
}

// Host is the subset of the HostV2 object (reference §5) that dn-tool acts on.
// id is the remote record identifier the --force delete and unenroll paths use;
// name drives the client-side orphan match (the host-list endpoint has no name
// filter).
type Host struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	NetworkID       string   `json:"networkID"`
	RoleID          string   `json:"roleID"`
	IPAddresses     []string `json:"ipAddresses"`
	StaticAddresses []string `json:"staticAddresses"`
	ListenPort      int      `json:"listenPort"`
	IsLighthouse    bool     `json:"isLighthouse"`
	IsRelay         bool     `json:"isRelay"`
}

// EnrollmentCode is the single-use code returned alongside a created host. The
// code is sensitive — enroll hands it to `dnclient enroll` in-memory and never
// logs it (design Req 7 / upstream SEC5). Code is a config.Secret so it redacts
// structurally in any log or marshaled output; callers reach the raw value with
// Code.Reveal() only at the dnclient enroll invocation.
type EnrollmentCode struct {
	Code            config.Secret `json:"code"`
	LifetimeSeconds int64         `json:"lifetimeSeconds"`
}

// HostAndCode is the data payload of POST /v2/host-and-enrollment-code: the new
// host record plus its enrollment code.
type HostAndCode struct {
	Host           Host           `json:"host"`
	EnrollmentCode EnrollmentCode `json:"enrollmentCode"`
}

// Downloads is the data payload of GET /v1/downloads (reference §6.1): the
// dnclient URL table keyed by version then os-arch, plus the resolved latest
// version string. install resolves a URL from this.
type Downloads struct {
	DNClient    map[string]map[string]string `json:"dnclient"`
	VersionInfo VersionInfo                  `json:"versionInfo"`
}

// VersionInfo carries the downloads service's view of the latest versions.
type VersionInfo struct {
	Latest LatestVersions `json:"latest"`
}

// LatestVersions names the latest concrete version per artifact; DNClient is the
// bare version string (e.g. "0.9.5") that aliases the "latest" key.
type LatestVersions struct {
	DNClient string `json:"dnclient"`
}

// CreateHostAndEnrollmentCode creates the remote host record and a single-use
// enrollment code in one call (POST /v2/host-and-enrollment-code, reference
// §4.1). A 400 ERR_DUPLICATE_VALUE on path "name" surfaces as a *APIError; the
// enroll state machine reads it via APIError.Has as the backstop for a duplicate
// created after its primary list-and-match pre-check (a TOCTOU/skip race).
func (c *Client) CreateHostAndEnrollmentCode(ctx context.Context, req CreateHostRequest) (*HostAndCode, error) {
	var out HostAndCode
	if err := c.do(ctx, http.MethodPost, "/v2/host-and-enrollment-code", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListHosts returns every host in networkID (GET /v2/hosts?filter.networkID=…,
// reference §4.2), walking all pages. The API has no name filter, so callers
// match by name client-side.
func (c *Client) ListHosts(ctx context.Context, networkID string) ([]Host, error) {
	q := url.Values{}
	q.Set("filter.networkID", networkID)

	var hosts []Host
	err := c.listAll(ctx, "/v2/hosts", q, func(item json.RawMessage) error {
		var h Host
		if err := json.Unmarshal(item, &h); err != nil {
			return err
		}
		hosts = append(hosts, h)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return hosts, nil
}

// DeleteHost deletes the remote host record (DELETE /v1/hosts/{hostID}, reference
// §4.4 — the only host endpoint with no v2/v3 successor). A 404 is treated as
// idempotent already-absent success per the unenroll invariant (design §2.5);
// any other non-2xx surfaces as its *APIError.
func (c *Client) DeleteHost(ctx context.Context, hostID string) error {
	_, err := c.execute(ctx, http.MethodDelete, "/v1/hosts/"+hostID, nil)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return nil
		}
		return err
	}
	return nil
}

// ListDownloads returns the dnclient downloads object (GET /v1/downloads,
// reference §4.5). install resolves a binary URL for the host's os-arch and
// version from it.
func (c *Client) ListDownloads(ctx context.Context) (*Downloads, error) {
	var out Downloads
	if err := c.do(ctx, http.MethodGet, "/v1/downloads", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
