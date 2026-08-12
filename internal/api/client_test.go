package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/andreswebs/dn-tool/internal/config"
	"github.com/andreswebs/dn-tool/internal/output"
	"github.com/hashicorp/go-retryablehttp"
)

// fastClient builds a *Client whose transport retries with negligible backoff,
// so retry-path tests stay fast. It exercises the same newRetryableClient wiring
// New uses, only with test-sized waits.
func fastClient(baseURL string, retryMax int) *Client {
	rc := newRetryableClient(nil, retryMax, time.Millisecond, 2*time.Millisecond)
	return &Client{baseURL: baseURL, httpClient: rc.StandardClient()}
}

func TestDoSetsBearerAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"data":{},"metadata":{}}`))
	}))
	defer srv.Close()

	c := New(&config.Config{APIURL: srv.URL, APIKey: "secret-token"})

	var out struct{}
	if err := c.do(context.Background(), http.MethodGet, "/v2/hosts", nil, &out); err != nil {
		t.Fatalf("do returned error: %v", err)
	}
	if gotAuth != "Bearer secret-token" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer secret-token")
	}
}

func TestDoChecksStatusBeforeBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`this is not json at all <<<`))
	}))
	defer srv.Close()

	c := fastClient(srv.URL, 2)

	out := struct {
		ID string `json:"id"`
	}{ID: "untouched"}
	err := c.do(context.Background(), http.MethodGet, "/v2/hosts/host-1", nil, &out)
	if err == nil {
		t.Fatal("do on 500 returned nil error, want error")
	}
	if out.ID != "untouched" {
		t.Fatalf("out was populated on error: %+v, want ID unchanged", out)
	}
}

func TestDoHonorsContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{},"metadata":{}}`))
	}))
	defer srv.Close()

	c := New(&config.Config{APIURL: srv.URL})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var out struct{}
	err := c.do(ctx, http.MethodGet, "/v2/hosts", nil, &out)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("do error = %v, want context.Canceled", err)
	}
}

func TestDoSendsJSONBodyWithContentType(t *testing.T) {
	var gotCT string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"data":{},"metadata":{}}`))
	}))
	defer srv.Close()

	c := New(&config.Config{APIURL: srv.URL})

	in := map[string]string{"name": "alpha"}
	var out struct{}
	if err := c.do(context.Background(), http.MethodPost, "/v2/hosts", in, &out); err != nil {
		t.Fatalf("do returned error: %v", err)
	}
	if gotCT != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", gotCT)
	}
	if string(gotBody) != `{"name":"alpha"}` {
		t.Fatalf("request body = %q, want %q", gotBody, `{"name":"alpha"}`)
	}
}

func TestDoReturnsTypedAPIErrorWithEnvelope(t *testing.T) {
	var count int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errors":[{"code":"ERR_DUPLICATE_VALUE","message":"value already exists","path":"name"}]}`))
	}))
	defer srv.Close()

	c := New(&config.Config{APIURL: srv.URL})

	err := c.do(context.Background(), http.MethodPost, "/v2/host-and-enrollment-code", map[string]string{"name": "alpha"}, nil)
	if err == nil {
		t.Fatal("do on 400 returned nil error, want error")
	}
	if count != 1 {
		t.Fatalf("request count = %d, want 1 (4xx must not be retried)", count)
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As failed to recover *APIError from %v", err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("StatusCode = %d, want 400", apiErr.StatusCode)
	}
	if !apiErr.Has("ERR_DUPLICATE_VALUE", "name") {
		t.Fatalf("Has(ERR_DUPLICATE_VALUE, name) = false, want true; errors=%+v", apiErr.Errors)
	}
}

func TestDoClientErrorsAreTerminal(t *testing.T) {
	for _, status := range []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var count int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				count++
				w.WriteHeader(status)
			}))
			defer srv.Close()

			c := New(&config.Config{APIURL: srv.URL})

			err := c.do(context.Background(), http.MethodGet, "/v2/hosts", nil, nil)
			if err == nil {
				t.Fatalf("status %d: do returned nil error, want error", status)
			}
			if count != 1 {
				t.Fatalf("status %d: request count = %d, want 1 (4xx must not be retried)", status, count)
			}

			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("status %d: errors.As failed to recover *APIError", status)
			}
			if apiErr.StatusCode != status {
				t.Fatalf("StatusCode = %d, want %d", apiErr.StatusCode, status)
			}
		})
	}
}

func TestDoNonJSONErrorBodyStillTyped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`<html>not json at all <<<`))
	}))
	defer srv.Close()

	c := New(&config.Config{APIURL: srv.URL})

	err := c.do(context.Background(), http.MethodGet, "/v2/hosts", nil, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("errors.As failed to recover *APIError from %v", err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("StatusCode = %d, want 400", apiErr.StatusCode)
	}
	if len(apiErr.Errors) != 0 {
		t.Fatalf("Errors = %+v, want empty for non-JSON body", apiErr.Errors)
	}
}

