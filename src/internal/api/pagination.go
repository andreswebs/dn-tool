package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// maxPageSize is the largest page the defined.net API accepts (reference §2.4).
// listAll requests it by default so callers walking a full list — e.g. enroll's
// client-side name match, which the API offers no name filter for — make as few
// round-trips as possible.
const maxPageSize = 500

// pageMetadata is the subset of the response `metadata` envelope that drives
// cursor pagination (reference §2.4).
type pageMetadata struct {
	HasNextPage bool   `json:"hasNextPage"`
	NextCursor  string `json:"nextCursor"`
}

// listAll walks every page of a cursor-paginated list endpoint, invoking each on
// the raw JSON of every item in order. It threads the response's nextCursor into
// the following request until the API reports no further pages. Caller-supplied
// query values (filters) are preserved; a pageSize is defaulted when unset. A
// failure on any page surfaces immediately rather than silently truncating the
// result, and an error from each aborts the walk.
func (c *Client) listAll(ctx context.Context, path string, q url.Values, each func(item json.RawMessage) error) error {
	query := url.Values{}
	for key, values := range q {
		for _, v := range values {
			query.Add(key, v)
		}
	}
	if query.Get("pageSize") == "" {
		query.Set("pageSize", strconv.Itoa(maxPageSize))
	}

	for {
		reqPath := path
		if encoded := query.Encode(); encoded != "" {
			reqPath = path + "?" + encoded
		}

		rawBody, err := c.execute(ctx, http.MethodGet, reqPath, nil)
		if err != nil {
			return err
		}

		var envelope struct {
			Data     []json.RawMessage `json:"data"`
			Metadata pageMetadata      `json:"metadata"`
		}
		if err := json.Unmarshal(rawBody, &envelope); err != nil {
			return fmt.Errorf("decoding list envelope: %w", err)
		}

		for _, item := range envelope.Data {
			if err := each(item); err != nil {
				return err
			}
		}

		if !envelope.Metadata.HasNextPage || envelope.Metadata.NextCursor == "" {
			return nil
		}
		query.Set("cursor", envelope.Metadata.NextCursor)
	}
}
