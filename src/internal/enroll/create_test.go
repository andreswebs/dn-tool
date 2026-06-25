package enroll

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/andreswebs/dn-tool/internal/api"
	"github.com/andreswebs/dn-tool/internal/config"
)

// scriptedAPI is a programmable API seam for the create cell: ListHosts and
// CreateHostAndEnrollmentCode return canned values, and every call is counted so
// tests can assert ordering and the "no create on list failure / no enroll on
// create failure" guards. DeleteHost fails the test unless allowDelete is set,
// so create-cell tests keep their "must not delete" guard while the orphan/force
// tests opt into recording deletes.
type scriptedAPI struct {
	t *testing.T

	hosts     []api.Host
	listErr   error
	created   *api.HostAndCode
	createErr error

	allowDelete bool
	deleteErr   error

	listCalls       int
	createCalls     int
	deleteCalls     int
	deletedID       string
	createSawDelete bool
}

func (s *scriptedAPI) ListHosts(_ context.Context, _ string) ([]api.Host, error) {
	s.listCalls++
	return s.hosts, s.listErr
}

func (s *scriptedAPI) CreateHostAndEnrollmentCode(_ context.Context, _ api.CreateHostRequest) (*api.HostAndCode, error) {
	s.createCalls++
	s.createSawDelete = s.deleteCalls > 0
	return s.created, s.createErr
}

func (s *scriptedAPI) DeleteHost(_ context.Context, hostID string) error {
	if !s.allowDelete {
		s.t.Fatalf("DeleteHost called: the create cell must not delete records")
		return nil
	}
	s.deleteCalls++
	s.deletedID = hostID
	return s.deleteErr
}

// recordingEnroller is a mock dnclient.Client that records the single Enroll
// hand-off (network name + code) so tests can assert the code reaches the
// subprocess in-memory and that enroll is skipped on API failure. Run is
// unused by enroll.
type recordingEnroller struct {
	t *testing.T

	err error

	calls      int
	gotNetwork string
	gotCode    string
}

func (r *recordingEnroller) Enroll(_ context.Context, networkName, code string) error {
	r.calls++
	r.gotNetwork = networkName
	r.gotCode = code
	return r.err
}

func (r *recordingEnroller) Run(_ context.Context, _ ...string) error {
	r.t.Fatalf("Run called: enroll must not run the daemon")
	return nil
}

func hostAndCode(hostID, code string) *api.HostAndCode {
	return &api.HostAndCode{
		Host:           api.Host{ID: hostID, Name: "host-a", NetworkID: "network-1"},
		EnrollmentCode: api.EnrollmentCode{Code: config.Secret(code)},
	}
}

// Behavior 1: local absent + no remote record → create record, obtain code, run
// dnclient enroll with it. Result reports Changed=true with the new host's id
// and the network, and the calls happen in order (list → create → enroll).
func TestEnrollCreateCellAbsentAbsent(t *testing.T) {
	root := t.TempDir() // no local config written → local absent

	cfg := validConfig()
	cfg.NetworkName = "defined"

	apiMock := &scriptedAPI{t: t, hosts: nil, created: hostAndCode("host-xyz", "ENROLL-CODE")}
	enroller := &recordingEnroller{t: t}

	got, err := Enroll(context.Background(), cfg, Deps{API: apiMock, ConfigRoot: root, DNClient: enroller})
	if err != nil {
		t.Fatalf("Enroll returned error on absent/absent: %v", err)
	}

	if !got.Changed {
		t.Errorf("Changed = false, want true on a fresh enroll")
	}
	if got.Action != "enroll" {
		t.Errorf("Action = %q, want %q", got.Action, "enroll")
	}
	if got.HostID != "host-xyz" {
		t.Errorf("HostID = %q, want %q", got.HostID, "host-xyz")
	}
	if got.Network != "defined" {
		t.Errorf("Network = %q, want %q", got.Network, "defined")
	}

	if apiMock.listCalls != 1 {
		t.Errorf("ListHosts called %d times, want 1", apiMock.listCalls)
	}
	if apiMock.createCalls != 1 {
		t.Errorf("CreateHostAndEnrollmentCode called %d times, want 1", apiMock.createCalls)
	}
	if enroller.calls != 1 {
		t.Errorf("dnclient Enroll called %d times, want 1", enroller.calls)
	}
	if enroller.gotNetwork != "defined" {
		t.Errorf("dnclient Enroll network = %q, want %q", enroller.gotNetwork, "defined")
	}
}