func TestRetryPolicy(t *testing.T) {
	tests := []struct {
		name      string
		resp      *http.Response
		err       error
		wantRetry bool
	}{
		{"connection error retried", nil, errors.New("connection refused"), true},
		{"500 retried", &http.Response{StatusCode: http.StatusInternalServerError}, nil, true},
		{"503 retried", &http.Response{StatusCode: http.StatusServiceUnavailable}, nil, true},
		{"429 retried", &http.Response{StatusCode: http.StatusTooManyRequests}, nil, true},
		{"400 terminal", &http.Response{StatusCode: http.StatusBadRequest}, nil, false},
		{"401 terminal", &http.Response{StatusCode: http.StatusUnauthorized}, nil, false},
		{"404 terminal", &http.Response{StatusCode: http.StatusNotFound}, nil, false},
		{"200 no retry", &http.Response{StatusCode: http.StatusOK}, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retry, err := retryPolicy(context.Background(), tt.resp, tt.err)
			if err != nil {
				t.Fatalf("retryPolicy err = %v, want nil", err)
			}
			if retry != tt.wantRetry {
				t.Fatalf("retry = %v, want %v", retry, tt.wantRetry)
			}
		})
	}
}

func TestRetryPolicyStopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	retry, err := retryPolicy(ctx, &http.Response{StatusCode: http.StatusServiceUnavailable}, nil)
	if retry {
		t.Fatal("retry = true on cancelled context, want false")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestDoUnwrapsDataEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"host-123","name":"alpha"},"metadata":{}}`))
	}))
	defer srv.Close()

	c := New(&config.Config{APIURL: srv.URL})

	var out struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := c.do(context.Background(), http.MethodGet, "/v2/hosts/host-123", nil, &out); err != nil {
		t.Fatalf("do returned error: %v", err)
	}
	if out.ID != "host-123" || out.Name != "alpha" {
		t.Fatalf("unwrapped data = %+v, want id=host-123 name=alpha", out)
	}
}

func TestRetriesTransient5xxThenSucceeds(t *testing.T) {
	var count int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count++
		if count < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"data":{},"metadata":{}}`))
	}))
	defer srv.Close()

	c := fastClient(srv.URL, 4)

	var out struct{}
	if err := c.do(context.Background(), http.MethodGet, "/v2/hosts", nil, &out); err != nil {
		t.Fatalf("do returned error after transient 5xx: %v", err)
	}
	if count < 3 {
		t.Fatalf("request count = %d, want >= 3 (two 503s then success)", count)
	}
}

func TestRetries429ThenSucceeds(t *testing.T) {
	var count int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count++
		if count < 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"data":{},"metadata":{}}`))
	}))
	defer srv.Close()

	c := fastClient(srv.URL, 4)

	var out struct{}
	if err := c.do(context.Background(), http.MethodGet, "/v2/hosts", nil, &out); err != nil {
		t.Fatalf("do returned error after 429: %v", err)
	}
	if count < 2 {
		t.Fatalf("request count = %d, want >= 2 (429 then success)", count)
	}
}

func TestRetriesConnectionErrorThenSucceeds(t *testing.T) {
	var count int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count++
		if count < 3 {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Errorf("ResponseWriter is not a Hijacker")
				return
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Errorf("hijack: %v", err)
				return
			}
			_ = conn.Close()
			return
		}
		_, _ = w.Write([]byte(`{"data":{},"metadata":{}}`))
	}))
	defer srv.Close()

	c := fastClient(srv.URL, 4)

	var out struct{}
	if err := c.do(context.Background(), http.MethodGet, "/v2/hosts", nil, &out); err != nil {
		t.Fatalf("do returned error after connection errors: %v", err)
	}
	if count != 3 {
		t.Fatalf("request count = %d, want 3 (two aborted connections then success)", count)
	}
}

func TestOverallDeadlineBoundsRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	c := fastClient(srv.URL, 100)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := c.do(ctx, http.MethodGet, "/v2/hosts", nil, nil)
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("do error = %v, want context.DeadlineExceeded", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("elapsed = %v, want bounded near the 50ms deadline (no retry-forever)", elapsed)
	}
}

func TestRetryBudgetExhaustionSurfacesError(t *testing.T) {
	var count int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := fastClient(srv.URL, 2)

	err := c.do(context.Background(), http.MethodGet, "/v2/hosts", nil, nil)
	if err == nil {
		t.Fatal("do on persistent 503 returned nil error, want error after retry budget exhausted")
	}
	if count != 3 {
		t.Fatalf("request count = %d, want 3 (initial + RetryMax=2)", count)
	}
}

func TestNewRetryableClientWiresPolicyAndLogger(t *testing.T) {
	withLogger := newRetryableClient(slog.New(slog.DiscardHandler), 4, time.Second, 30*time.Second)
	if withLogger.CheckRetry == nil {
		t.Fatal("CheckRetry not wired")
	}
	if _, ok := withLogger.Logger.(*slogLeveledLogger); !ok {
		t.Fatalf("Logger = %T, want *slogLeveledLogger", withLogger.Logger)
	}

	noLogger := newRetryableClient(nil, 4, time.Second, 30*time.Second)
	if noLogger.Logger != nil {
		t.Fatalf("Logger = %v, want nil when no slog logger is provided", noLogger.Logger)
	}
}

// dt-2efx: a 2xx whose body does not match the documented envelope is bad data
// received, so it exits 65 (EX_DATAERR) rather than the 70 it took while the
// decode error carried no class. 70 stays reserved for dn-tool's own faults —
// encoding the request body, building the request.
func TestDoMalformedSuccessBodyIsDataError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data": not json`))
	}))
	defer srv.Close()

	var out struct{}
	err := fastClient(srv.URL, 0).do(context.Background(), http.MethodGet, "/v2/hosts", nil, &out)
	if !errors.Is(err, ErrAPIMalformedResponse) {
		t.Fatalf("err = %v, want ErrAPIMalformedResponse", err)
	}
	if got := output.ExitCodeFor(err); got != output.CodeData {
		t.Errorf("exit code = %d, want %d", got, output.CodeData)
	}
}

