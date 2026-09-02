package sources

import (
	"net/url"
	"strings"

	"github.com/leaktk/leaktk/internal/auths"
	"github.com/leaktk/leaktk/internal/httpclient"
	"github.com/leaktk/leaktk/pkg/logger"
)

type AtlassianCloudAdmin struct {
	id               string
	bURL             url.URL // stores a parsed version of the URL
	OrgID            string
	BaseURL          string
	RateLimit        *httpclient.RateLimit
	auths.BearerAuth // imlements httpHeaderSetter
}

func (s *AtlassianCloudAdmin) ID() string {
	return s.id
}

func (s *AtlassianCloudAdmin) Kind() Kind {
	return AtlassianCloudAdminKind
}

func (s *AtlassianCloudAdmin) AppliesTo(u *url.URL) bool {
	if len(s.bURL.Host) == 0 {
		bURL, err := url.Parse(s.BaseURL)
		if err != nil {
			logger.Debug("AtlassianCloudAdmin: could not parse base URL: source_id=%q base_url=%q", s.id, s.BaseURL)
			return false
		}
		s.bURL = *bURL
		s.bURL.Host = strings.ToLower(s.bURL.Host)
	}

	// Make sure this is talking to the correct service
	if !(strings.ToLower(u.Host) == s.bURL.Host && strings.HasPrefix(u.Path, s.bURL.Path)) {
		return false
	}

	// If it's a call referencing a specific org, make sure the org ID is in the path
	if strings.Contains(u.Path, "/orgs/") {
		return strings.Contains(u.Path, "/orgs/"+s.OrgID)
	}

	// Assume this is the right source for other non-org cloud admin API calls
	return true
}

type AtlassianCloudJira struct {
	id              string
	bURL            url.URL // stores a parsed version of the URL
	BaseURL         string
	RateLimit       *httpclient.RateLimit
	auths.BasicAuth // imlements httpHeaderSetter
}

func (s *AtlassianCloudJira) ID() string {
	return s.id
}

func (s *AtlassianCloudJira) Kind() Kind {
	return AtlassianCloudJiraKind
}

func (s *AtlassianCloudJira) AppliesTo(u *url.URL) bool {
	if len(s.bURL.Host) == 0 {
		bURL, err := url.Parse(s.BaseURL)
		if err != nil {
			logger.Debug("AtlassianCloudJira: could not parse base URL: source_id=%q base_url=%q", s.id, s.BaseURL)
			return false
		}
		s.bURL = *bURL
		s.bURL.Host = strings.ToLower(s.bURL.Host)
	}

	// Confirm it's the same host (case-insensitive)
	return strings.ToLower(u.Host) == s.bURL.Host
}
