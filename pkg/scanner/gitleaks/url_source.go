package gitleaks

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/zricethezav/gitleaks/v8/config"
	"github.com/zricethezav/gitleaks/v8/sources"

	httpclient "github.com/leaktk/leaktk/pkg/http"
	"github.com/leaktk/leaktk/pkg/logger"
)

type URL struct {
	Config           *config.Config
	FetchURLPatterns []string
	MaxArchiveDepth  int
	RawURL           string
}

func (s *URL) Fragments(ctx context.Context, yield sources.FragmentsFunc) error {
	parsedURL, err := url.Parse(s.RawURL)
	if err != nil {
		return fmt.Errorf("could not parse URL: %w", err)
	}

	client := httpclient.NewClient()
	req, err := http.NewRequestWithContext(ctx, "GET", s.RawURL, nil)
	if err != nil {
		return fmt.Errorf("error creating HTTP GET request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP GET error: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: status_code=%d", resp.StatusCode)
	}

	defer (func() {
		if err := resp.Body.Close(); err != nil {
			logger.Debug("error closing url source response body: %v url=%q", err, s.RawURL)
		}
	})()

	if strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("could not read JSON response body: %w", err)
		}

		json := &JSON{
			Config:           s.Config,
			FetchURLPatterns: s.FetchURLPatterns,
			MaxArchiveDepth:  s.MaxArchiveDepth,
			Path:             parsedURL.Path,
			RawMessage:       data,
		}

		return json.Fragments(ctx, yield)
	}

	file := &sources.File{
		Config:          s.Config,
		Content:         resp.Body,
		MaxArchiveDepth: s.MaxArchiveDepth,
		Path:            parsedURL.Path,
	}

	return file.Fragments(ctx, yield)
}
