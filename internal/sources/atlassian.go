package sources

import (
	"github.com/leaktk/leaktk/internal/auths"
	"github.com/leaktk/leaktk/pkg/http"
)

type AtlassianCloudAdmin struct {
	id        string
	OrgID     string
	BaseURL   string
	RateLimit http.RateLimit
	auths.BearerAuth
}

func (s *AtlassianCloudAdmin) ID() string {
	return s.id
}

func (s *AtlassianCloudAdmin) Kind() Kind {
	return AtlassianCloudAdminKind
}

type AtlassianCloudJira struct {
	id        string
	BaseURL   string
	RateLimit http.RateLimit
	auths.BasicAuth
}

func (s *AtlassianCloudJira) ID() string {
	return s.id
}

func (s *AtlassianCloudJira) Kind() Kind {
	return AtlassianCloudJiraKind
}
