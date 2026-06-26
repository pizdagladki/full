package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type httpRatingsClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewHTTPRatingsClient constructs a RatingsClient that POSTs to baseURL.
func NewHTTPRatingsClient(baseURL string, httpClient *http.Client) RatingsClient {
	return &httpRatingsClient{baseURL: baseURL, httpClient: httpClient}
}

func (c *httpRatingsClient) ApplyResult(ctx context.Context, req ApplyResultRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal ratings request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/v1/matches/result", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build ratings request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("call ratings service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ratings service returned %d", resp.StatusCode)
	}

	return nil
}
