package betterleaks

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/betterleaks/betterleaks/sources"

	httpclient "github.com/leaktk/leaktk/pkg/http"
	"github.com/leaktk/leaktk/pkg/logger"
)

type URL struct {
	FetchURLPatterns []string
	MaxArchiveDepth  int
	RawURL           string
	ShouldSkip       sources.SkipFunc
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
	resp, err := client.Do(req) // #nosec G704
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
			FetchURLPatterns: s.FetchURLPatterns,
			MaxArchiveDepth:  s.MaxArchiveDepth,
			Path:             parsedURL.Path,
			RawMessage:       data,
			ShouldSkip:       s.ShouldSkip,
		}
		return json.Fragments(ctx, yield)
	}

	file := &sources.File{
		Content:         resp.Body,
		MaxArchiveDepth: s.MaxArchiveDepth,
		Path:            parsedURL.Path,
		ShouldSkip:      s.ShouldSkip,
	}
	return file.Fragments(ctx, yield)
}
