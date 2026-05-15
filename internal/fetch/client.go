package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	resp, err := c.get(ctx, url, "application/json")
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", url, err)
	}
	return nil
}

// GetText performs a GET request and returns the response body as text.
func (c *Client) GetText(ctx context.Context, url string) (string, error) {
	resp, err := c.get(ctx, url, "text/html, text/plain;q=0.9, */*;q=0.8")
	if err != nil {
		return "", err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", url, err)
	}
	return string(data), nil
}

func (c *Client) get(ctx context.Context, url string, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %s: %w", url, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer func() {
			_ = resp.Body.Close()
		}()
		return nil, fmt.Errorf("request %s: unexpected status %s", url, resp.Status)
	}
	return resp, nil
}
