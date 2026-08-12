package enroll

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/andreswebs/dn-tool/internal/api"
	"github.com/andreswebs/dn-tool/internal/output"
)

// Behavior 1: local absent + remote present without --force → orphan error with
// operator guidance to re-run with --force. No delete, no create, no enroll.
func TestEnrollOrphanWithoutForceFailsWithGuidance(t *testing.T) {
	root := t.TempDir() // no local config → local absent

	cfg := validConfig()
	cfg.Hostname = "host-a"
	cfg.NetworkName = "defined"

	apiMock := &scriptedAPI{
		t:     t,
		hosts: []api.Host{{ID: "stale-1", Name: "host-a", NetworkID: "network-1"}},
	}
	enroller := &recordingEnroller{t: t}

	got, err := Enroll(context.Background(), cfg, Deps{API: apiMock, ConfigRoot: root, DNClient: enroller})
	if err == nil {
		t.Fatalf("expected orphan error without --force, got nil")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("orphan error %q does not mention --force", err.Error())
	}
	if got.Changed {
		t.Errorf("Changed = true, want false on orphan failure")
	}
	if apiMock.deleteCalls != 0 {
		t.Errorf("DeleteHost called %d times, want 0 without --force", apiMock.deleteCalls)
	}
	if apiMock.createCalls != 0 {
		t.Errorf("CreateHostAndEnrollmentCode called %d times, want 0 without --force", apiMock.createCalls)
	}
	if enroller.calls != 0 {
		t.Errorf("dnclient Enroll called %d times, want 0 without --force", enroller.calls)
	}
	// dt-2efx: an orphan is a deployment state the operator resolves, not a
	// dn-tool bug, so it exits 78 (EX_CONFIG) rather than the 70 it took while
	// the error carried no class.
	if !errors.Is(err, ErrOrphanedEnrollment) {
		t.Errorf("err = %v, want ErrOrphanedEnrollment", err)
	}
	if got := output.ExitCodeFor(err); got != output.CodeConfig {
		t.Errorf("orphan exit code = %d, want %d", got, output.CodeConfig)
	}
}

// dt-2efx: the create-time duplicate backstop is the same orphan condition
// reached through a TOCTOU race, so it carries the same code and exit class.
func TestEnrollCreateDuplicateIsCodedOrphan(t *testing.T) {
	root := t.TempDir()

	cfg := validConfig()
	cfg.Hostname = "host-a"

	apiMock := &scriptedAPI{
		t: t,
		createErr: &api.APIError{
			StatusCode: http.StatusBadRequest,
			Errors:     []api.APIErrorItem{{Code: "ERR_DUPLICATE_VALUE", Path: "name", Message: "already taken"}},
		},
	}

	_, err := Enroll(context.Background(), cfg, Deps{API: apiMock, ConfigRoot: root, DNClient: &recordingEnroller{t: t}})
	if !errors.Is(err, ErrOrphanedEnrollment) {
		t.Fatalf("err = %v, want ErrOrphanedEnrollment", err)
	}
	if got := output.ExitCodeFor(err); got != output.CodeConfig {
		t.Errorf("duplicate-orphan exit code = %d, want %d", got, output.CodeConfig)
	}
}

// Behavior 2: --force on an orphan deletes the stale record by id, then runs the
// create→code→enroll flow. Result reports Changed=true with the fresh host id,
// and the delete happens before the create.
func TestEnrollForceDeletesThenReEnrolls(t *testing.T) {
	root := t.TempDir()

	cfg := validConfig()
	cfg.Hostname = "host-a"
	cfg.NetworkName = "defined"

	apiMock := &scriptedAPI{
		t:           t,
		hosts:       []api.Host{{ID: "stale-1", Name: "host-a", NetworkID: "network-1"}},
		created:     hostAndCode("host-fresh", "CODE"),
		allowDelete: true,
	}
	enroller := &recordingEnroller{t: t}

	got, err := Enroll(context.Background(), cfg, Deps{API: apiMock, ConfigRoot: root, DNClient: enroller, Force: true})
	if err != nil {
		t.Fatalf("Enroll returned error under --force: %v", err)
	}

	if !got.Changed {
		t.Errorf("Changed = false, want true after force re-enroll")
	}
	if got.HostID != "host-fresh" {
		t.Errorf("HostID = %q, want %q (the freshly created record)", got.HostID, "host-fresh")
	}
	if apiMock.deleteCalls != 1 {
		t.Errorf("DeleteHost called %d times, want 1", apiMock.deleteCalls)
	}
	if apiMock.deletedID != "stale-1" {
		t.Errorf("DeleteHost id = %q, want %q (the matched stale id)", apiMock.deletedID, "stale-1")
	}
	if apiMock.createCalls != 1 {
		t.Errorf("CreateHostAndEnrollmentCode called %d times, want 1", apiMock.createCalls)
	}
	if !apiMock.createSawDelete {
		t.Errorf("create ran before delete: the stale record must be deleted first")
	}
	if enroller.calls != 1 {
		t.Errorf("dnclient Enroll called %d times, want 1", enroller.calls)
	}
}

// Behavior 3: a DeleteHost failure under --force aborts before create/enroll and
// surfaces the delete error.
func TestEnrollForceDeleteFailureAborts(t *testing.T) {
	root := t.TempDir()

	cfg := validConfig()
	cfg.Hostname = "host-a"
	cfg.NetworkName = "defined"

	deleteErr := &api.APIError{StatusCode: http.StatusForbidden}
	apiMock := &scriptedAPI{
		t:           t,
		hosts:       []api.Host{{ID: "stale-1", Name: "host-a", NetworkID: "network-1"}},
		allowDelete: true,
		deleteErr:   deleteErr,
	}
	enroller := &recordingEnroller{t: t}

	_, err := Enroll(context.Background(), cfg, Deps{API: apiMock, ConfigRoot: root, DNClient: enroller, Force: true})
	if err == nil {
		t.Fatalf("expected error when DeleteHost fails under --force, got nil")
	}
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		t.Errorf("error %v does not wrap *api.APIError from the delete", err)
	}
	if apiMock.createCalls != 0 {
		t.Errorf("CreateHostAndEnrollmentCode called %d times, want 0 after delete failure", apiMock.createCalls)
	}
	if enroller.calls != 0 {
		t.Errorf("dnclient Enroll called %d times, want 0 after delete failure", enroller.calls)
	}
}

// Behavior 4: --force with no matching remote record behaves exactly like the
// plain create path — no spurious DeleteHost call.
func TestEnrollForceWithoutOrphanDoesNotDelete(t *testing.T) {
	root := t.TempDir()

	cfg := validConfig()
	cfg.Hostname = "host-a"
	cfg.NetworkName = "defined"

	apiMock := &scriptedAPI{
		t:           t,
		hosts:       nil,
		created:     hostAndCode("host-new", "CODE"),
		allowDelete: true,
	}
	enroller := &recordingEnroller{t: t}

	got, err := Enroll(context.Background(), cfg, Deps{API: apiMock, ConfigRoot: root, DNClient: enroller, Force: true})
	if err != nil {
		t.Fatalf("Enroll returned error under --force on absent/absent: %v", err)
	}
	if !got.Changed {
		t.Errorf("Changed = false, want true on a fresh enroll")
	}
	if apiMock.deleteCalls != 0 {
		t.Errorf("DeleteHost called %d times, want 0 (no orphan to delete)", apiMock.deleteCalls)
	}
	if apiMock.createCalls != 1 {
		t.Errorf("CreateHostAndEnrollmentCode called %d times, want 1", apiMock.createCalls)
	}
}
