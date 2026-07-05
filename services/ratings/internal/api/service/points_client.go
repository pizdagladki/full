package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// httpPointsClient is the production PointsClient, POSTing credits to the
// store service's ledger endpoint.
type httpPointsClient struct {
	baseURL       string
	internalToken string
	httpClient    *http.Client
}

// NewHTTPPointsClient constructs a PointsClient that POSTs to baseURL,
// authenticating with internalToken as "Authorization: Bearer <token>"
// against the store's internal-auth-protected credit endpoint.
func NewHTTPPointsClient(baseURL string, internalToken string, httpClient *http.Client) PointsClient {
	return &httpPointsClient{baseURL: baseURL, internalToken: internalToken, httpClient: httpClient}
}

// Credit calls the store's POST /v1/points/credit. A non-2xx status or a
// transport/marshal error is returned wrapped; the caller decides whether to
// treat it as fatal (ratings treats it as non-blocking: log and continue).
func (c *httpPointsClient) Credit(ctx context.Context, req CreditRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal points credit request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/v1/points/credit", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build points credit request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.internalToken)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("call points service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("points service returned %d", resp.StatusCode)
	}

	return nil
}
