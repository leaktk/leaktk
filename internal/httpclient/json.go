package httpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/leaktk/leaktk/internal/auths"
	"github.com/leaktk/leaktk/pkg/logger"
)

type API struct {
	Name       string
	Auth       auths.HTTPAuth
	BaseURL    string
	Client     *http.Client
	Cursor     string
	NextCursor func(resp *http.Response) (string, error)
	RateLimit  *RateLimit
}

func (c *API) Request(ctx context.Context, respData any, method, path, query string, reqBody io.Reader) error {
	var (
		err  error
		req  *http.Request
		resp *http.Response
	)

	url = a.BaseURL + path + "?" + query

	for resp == nil || resp.StatusCode == http.StatusTooManyRequests {
		req, err = http.NewRequestWithContext(ctx, method, url, reqBody)
		if err != nil {
			return fmt.Errorf("failed create request: %w", err)
		}

		req.Header.Set("Accept", "application/json")
		if reqBody != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if err = a.auth.SetHeader(req.Header); err != nil {
			return err
		}
		// Wait if needed
		if err := a.RateLimit.Wait(ctx, req); err != nil {
			return fmt.Errorf("error rate limiting requests: %w url=%s", err, req.URL)
		}
		resp, err = client.Do(req)
		if err != nil {
			return fmt.Errorf("request failed: %w", err)
		}
		// Update the rate limit based on the server's response
		a.RateLimit.Update(resp)
	}

	respBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return fmt.Errorf("could not read complete resp body: %w", err)
	}
	if resp.StatusCode == 200 {
		if err = json.Unmarshal(respBody, respData); err != nil {
			logger.Trace("%s response body: %s", a.Name, string(respBody))
			return fmt.Errorf("decode response: %w", err)
		}
		logger.Trace("%s response data: %+v", a.Name, respData)
		return nil
	}
	logger.Trace("%s response body: %s", a.Name, string(respBody))
	return fmt.Errorf("unexpected status status_code=%d url=%s", resp.StatusCode)
}

func (c *API) Get(ctx context.Context, respData any, path, query string) error {
	return a.Request(ctx, respData, "GET", path, query, nil)
}
