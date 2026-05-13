package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const DefaultUserAgent = "tuip/0.1 (+https://github.com/tuipcli/tuip)"

// Client wraps http.Client with tuip defaults used by every provider.
type Client struct {
	httpClient *http.Client
	userAgent  string
}

// NewClient creates a fetch client with a timeout and default user-agent.
func NewClient(timeout time.Duration) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: timeout},
		userAgent:  DefaultUserAgent,
	}
}

// WithHTTPClient creates a fetch client from a caller-provided HTTP client.
// This is primarily useful for tests.
func WithHTTPClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	return &Client{httpClient: httpClient, userAgent: DefaultUserAgent}
}

// GetJSON performs a GET request and decodes a JSON response into target.
func (c *Client) GetJSON(ctx context.Context, url string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("request %s: unexpected status %s", url, resp.Status)
	}

	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", url, err)
	}
	return nil
}