// The list walker decodes its own envelope, so a malformed list page carries the
// same code as a malformed single-object response.
func TestListAllMalformedEnvelopeIsDataError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data": [ broken`))
	}))
	defer srv.Close()

	err := fastClient(srv.URL, 0).listAll(context.Background(), "/v2/hosts", nil, func(json.RawMessage) error { return nil })
	if !errors.Is(err, ErrAPIMalformedResponse) {
		t.Fatalf("err = %v, want ErrAPIMalformedResponse", err)
	}
	if got := output.ExitCodeFor(err); got != output.CodeData {
		t.Errorf("exit code = %d, want %d", got, output.CodeData)
	}
}

// The API classes reach the boundary as codes too: a 4xx is ErrAPIRejected
// (EX_DATAERR, 65) and the typed *APIError stays reachable through it, so
// enroll's duplicate-name recovery still branches on the static code/path.
func TestAPIClientErrorIsCodedAndKeepsTheTypedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errors":[{"code":"ERR_DUPLICATE_VALUE","path":"name","message":"taken"}]}`))
	}))
	defer srv.Close()

	err := fastClient(srv.URL, 0).do(context.Background(), http.MethodGet, "/v2/hosts", nil, nil)
	if !errors.Is(err, ErrAPIRejected) {
		t.Fatalf("err = %v, want ErrAPIRejected", err)
	}
	if got := output.ExitCodeFor(err); got != output.CodeData {
		t.Errorf("exit code = %d, want %d", got, output.CodeData)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatal("the typed *APIError must stay reachable through the coded error")
	}
	if !apiErr.Has("ERR_DUPLICATE_VALUE", "name") {
		t.Errorf("envelope lost: %+v", apiErr)
	}
}

// A 5xx is ErrAPIUnavailable (EX_UNAVAILABLE, 69) whether it surfaces from the
// status branch or from the retrying transport giving up on it.
func TestAPIServerErrorIsCodedUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := fastClient(srv.URL, 1).do(context.Background(), http.MethodGet, "/v2/hosts", nil, nil)
	if !errors.Is(err, ErrAPIUnavailable) {
		t.Fatalf("err = %v, want ErrAPIUnavailable", err)
	}
	if got := output.ExitCodeFor(err); got != output.CodeUnavailable {
		t.Errorf("exit code = %d, want %d", got, output.CodeUnavailable)
	}
}

// dt-cr62: the transport's logger is injected by the orchestration layer instead
// of read from slog's process-global default, so nothing in the tree depends on
// slog.SetDefault.
func TestWithLogger_RoutesTransportChatterToTheInjectedLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	defer srv.Close()

	client := New(&config.Config{APIURL: srv.URL}, WithLogger(logger))
	var out struct{}
	if err := client.do(context.Background(), http.MethodGet, "/v1/thing", nil, &out); err != nil {
		t.Fatalf("do: %v", err)
	}

	if buf.Len() == 0 {
		t.Error("the injected logger received no records, want the transport's chatter")
	}
}

// A client built without the option stays silent, which is what every other test
// in this file relies on, and what keeps a library from writing to a stream it
// was never handed.
func TestNew_WithoutLoggerIsSilent(t *testing.T) {
	client := New(&config.Config{APIURL: "https://example.invalid"})

	transport, ok := client.httpClient.Transport.(*retryablehttp.RoundTripper)
	if !ok {
		t.Fatalf("transport = %T, want *retryablehttp.RoundTripper", client.httpClient.Transport)
	}
	if transport.Client.Logger != nil {
		t.Errorf("Logger = %v, want nil so a client built without WithLogger stays silent", transport.Client.Logger)
	}
}
