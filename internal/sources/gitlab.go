package sources

import (
	"net/url"
	"strings"

	"github.com/leaktk/leaktk/internal/auths"
	"github.com/leaktk/leaktk/internal/httpclient"
	"github.com/leaktk/leaktk/pkg/logger"
)

type GitLab struct {
	id               string
	bURL             url.URL // stores a parsed version of the URL
	BaseURL          string
	RateLimit        *httpclient.RateLimit
	auths.BearerAuth // imlements httpHeaderSetter
}

func (s *GitLab) ID() string {
	return s.id
}

func (s *GitLab) Kind() Kind {
	return GitLabKind
}

func (s *GitLab) AppliesTo(u *url.URL) bool {
	if len(s.bURL.Host) == 0 {
		bURL, err := url.Parse(s.BaseURL)
		if err != nil {
			logger.Debug("GitLab: could not parse base URL: source_id=%q base_url=%q", s.id, s.BaseURL)
			return false
		}
		s.bURL = *bURL
		s.bURL.Host = strings.ToLower(s.bURL.Host)
	}

	// Make sure this is talking to the correct service
	if !(strings.ToLower(u.Host) == s.bURL.Host && strings.HasPrefix(u.Path, s.bURL.Path)) {
		return false
	}

	// Assume this is the right source if the host and path prefix match
	return true
}
