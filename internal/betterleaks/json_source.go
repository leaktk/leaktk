package betterleaks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	blsources "github.com/betterleaks/betterleaks/v2/sources"

	"github.com/leaktk/leaktk/internal/fs"
	"github.com/leaktk/leaktk/internal/httpclient"
	"github.com/leaktk/leaktk/internal/logger"
	"github.com/leaktk/leaktk/internal/sources"
)

var urlRegexp = regexp.MustCompile(`^https?:\/\/\S+$`)

// JSON is a source for yielding fragments from strings in json data
// and from URLs contained in the data that match FetchURLPatterns
type JSON struct {
	ShouldSkip       blsources.SkipFunc
	FetchURLPatterns []string
	Logger           *slog.Logger
	MaxArchiveDepth  int
	Path             string
	RateLimit        *httpclient.RateLimit
	RawMessage       json.RawMessage
	Sources          sources.Sources
	data             any
}

type jsonNode struct {
	path  string
	value any
}

// Fragments yields the fragments contained in this resource
func (s *JSON) Fragments(ctx context.Context, yield blsources.FragmentsFunc) error {
	if s.data == nil {
		if err := json.Unmarshal([]byte(s.RawMessage), &s.data); err != nil {
			return fmt.Errorf("could not unmarshal json data: %w", err)
		}
	}

	return s.walkAndYield(ctx, jsonNode{path: s.Path, value: s.data}, yield)
}

func (s *JSON) walkAndYield(ctx context.Context, currentNode jsonNode, yield blsources.FragmentsFunc) error {
	switch obj := currentNode.value.(type) {
	case map[string]any:
		for key, value := range obj {
			childNode := jsonNode{
				path:  s.JoinPath(currentNode.path, key),
				value: value,
			}
			if err := s.walkAndYield(ctx, childNode, yield); err != nil {
				return err
			}
		}

		return nil
	case []any:
		for i, value := range obj {
			childNode := jsonNode{
				path:  s.JoinPath(currentNode.path, strconv.Itoa(i)),
				value: value,
			}
			if err := s.walkAndYield(ctx, childNode, yield); err != nil {
				return err
			}
		}

		return nil
	case string:
		if s.shouldFetchURL(currentNode.path) && urlRegexp.MatchString(obj) {
			client := httpclient.NewClient()
			var resp *http.Response
			for resp == nil || resp.StatusCode == http.StatusTooManyRequests {
				req, err := http.NewRequestWithContext(ctx, "GET", obj, nil)
				if err != nil {
					logger.Error("json fetch url failed: %v path=%q", err, currentNode.path)
					return nil
				}

				if err := s.Sources.SetHeader(req); err != nil {
					logger.Error("json fetch url failed: set header: %v path=%q", err, currentNode.path)
					return nil
				}

				// Wait if needed
				if err := s.RateLimit.Wait(ctx, req); err != nil {
					return fmt.Errorf("error rate limiting requests: %w url=%s", err, req.URL)
				}

				resp, err = client.Do(req) // #nosec G704
				if err != nil {
					logger.Error("json fetch url failed: request: %v path=%q", err, currentNode.path)
					return nil
				}

				// Update the rate limit based on the server's response
				s.RateLimit.Update(resp)
			}

			if resp.StatusCode != http.StatusOK {
				logger.Error(
					"json fetch url failed with an unexpected status code: status_code=%d path=%q",
					resp.StatusCode,
					currentNode.path,
				)
				file := &blsources.File{
					Content:         strings.NewReader(obj),
					Logger:          s.Logger,
					MaxArchiveDepth: s.MaxArchiveDepth,
					Path:            currentNode.path,
					ShouldSkip:      s.ShouldSkip,
				}

				return file.Fragments(ctx, yield)
			}
			defer (func() {
				if err := resp.Body.Close(); err != nil {
					logger.Debug("error closing json source response body: %v", err)
				}
			})()

			// Handle when the URL returns more json
			if strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
				data, err := io.ReadAll(resp.Body)
				if err != nil {
					logger.Error("could not read fetched json response body: %s path=%q", err, currentNode.path)
					return nil
				}

				jsonData := &JSON{
					ShouldSkip:      s.ShouldSkip,
					MaxArchiveDepth: s.MaxArchiveDepth,
					Path:            currentNode.path,
					RawMessage:      data,
				}

				return jsonData.Fragments(ctx, yield)
			}

			file := &blsources.File{
				Content: resp.Body,
				Logger:  s.Logger,
				Path:    currentNode.path,
			}

			return file.Fragments(ctx, yield)
		}

		file := &blsources.File{
			Content: strings.NewReader(obj),
			Logger:  s.Logger,
			Path:    currentNode.path,
		}

		return file.Fragments(ctx, yield)
	default:
		return nil
	}
}

func (s *JSON) JoinPath(root, child string) string {
	if len(s.Path) > 0 && s.Path == root {
		return root + blsources.InnerPathSeparator + child
	}

	return filepath.Join(root, child)
}

func (s *JSON) shouldFetchURL(path string) bool {
	if len(s.FetchURLPatterns) == 0 {
		return false
	}

	for _, pattern := range s.FetchURLPatterns {
		if fs.Match(pattern, path) {
			return true
		}
	}

	return false
}