// Behavior 2: ListHosts returns hosts but none match the enrollment name → the
// remote record is absent, so the create path runs.
func TestEnrollCreateCellListMatchFindsNoName(t *testing.T) {
	root := t.TempDir()

	cfg := validConfig()
	cfg.Hostname = "host-a"
	cfg.NetworkName = "defined"

	apiMock := &scriptedAPI{
		t: t,
		hosts: []api.Host{
			{ID: "other-1", Name: "host-b", NetworkID: "network-1"},
			{ID: "other-2", Name: "host-c", NetworkID: "network-1"},
		},
		created: hostAndCode("host-new", "CODE"),
	}
	enroller := &recordingEnroller{t: t}

	got, err := Enroll(context.Background(), cfg, Deps{API: apiMock, ConfigRoot: root, DNClient: enroller})
	if err != nil {
		t.Fatalf("Enroll returned error when no remote name matched: %v", err)
	}
	if !got.Changed {
		t.Errorf("Changed = false, want true (no matching remote host → create)")
	}
	if apiMock.createCalls != 1 {
		t.Errorf("CreateHostAndEnrollmentCode called %d times, want 1", apiMock.createCalls)
	}
	if enroller.calls != 1 {
		t.Errorf("dnclient Enroll called %d times, want 1", enroller.calls)
	}
}

// Behavior 3 (the B4 guard): a management-API error from create aborts the
// enrollment before `dnclient enroll` runs. The mock subprocess must never be
// called.
func TestEnrollCreateCellAPIErrorAbortsBeforeEnroll(t *testing.T) {
	root := t.TempDir()

	cfg := validConfig()
	cfg.NetworkName = "defined"

	apiMock := &scriptedAPI{
		t:         t,
		hosts:     nil,
		createErr: &api.APIError{StatusCode: http.StatusBadRequest},
	}
	enroller := &recordingEnroller{t: t}

	_, err := Enroll(context.Background(), cfg, Deps{API: apiMock, ConfigRoot: root, DNClient: enroller})
	if err == nil {
		t.Fatalf("expected error when create fails, got nil")
	}

	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		t.Errorf("error %v does not wrap *api.APIError", err)
	}
	if enroller.calls != 0 {
		t.Errorf("dnclient Enroll called %d times, want 0 (must abort before subprocess)", enroller.calls)
	}
}

// A list failure likewise aborts before any create or enroll: the remote
// presence is unknown, so the state machine cannot safely proceed.
func TestEnrollCreateCellListErrorAborts(t *testing.T) {
	root := t.TempDir()

	cfg := validConfig()
	cfg.NetworkName = "defined"

	apiMock := &scriptedAPI{t: t, listErr: &api.APIError{StatusCode: http.StatusForbidden}}
	enroller := &recordingEnroller{t: t}

	_, err := Enroll(context.Background(), cfg, Deps{API: apiMock, ConfigRoot: root, DNClient: enroller})
	if err == nil {
		t.Fatalf("expected error when ListHosts fails, got nil")
	}
	if apiMock.createCalls != 0 {
		t.Errorf("CreateHostAndEnrollmentCode called %d times, want 0 after list failure", apiMock.createCalls)
	}
	if enroller.calls != 0 {
		t.Errorf("dnclient Enroll called %d times, want 0 after list failure", enroller.calls)
	}
}

