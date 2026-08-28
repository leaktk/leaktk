package betterleaks

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	blconfig "github.com/betterleaks/betterleaks/config"
	blsources "github.com/betterleaks/betterleaks/sources"

	"github.com/leaktk/leaktk/internal/logger"
	"github.com/leaktk/leaktk/internal/sources"
	httpclient "github.com/leaktk/leaktk/pkg/http"
)

type URL struct {
	Config           *blconfig.Config
	Sources          sources.Sources
	FetchURLPatterns []string
	MaxArchiveDepth  int
	RawURL           string
}

func (s *URL) Fragments(ctx context.Context, yield blsources.FragmentsFunc) error {
	parsedURL, err := url.Parse(s.RawURL)
	if err != nil {
		return fmt.Errorf("could not parse URL: %w", err)
	}

	client := httpclient.NewClient()
	req, err := http.NewRequestWithContext(ctx, "GET", s.RawURL, nil)
	if err != nil {
		return fmt.Errorf("error creating HTTP GET request: %w", err)
	}
	if err := s.Sources.SetHeader(req); err != nil {
		return fmt.Errorf("set header error: %w", err)
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
			Config:           s.Config,
			Sources:          s.Sources,
			FetchURLPatterns: s.FetchURLPatterns,
			MaxArchiveDepth:  s.MaxArchiveDepth,
			Path:             parsedURL.Path,
			RawMessage:       data,
		}

		return json.Fragments(ctx, yield)
	}

	file := &blsources.File{
		Config:          s.Config,
		Content:         resp.Body,
		MaxArchiveDepth: s.MaxArchiveDepth,
		Path:            parsedURL.Path,
	}

	return file.Fragments(ctx, yield)
}
