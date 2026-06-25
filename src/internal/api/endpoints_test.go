package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andreswebs/dn-tool/internal/config"
)

func TestCreateHostAndEnrollmentCode(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody CreateHostRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		_, _ = w.Write([]byte(`{"data":{"host":{"id":"host-abc","name":"node-1"},"enrollmentCode":{"code":"SECRET","lifetimeSeconds":86400}},"metadata":{}}`))
	}))
	defer srv.Close()

	c := New(&config.Config{APIURL: srv.URL})
	req := CreateHostRequest{
		Name:      "node-1",
		NetworkID: "network-1",
		RoleID:    "role-1",
	}
	got, err := c.CreateHostAndEnrollmentCode(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateHostAndEnrollmentCode error: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/v2/host-and-enrollment-code" {
		t.Errorf("path = %q, want /v2/host-and-enrollment-code", gotPath)
	}
	if gotBody.Name != "node-1" || gotBody.NetworkID != "network-1" || gotBody.RoleID != "role-1" {
		t.Errorf("decoded request body = %+v, want name/networkID/roleID populated", gotBody)
	}
	if got.Host.ID != "host-abc" || got.Host.Name != "node-1" {
		t.Errorf("host = %+v, want id host-abc name node-1", got.Host)
	}
	if got.EnrollmentCode.Code != "SECRET" {
		t.Errorf("enrollment code = %q, want SECRET", got.EnrollmentCode.Code)
	}
}

func TestCreateHostAndEnrollmentCodeLighthouseBody(t *testing.T) {
	var raw map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &raw)
		_, _ = w.Write([]byte(`{"data":{"host":{"id":"host-lh","name":"lh-1"},"enrollmentCode":{"code":"X"}},"metadata":{}}`))
	}))
	defer srv.Close()

	c := New(&config.Config{APIURL: srv.URL})
	req := CreateHostRequest{
		Name:            "lh-1",
		NetworkID:       "network-1",
		StaticAddresses: []string{"203.0.113.1:4242"},
		ListenPort:      4242,
		IsLighthouse:    true,
		Tags:            []string{"role:lighthouse"},
	}
	if _, err := c.CreateHostAndEnrollmentCode(context.Background(), req); err != nil {
		t.Fatalf("CreateHostAndEnrollmentCode error: %v", err)
	}

	if raw["isLighthouse"] != true {
		t.Errorf("isLighthouse = %v, want true", raw["isLighthouse"])
	}
	if raw["listenPort"].(float64) != 4242 {
		t.Errorf("listenPort = %v, want 4242", raw["listenPort"])
	}
	sa, ok := raw["staticAddresses"].([]any)
	if !ok || len(sa) != 1 || sa[0] != "203.0.113.1:4242" {
		t.Errorf("staticAddresses = %v, want [203.0.113.1:4242]", raw["staticAddresses"])
	}
}

func TestCreateHostAndEnrollmentCodeDuplicateNameError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errors":[{"code":"ERR_DUPLICATE_VALUE","message":"value already exists","path":"name"}]}`))
	}))
	defer srv.Close()

	c := New(&config.Config{APIURL: srv.URL})
	_, err := c.CreateHostAndEnrollmentCode(context.Background(), CreateHostRequest{Name: "dup", NetworkID: "network-1"})
	if err == nil {
		t.Fatal("expected error on 400 duplicate, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v is not *APIError", err)
	}
	if !apiErr.Has("ERR_DUPLICATE_VALUE", "name") {
		t.Errorf("APIError missing ERR_DUPLICATE_VALUE/name: %+v", apiErr.Errors)
	}
}

