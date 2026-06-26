package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type ratingsHTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewRatingsHTTPClient creates an HTTP implementation of RatingsClient.
func NewRatingsHTTPClient(baseURL string) RatingsClient {
	return &ratingsHTTPClient{
		baseURL:    baseURL,
		httpClient: &http.Client{},
	}
}

type ratingsResponse struct {
	Level int `json:"level"`
}

func (c *ratingsHTTPClient) GetLevel(ctx context.Context, userID int64) (int, error) {
	url := fmt.Sprintf("%s/v1/ratings/%d", c.baseURL, userID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("ratings service returned %d", resp.StatusCode)
	}

	var r ratingsResponse

	err = json.NewDecoder(resp.Body).Decode(&r)
	if err != nil {
		return 0, fmt.Errorf("decode response: %w", err)
	}

	return r.Level, nil
}
