package service

import (
	"context"
	"fmt"
	"net/http"
)

// httpMediaClient is the production MediaClient, DELETE-ing king clips
// against the media service.
type httpMediaClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewHTTPMediaClient constructs a MediaClient that targets baseURL.
func NewHTTPMediaClient(baseURL string, httpClient *http.Client) MediaClient {
	return &httpMediaClient{baseURL: baseURL, httpClient: httpClient}
}

// ExpireKingClip calls the media service's DELETE /v1/king-clips/{clipID} —
// the king-clip cleanup contract introduced in #97. A non-2xx status or a
// transport error is returned wrapped; the caller decides whether to treat
// it as fatal (the reset service treats it as non-blocking: log and
// continue).
func (c *httpMediaClient) ExpireKingClip(ctx context.Context, clipID string) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		c.baseURL+"/v1/king-clips/"+clipID, nil)
	if err != nil {
		return fmt.Errorf("build king clip expiry request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("call media service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("media service returned %d", resp.StatusCode)
	}

	return nil
}
