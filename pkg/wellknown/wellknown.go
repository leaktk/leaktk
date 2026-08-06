package wellknown

import (
	"net/http"
	"strings"
	"encoding/json"
	"net/url"
	"context"
)

type WellKnown struct {
	Bundles  map[string]map[string]string `json:"bundles,omitempty"`
	Patterns map[string]map[string]string `json:"patterns,omitempty"`
}

type Client struct {
	baseURL    string
	httpClient *http.Client
	authToken string
}

func NewClient(baseURL string, httpClient *http.Client, authToken string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
		authToken:  authToken,
	}
}

// Fetch fetches and parses .well-known/leaktk. Returns nil if missing or unreachable.
func (c *Client) Fetch(ctx context.Context) *WellKnown {
	wellKnownURL, err := url.JoinPath(c.baseURL, ".well-known", "leaktk")
	if err != nil {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnownURL, nil)
	if err != nil {
		return nil
	}

	if len(c.authToken) > 0 {
		req.Header.Add("Authorization", "Bearer "+c.authToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil
	}
	defer resp.Body.Close()

	var wk WellKnown
	if err := json.NewDecoder(resp.Body).Decode(&wk); err != nil {
		return nil
	}

	return &wk
}

// BundleURL resolves the bundle URL for a given release tag (e.g., "latest").
func (c *Client) BundleURL(wk *WellKnown, releaseTag, asset string) (string, bool) {
	if wk != nil && wk.Bundles != nil {
		if release, ok := wk.Bundles[releaseTag]; ok {
			if urlStr, ok := release[asset]; ok {
				return urlStr, true
			}
		}
	}
	return "", false
}

// PatternURL returns an explicit pattern URL if declared in .well-known,
// or falls back to <base-url>/patterns/<provider>/<version>.
func (c *Client) PatternURL(wk *WellKnown, provider, version string) string {
	if wk != nil && wk.Patterns != nil {
		if provMap, ok := wk.Patterns[provider]; ok {
			if urlStr, ok := provMap[version]; ok {
				return urlStr
			}
		}
	}

	fallback, _ := url.JoinPath(c.baseURL, "patterns", provider, version)
	return fallback
}