// A Create-time 400 ERR_DUPLICATE_VALUE on path "name" means a record appeared
// after the list-and-match pre-check (a TOCTOU race, or the pre-check was
// skipped). The state machine surfaces the §2.4 orphan guidance (re-run with
// --force), not the generic "creating host record" error, so the operator gets
// an actionable message. Other create errors stay wrapped generically.
func TestEnrollCreateCellDuplicateNameSurfacesOrphanGuidance(t *testing.T) {
	root := t.TempDir()

	cfg := validConfig()
	cfg.Hostname = "host-a"
	cfg.NetworkName = "defined"

	dupErr := &api.APIError{
		StatusCode: http.StatusBadRequest,
		Errors:     []api.APIErrorItem{{Code: "ERR_DUPLICATE_VALUE", Path: "name", Message: "name already in use"}},
	}
	apiMock := &scriptedAPI{t: t, hosts: nil, createErr: dupErr}
	enroller := &recordingEnroller{t: t}

	_, err := Enroll(context.Background(), cfg, Deps{API: apiMock, ConfigRoot: root, DNClient: enroller})
	if err == nil {
		t.Fatalf("expected error on a Create-time duplicate, got nil")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("duplicate error %q does not surface the orphan --force guidance", err.Error())
	}
	if !strings.Contains(err.Error(), "host-a") {
		t.Errorf("duplicate error %q does not name the host", err.Error())
	}
	if strings.Contains(err.Error(), "creating host record") {
		t.Errorf("duplicate error should not be the generic create error: %q", err.Error())
	}
	if enroller.calls != 0 {
		t.Errorf("dnclient Enroll called %d times, want 0 after create failure", enroller.calls)
	}
}

// A non-duplicate create error stays wrapped as the generic "creating host
// record" failure — the orphan guidance is reserved for the duplicate-name case.
func TestEnrollCreateCellOtherErrorStaysGeneric(t *testing.T) {
	root := t.TempDir()

	cfg := validConfig()
	cfg.NetworkName = "defined"

	apiMock := &scriptedAPI{t: t, createErr: &api.APIError{StatusCode: http.StatusInternalServerError}}
	enroller := &recordingEnroller{t: t}

	_, err := Enroll(context.Background(), cfg, Deps{API: apiMock, ConfigRoot: root, DNClient: enroller})
	if err == nil {
		t.Fatalf("expected error on a server-side create failure, got nil")
	}
	if !strings.Contains(err.Error(), "creating host record") {
		t.Errorf("non-duplicate create error %q should stay the generic create error", err.Error())
	}
	if strings.Contains(err.Error(), "--force") {
		t.Errorf("non-duplicate create error %q must not surface the orphan guidance", err.Error())
	}
}

// Behavior 4: when `dnclient enroll` exits non-zero, Enroll fails and surfaces
// the subprocess error. (This is the narrow state that produces the §2.5
// enroll-path orphan, recovered via --force in the orphan cell.)
func TestEnrollCreateCellEnrollFailureSurfaced(t *testing.T) {
	root := t.TempDir()

	cfg := validConfig()
	cfg.NetworkName = "defined"

	apiMock := &scriptedAPI{t: t, created: hostAndCode("host-xyz", "CODE")}
	enrollErr := errors.New("dnclient enroll failed: exit status 1")
	enroller := &recordingEnroller{t: t, err: enrollErr}

	_, err := Enroll(context.Background(), cfg, Deps{API: apiMock, ConfigRoot: root, DNClient: enroller})
	if err == nil {
		t.Fatalf("expected error when dnclient enroll fails, got nil")
	}
	if !errors.Is(err, enrollErr) {
		t.Errorf("error %v does not surface the dnclient enroll failure", err)
	}
}

// Behavior 5: the single-use code is handed to `dnclient enroll` in-memory and
// nowhere else. Assert the exact code returned by the API reaches the subprocess
// and that it never appears in the machine-readable Result.
func TestEnrollCreateCellCodePassedInMemory(t *testing.T) {
	root := t.TempDir()

	cfg := validConfig()
	cfg.NetworkName = "defined"

	const secretCode = "SUPER-SECRET-CODE"
	apiMock := &scriptedAPI{t: t, created: hostAndCode("host-xyz", secretCode)}
	enroller := &recordingEnroller{t: t}

	got, err := Enroll(context.Background(), cfg, Deps{API: apiMock, ConfigRoot: root, DNClient: enroller})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if enroller.gotCode != secretCode {
		t.Errorf("dnclient Enroll code = %q, want the API code %q", enroller.gotCode, secretCode)
	}
	if got.HostID == secretCode || got.Network == secretCode || got.Action == secretCode {
		t.Errorf("Result %+v leaks the enrollment code", got)
	}
}
