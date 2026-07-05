package service

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// httpMediaClient is the production MediaClient, DELETE-ing king clips
// against the media service's internal, S2S-authenticated route.
type httpMediaClient struct {
	baseURL       string
	internalToken string
	httpClient    *http.Client
}

// NewHTTPMediaClient constructs a MediaClient that targets baseURL,
// authenticating with internalToken as "Authorization: Bearer <token>"
// against the media service's internal-auth-protected expiry endpoint.
func NewHTTPMediaClient(baseURL string, internalToken string, httpClient *http.Client) MediaClient {
	return &httpMediaClient{baseURL: baseURL, internalToken: internalToken, httpClient: httpClient}
}

// ExpireKingClip calls the media service's internal
// DELETE /internal/v1/king-clips/{clipID} — the S2S-authenticated king-clip
// expiry route (#143), reached only by trusted internal callers such as the
// koth reset worker. A 404 (clip already gone) is treated as success; any
// other non-2xx status or a transport error is returned wrapped, and the
// caller decides whether to treat it as fatal (the reset service treats it
// as non-blocking: log and continue). clipID is escaped with
// url.PathEscape before being interpolated into the request path, so a
// clipID containing "/" or ".." cannot redirect the trusted internal
// bearer token to an unintended route (confused-deputy / path traversal).
func (c *httpMediaClient) ExpireKingClip(ctx context.Context, clipID string) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		c.baseURL+"/internal/v1/king-clips/"+url.PathEscape(clipID), nil)
	if err != nil {
		return fmt.Errorf("build king clip expiry request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.internalToken)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("call media service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("media service returned %d", resp.StatusCode)
	}

	return nil
}