func TestListHosts(t *testing.T) {
	var gotMethod, gotPath, gotFilter string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotFilter = r.URL.Query().Get("filter.networkID")
		_, _ = w.Write([]byte(`{"data":[{"id":"host-1","name":"a"},{"id":"host-2","name":"b"}],"metadata":{"hasNextPage":false}}`))
	}))
	defer srv.Close()

	c := New(&config.Config{APIURL: srv.URL})
	hosts, err := c.ListHosts(context.Background(), "network-1")
	if err != nil {
		t.Fatalf("ListHosts error: %v", err)
	}

	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/v2/hosts" {
		t.Errorf("path = %q, want /v2/hosts", gotPath)
	}
	if gotFilter != "network-1" {
		t.Errorf("filter.networkID = %q, want network-1", gotFilter)
	}
	if len(hosts) != 2 || hosts[0].ID != "host-1" || hosts[1].Name != "b" {
		t.Errorf("hosts = %+v, want two hosts host-1/host-2", hosts)
	}
}

func TestListHostsAggregatesPages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("cursor") {
		case "":
			_, _ = w.Write([]byte(`{"data":[{"id":"host-1","name":"a"}],"metadata":{"hasNextPage":true,"nextCursor":"c1"}}`))
		default:
			_, _ = w.Write([]byte(`{"data":[{"id":"host-2","name":"b"}],"metadata":{"hasNextPage":false}}`))
		}
	}))
	defer srv.Close()

	c := New(&config.Config{APIURL: srv.URL})
	hosts, err := c.ListHosts(context.Background(), "network-1")
	if err != nil {
		t.Fatalf("ListHosts error: %v", err)
	}
	if len(hosts) != 2 || hosts[0].ID != "host-1" || hosts[1].ID != "host-2" {
		t.Errorf("hosts = %+v, want host-1 then host-2 across pages", hosts)
	}
}

func TestDeleteHost(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"data":{},"metadata":{}}`))
	}))
	defer srv.Close()

	c := New(&config.Config{APIURL: srv.URL})
	if err := c.DeleteHost(context.Background(), "host-abc"); err != nil {
		t.Fatalf("DeleteHost error: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/v1/hosts/host-abc" {
		t.Errorf("path = %q, want /v1/hosts/host-abc", gotPath)
	}
}

func TestDeleteHostNotFoundIsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":[{"code":"ERR_NOT_FOUND","message":"not found"}]}`))
	}))
	defer srv.Close()

	c := New(&config.Config{APIURL: srv.URL})
	if err := c.DeleteHost(context.Background(), "host-gone"); err != nil {
		t.Fatalf("DeleteHost on 404 = %v, want nil (idempotent already-absent)", err)
	}
}

func TestDeleteHostOtherErrorSurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":[{"code":"ERR_FORBIDDEN","message":"nope"}]}`))
	}))
	defer srv.Close()

	c := New(&config.Config{APIURL: srv.URL})
	err := c.DeleteHost(context.Background(), "host-x")
	if err == nil {
		t.Fatal("DeleteHost on 403 = nil, want error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden {
		t.Errorf("error %v, want *APIError status 403", err)
	}
}

func TestListDownloads(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"data":{"dnclient":{"0.9.5":{"linux-amd64":"https://dl/0.9.5/linux/amd64/dnclient"},"latest":{"linux-amd64":"https://dl/0.9.5/linux/amd64/dnclient"}},"versionInfo":{"latest":{"dnclient":"0.9.5"}}},"metadata":{}}`))
	}))
	defer srv.Close()

	c := New(&config.Config{APIURL: srv.URL})
	dl, err := c.ListDownloads(context.Background())
	if err != nil {
		t.Fatalf("ListDownloads error: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/v1/downloads" {
		t.Errorf("path = %q, want /v1/downloads", gotPath)
	}
	if dl.VersionInfo.Latest.DNClient != "0.9.5" {
		t.Errorf("versionInfo.latest.dnclient = %q, want 0.9.5", dl.VersionInfo.Latest.DNClient)
	}
	if url := dl.DNClient["latest"]["linux-amd64"]; url != "https://dl/0.9.5/linux/amd64/dnclient" {
		t.Errorf("latest/linux-amd64 = %q, want the dl URL", url)
	}
}
