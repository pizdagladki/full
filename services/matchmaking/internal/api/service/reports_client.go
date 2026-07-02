package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type cooldownResponse struct {
	Active           bool `json:"active"`
	SecondsRemaining int  `json:"seconds_remaining"`
}

// httpReportsClient is the production ReportsClient.
type httpReportsClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewHTTPReportsClient constructs an httpReportsClient.
func NewHTTPReportsClient(baseURL string) ReportsClient {
	return &httpReportsClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// GetCooldown calls GET /v1/reports/cooldown/:user_id and returns the status.
func (c *httpReportsClient) GetCooldown(ctx context.Context, userID int64) (CooldownStatus, error) {
	url := fmt.Sprintf("%s/v1/reports/cooldown/%d", c.baseURL, userID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return CooldownStatus{}, fmt.Errorf("build cooldown request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return CooldownStatus{}, fmt.Errorf("do cooldown request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return CooldownStatus{}, fmt.Errorf("reports cooldown: unexpected status %d", resp.StatusCode)
	}

	var r cooldownResponse
	err = json.NewDecoder(resp.Body).Decode(&r)
	if err != nil {
		return CooldownStatus{}, fmt.Errorf("decode cooldown response: %w", err)
	}

	return CooldownStatus(r), nil
}
