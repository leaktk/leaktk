package betterleaks

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	blsources "github.com/betterleaks/betterleaks/v2/sources"

	"github.com/leaktk/leaktk/internal/httpclient"
	"github.com/leaktk/leaktk/internal/logger"
	"github.com/leaktk/leaktk/internal/sources"
)

type URL struct {
	FetchURLPatterns []string
	Logger           *slog.Logger
	MaxArchiveDepth  int
	RateLimit        *httpclient.RateLimit
	RawURL           string
	ShouldSkip       blsources.SkipFunc
	Sources          sources.Sources
}

func (s *URL) Fragments(ctx context.Context, yield blsources.FragmentsFunc) error {
	parsedURL, err := url.Parse(s.RawURL)
	if err != nil {
		return fmt.Errorf("could not parse URL: %w", err)
	}

	client := httpclient.NewClient()

	var resp *http.Response
	for resp == nil || resp.StatusCode == http.StatusTooManyRequests {
		req, err := http.NewRequestWithContext(ctx, "GET", s.RawURL, nil)
		if err != nil {
			return fmt.Errorf("error creating HTTP GET request: %w", err)
		}
		if err := s.Sources.SetHeader(req); err != nil {
			return fmt.Errorf("set header error: %w", err)
		}

		// Wait if needed
		if err := s.RateLimit.Wait(ctx, req); err != nil {
			return fmt.Errorf("error rate limiting requests: %w url=%s", err, req.URL)
		}

		resp, err = client.Do(req) // #nosec G704
		if err != nil {
			return fmt.Errorf("HTTP GET error: %w", err)
		}

		// Update the rate limit based on the server's response
		s.RateLimit.Update(resp)
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
			Sources:          s.Sources,
			RateLimit:        s.RateLimit,
			ShouldSkip:       s.ShouldSkip,
			FetchURLPatterns: s.FetchURLPatterns,
			MaxArchiveDepth:  s.MaxArchiveDepth,
			Path:             parsedURL.Path,
			RawMessage:       data,
		}

		return json.Fragments(ctx, yield)
	}

	file := &blsources.File{
		Content:         resp.Body,
		Logger:          s.Logger,
		MaxArchiveDepth: s.MaxArchiveDepth,
		Path:            parsedURL.Path,
		ShouldSkip:      s.ShouldSkip,
	}

	return file.Fragments(ctx, yield)
}
