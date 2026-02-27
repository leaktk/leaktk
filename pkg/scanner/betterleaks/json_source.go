package betterleaks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/betterleaks/betterleaks/config"
	"github.com/betterleaks/betterleaks/sources"

	"github.com/leaktk/leaktk/pkg/fs"
	httpclient "github.com/leaktk/leaktk/pkg/http"
	"github.com/leaktk/leaktk/pkg/logger"
)

var urlRegexp = regexp.MustCompile(`^https?:\/\/\S+$`)

// JSON is a source for yielding fragments from strings in json data
// and from URLs contained in the data that match FetchURLPatterns
type JSON struct {
	Config           *config.Config
	FetchURLPatterns []string
	MaxArchiveDepth  int
	Path             string
	RawMessage       json.RawMessage
	data             any
}

type jsonNode struct {
	path  string
	value any
}

// Fragments yields the fragments contained in this resource
func (s *JSON) Fragments(ctx context.Context, yield sources.FragmentsFunc) error {
	if s.data == nil {
		if err := json.Unmarshal([]byte(s.RawMessage), &s.data); err != nil {
			return fmt.Errorf("could not unmarshal json data: %w", err)
		}
	}

	return s.walkAndYield(ctx, jsonNode{path: s.Path, value: s.data}, yield)
}

func (s *JSON) walkAndYield(ctx context.Context, currentNode jsonNode, yield sources.FragmentsFunc) error {
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
			req, err := http.NewRequestWithContext(ctx, "GET", obj, nil)
			if err != nil {
				logger.Error("json fetch url failed: %v path=%q", err, currentNode.path)

				return nil
			}
			resp, err := client.Do(req)
			if err != nil {
				logger.Error("json fetch url failed: %v path=%q", err, currentNode.path)

				return nil
			}
			if resp.StatusCode != http.StatusOK {
				logger.Error(
					"json fetch url failed with an unexpected status code: status_code=%d path=%q",
					resp.StatusCode,
					currentNode.path,
				)
				file := &sources.File{
					Config:          s.Config,
					Content:         strings.NewReader(obj),
					MaxArchiveDepth: s.MaxArchiveDepth,
					Path:            currentNode.path,
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
					Config:          s.Config,
					MaxArchiveDepth: s.MaxArchiveDepth,
					Path:            currentNode.path,
					RawMessage:      data,
				}

				return jsonData.Fragments(ctx, yield)
			}

			file := &sources.File{
				Path:    currentNode.path,
				Content: resp.Body,
			}

			return file.Fragments(ctx, yield)
		}

		file := &sources.File{
			Path:    currentNode.path,
			Content: strings.NewReader(obj),
		}

		return file.Fragments(ctx, yield)
	default:
		return nil
	}
}

func (s *JSON) JoinPath(root, child string) string {
	if len(s.Path) > 0 && s.Path == root {
		return root + sources.InnerPathSeparator + child
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
