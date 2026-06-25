// Package api is the typed client for the defined.net REST API. It centralizes
// bearer authentication and verifies the HTTP status of every response before
// reading its body (closes upstream finding D4).
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/andreswebs/dn-tool/internal/config"
	"github.com/hashicorp/go-retryablehttp"
)

// Retry defaults for the management API transport. The overall ceiling is the
// per-command DN_API_TIMEOUT (a context deadline set by the command); these
// bound the per-attempt backoff within that window (design §2.10).
const (
	defaultRetryMax     = 4
	defaultRetryWaitMin = 1 * time.Second
	defaultRetryWaitMax = 30 * time.Second
)

// APIErrorItem is one entry of the defined.net error envelope: a static machine
// code, a human message, and an optional path naming the offending field.
type APIErrorItem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path"`
}

// APIError is returned for any non-2xx management API response. It carries the
// HTTP status and the parsed error envelope so callers can branch on the static
// code/path (via Has) instead of scraping error messages — enroll detects an
// orphan primarily by list-and-match before create, and reads a Create-time 400
// ERR_DUPLICATE_VALUE at path "name" via Has as the backstop for the TOCTOU/skip
// case.
type APIError struct {
	StatusCode int
	Errors     []APIErrorItem
}

// Error implements error.
func (e *APIError) Error() string {
	if len(e.Errors) == 0 {
		return fmt.Sprintf("api error: status %d", e.StatusCode)
	}
	parts := make([]string, 0, len(e.Errors))
	for _, item := range e.Errors {
		if item.Path != "" {
			parts = append(parts, fmt.Sprintf("%s (%s): %s", item.Code, item.Path, item.Message))
		} else {
			parts = append(parts, fmt.Sprintf("%s: %s", item.Code, item.Message))
		}
	}
	return fmt.Sprintf("api error: status %d: %s", e.StatusCode, strings.Join(parts, "; "))
}

// Has reports whether the envelope contains an item with the given code and
// path.
func (e *APIError) Has(code, path string) bool {
	for _, item := range e.Errors {
		if item.Code == code && item.Path == path {
			return true
		}
	}
	return false
}

// retryPolicy classifies a response as retryable. It matches the
// retryablehttp.CheckRetry signature so dt-egz4 can wire it into the transport:
// transient failures — connection errors, 5xx, and 429 — are retried; 4xx
// client errors are terminal; a cancelled/expired context stops immediately.
func retryPolicy(ctx context.Context, resp *http.Response, err error) (bool, error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return false, ctxErr
	}
	if err != nil {
		return true, nil
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return true, nil
	}
	return false, nil
}

// slogLeveledLogger adapts a *slog.Logger to retryablehttp.LeveledLogger so the
// transport's retry chatter flows through dn-tool's structured logger instead of
// retryablehttp's default stderr writer.
type slogLeveledLogger struct {
	logger *slog.Logger
}

func (l *slogLeveledLogger) Error(msg string, kv ...any) { l.logger.Error(msg, kv...) }
func (l *slogLeveledLogger) Warn(msg string, kv ...any)  { l.logger.Warn(msg, kv...) }
func (l *slogLeveledLogger) Info(msg string, kv ...any)  { l.logger.Info(msg, kv...) }
func (l *slogLeveledLogger) Debug(msg string, kv ...any) { l.logger.Debug(msg, kv...) }

// newRetryableClient builds the retrying transport: retryPolicy classifies which
// failures are transient (connection errors, 5xx, 429) and the wait bounds the
// exponential backoff. A nil logger leaves the transport silent (no stray stderr
// noise); otherwise its retry logs are routed through slog.
func newRetryableClient(logger *slog.Logger, retryMax int, waitMin, waitMax time.Duration) *retryablehttp.Client {
	rc := retryablehttp.NewClient()
	rc.RetryMax = retryMax
	rc.RetryWaitMin = waitMin
	rc.RetryWaitMax = waitMax
	rc.CheckRetry = retryPolicy
	if logger != nil {
		rc.Logger = &slogLeveledLogger{logger: logger}
	} else {
		rc.Logger = nil
	}
	return rc
}

// Client talks to the defined.net REST API. Construct it with New; the request
// context is always passed per-call and never stored on the struct.
type Client struct {
	baseURL    string
	apiKey     config.Secret
	httpClient *http.Client
}

// New builds a Client from cfg: baseURL from cfg.APIURL, bearer auth from
// cfg.APIKey. Its HTTP transport retries transient failures with bounded
// exponential backoff (Requirement 9); the overall deadline is the context
// passed per call (set from DN_API_TIMEOUT by the command layer).
func New(cfg *config.Config) *Client {
	rc := newRetryableClient(slog.Default(), defaultRetryMax, defaultRetryWaitMin, defaultRetryWaitMax)
	return &Client{
		baseURL:    cfg.APIURL,
		apiKey:     cfg.APIKey,
		httpClient: rc.StandardClient(),
	}
}

// HTTPClient returns the resilient HTTP client (bounded retry + exponential
// backoff, Requirement 9) the API uses, so a non-API fetch that should share
// that resilience can reuse it instead of a bare http.DefaultClient. The
// dnclient binary download (dnclient.DownloadAndVerify) is the intended consumer
// — its doc calls for "api.Client's StandardClient".
func (c *Client) HTTPClient() *http.Client { return c.httpClient }

// execute issues a request to path (joined onto the base URL), authenticates with
// the bearer token, and verifies the response status before reading the body. On
// a non-2xx status it returns a typed *APIError without decoding the success
// body (upstream finding D4); on success it returns the raw response body bytes.
// A nil body sends no payload.
func (c *Client) execute(ctx context.Context, method, path string, body any) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encoding request body: %w", err)
		}
		reqBody = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey.Reveal())
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := &APIError{StatusCode: resp.StatusCode}
		if rawBody, readErr := io.ReadAll(resp.Body); readErr == nil {
			var envelope struct {
				Errors []APIErrorItem `json:"errors"`
			}
			if json.Unmarshal(rawBody, &envelope) == nil {
				apiErr.Errors = envelope.Errors
			}
		}
		return nil, fmt.Errorf("%s %s: %w", method, path, apiErr)
	}

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s %s: reading response body: %w", method, path, err)
	}
	return rawBody, nil
}

// do issues a request via execute and unwraps the {"data": …} success envelope
// into out. A nil body sends no payload; a nil out discards the response body.
// out is never populated on failure.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	rawBody, err := c.execute(ctx, method, path, body)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}

	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(rawBody, &envelope); err != nil {
		return fmt.Errorf("decoding response envelope: %w", err)
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("decoding response data: %w", err)
	}
	return nil
}
