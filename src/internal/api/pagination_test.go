package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andreswebs/dn-tool/internal/config"
)

// collectIDs walks listAll over path and returns the "id" field of every item,
// in the order each was invoked.
func collectIDs(t *testing.T, c *Client, path string) []string {
	t.Helper()
	var got []string
	err := c.listAll(context.Background(), path, nil, func(item json.RawMessage) error {
		var h struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(item, &h); err != nil {
			return err
		}
		got = append(got, h.ID)
		return nil
	})
	if err != nil {
		t.Fatalf("listAll returned error: %v", err)
	}
	return got
}

func TestListAllSinglePage(t *testing.T) {
	var count int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count++
		_, _ = w.Write([]byte(`{"data":[{"id":"a"},{"id":"b"}],"metadata":{"hasNextPage":false}}`))
	}))
	defer srv.Close()

	c := New(&config.Config{APIURL: srv.URL})

	got := collectIDs(t, c, "/v2/hosts")
	if want := []string{"a", "b"}; !equalStrings(got, want) {
		t.Fatalf("items = %v, want %v", got, want)
	}
	if count != 1 {
		t.Fatalf("request count = %d, want 1", count)
	}
}

func TestListAllAggregatesPagesInOrder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("cursor") {
		case "":
			_, _ = w.Write([]byte(`{"data":[{"id":"a"},{"id":"b"}],"metadata":{"hasNextPage":true,"nextCursor":"c1"}}`))
		case "c1":
			_, _ = w.Write([]byte(`{"data":[{"id":"c"}],"metadata":{"hasNextPage":false}}`))
		default:
			t.Errorf("unexpected cursor %q", r.URL.Query().Get("cursor"))
		}
	}))
	defer srv.Close()

	c := New(&config.Config{APIURL: srv.URL})

	got := collectIDs(t, c, "/v2/hosts")
	if want := []string{"a", "b", "c"}; !equalStrings(got, want) {
		t.Fatalf("items = %v, want %v", got, want)
	}
}

func TestListAllThreadsCursor(t *testing.T) {
	var sawCursors []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawCursors = append(sawCursors, r.URL.Query().Get("cursor"))
		switch r.URL.Query().Get("cursor") {
		case "":
			_, _ = w.Write([]byte(`{"data":[{"id":"a"}],"metadata":{"hasNextPage":true,"nextCursor":"page-2-cursor"}}`))
		default:
			_, _ = w.Write([]byte(`{"data":[{"id":"b"}],"metadata":{"hasNextPage":false}}`))
		}
	}))
	defer srv.Close()

	c := New(&config.Config{APIURL: srv.URL})
	_ = collectIDs(t, c, "/v2/hosts")

	if want := []string{"", "page-2-cursor"}; !equalStrings(sawCursors, want) {
		t.Fatalf("server saw cursors %v, want %v", sawCursors, want)
	}
}

func TestListAllEmptyResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[],"metadata":{"hasNextPage":false}}`))
	}))
	defer srv.Close()

	c := New(&config.Config{APIURL: srv.URL})

	got := collectIDs(t, c, "/v2/hosts")
	if len(got) != 0 {
		t.Fatalf("items = %v, want empty", got)
	}
}

func TestListAllSurfacesErrorMidPagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("cursor") == "" {
			_, _ = w.Write([]byte(`{"data":[{"id":"a"}],"metadata":{"hasNextPage":true,"nextCursor":"c1"}}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := fastClient(srv.URL, 2)

	var got []string
	err := c.listAll(context.Background(), "/v2/hosts", nil, func(item json.RawMessage) error {
		var h struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(item, &h); err != nil {
			return err
		}
		got = append(got, h.ID)
		return nil
	})
	if err == nil {
		t.Fatal("listAll returned nil error on 5xx mid-pagination, want error")
	}
	if want := []string{"a"}; !equalStrings(got, want) {
		t.Fatalf("items before failure = %v, want %v (no silent truncation to empty)", got, want)
	}
}

func TestListAllPreservesCallerFilters(t *testing.T) {
	var gotNetworkID, gotPageSize string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotNetworkID = r.URL.Query().Get("filter.networkID")
		gotPageSize = r.URL.Query().Get("pageSize")
		_, _ = w.Write([]byte(`{"data":[],"metadata":{"hasNextPage":false}}`))
	}))
	defer srv.Close()

	c := New(&config.Config{APIURL: srv.URL})

	q := map[string][]string{"filter.networkID": {"net-1"}}
	if err := c.listAll(context.Background(), "/v2/hosts", q, func(json.RawMessage) error { return nil }); err != nil {
		t.Fatalf("listAll returned error: %v", err)
	}
	if gotNetworkID != "net-1" {
		t.Fatalf("filter.networkID = %q, want net-1", gotNetworkID)
	}
	if gotPageSize == "" {
		t.Fatal("pageSize not set; want a default to bound round-trips")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
