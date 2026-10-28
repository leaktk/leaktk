package sources

import (
	"errors"
	"fmt"
	"strings"

	"github.com/leaktk/leaktk/internal/auths"
	"github.com/leaktk/leaktk/internal/httpclient"
	"github.com/leaktk/leaktk/internal/urlutil"
)

type AtlassianCloudAdmin struct {
	id               string      `toml:"id"`
	OrgID            string      `toml:"org_id"`
	BaseURL          urlutil.URL `toml:"base_url"`
	RateLimit        *httpclient.RateLimit
	auths.BearerAuth // imlements httpHeaderSetter
}

func (s *AtlassianCloudAdmin) ID() string {
	return s.id
}

func (s *AtlassianCloudAdmin) Load(d any) error {
	m, ok := d.(map[string]any)
	if !ok {
		return fmt.Errorf("could not load data: expected=\"map[string]any\" got=%T", d)
	}

	for k, v := range m {
		switch k {
		case "id":
			id, ok := v.(string)
			if !ok {
				return errors.New("id must be a string")
			}
			s.id = id
		case "org_id":
			orgID, ok := v.(string)
			if !ok {
				return errors.New("id must be a string")
			}
			s.OrgID = orgID
		case "base_url":
			rawURL, ok := v.(string)
			if !ok {
				return errors.New("base_url must be a string")
			}
			err := s.BaseURL.UnmarshalText([]byte(rawURL))
			if err != nil {
				return fmt.Errorf("invalid base_url: %w base_url=%q", err, rawURL)
			}
		}
	}

	return s.BearerAuth.Load(d)
}

func (s *AtlassianCloudAdmin) Kind() Kind {
	return AtlassianCloudAdminKind
}

func (s *AtlassianCloudAdmin) Merge(o Source) error {
	oACA, ok := o.(*AtlassianCloudAdmin)
	if !ok {
		return fmt.Errorf("cannot merge %s sources into %s", o.Kind(), s.Kind())
	}

	if len(oACA.BaseURL.Host) != 0 {
		s.BaseURL = oACA.BaseURL
	}

	if len(oACA.OrgID) != 0 {
		s.OrgID = oACA.OrgID
	}

	return s.BearerAuth.Merge(&oACA.BearerAuth)
}

func (s *AtlassianCloudAdmin) AppliesTo(u *urlutil.URL) bool {
	// Make sure this is talking to the correct service
	if !strings.EqualFold(s.BaseURL.Host, u.Host) || !strings.HasPrefix(u.Path, s.BaseURL.Path) {
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
	BaseURL         urlutil.URL
	RateLimit       *httpclient.RateLimit
	auths.BasicAuth // imlements httpHeaderSetter
}

func (s *AtlassianCloudJira) ID() string {
	return s.id
}

func (s *AtlassianCloudJira) Load(d any) error {
	m, ok := d.(map[string]any)
	if !ok {
		return fmt.Errorf("could not load data: expected=\"map[string]any\" got=%T", d)
	}

	for k, v := range m {
		switch k {
		case "id":
			id, ok := v.(string)
			if !ok {
				return errors.New("id must be a string")
			}
			s.id = id
		case "base_url":
			rawURL, ok := v.(string)
			if !ok {
				return errors.New("base_url must be a string")
			}
			err := s.BaseURL.UnmarshalText([]byte(rawURL))
			if err != nil {
				return fmt.Errorf("invalid base_url: %w base_url=%q", err, rawURL)
			}
		}
	}

	return s.BasicAuth.Load(d)
}

func (s *AtlassianCloudJira) Kind() Kind {
	return AtlassianCloudJiraKind
}

func (s *AtlassianCloudJira) Merge(o Source) error {
	oACJ, ok := o.(*AtlassianCloudJira)
	if !ok {
		return fmt.Errorf("cannot merge %s sources into %s", o.Kind(), s.Kind())
	}

	if len(oACJ.BaseURL.Host) != 0 {
		s.BaseURL = oACJ.BaseURL
	}

	return s.BasicAuth.Merge(&oACJ.BasicAuth)
}

func (s *AtlassianCloudJira) AppliesTo(u *urlutil.URL) bool {
	// Confirm it's the same host (case-insensitive)
	return strings.EqualFold(s.BaseURL.Host, u.Host)
}
