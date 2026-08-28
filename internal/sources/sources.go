package sources

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/leaktk/leaktk/internal/auths"
	"github.com/leaktk/leaktk/pkg/logger"
)

type Sources []Source

type Source interface {
	ID() string
	Kind() Kind
}

type httpHeaderSetter interface {
	SetHeader(h http.Header) error
	AppliesTo(url *url.URL) bool
}

// SetHeader runs source.SetHeader(req.Header) for each applicable source in order that they apper in the config.
// NOTE: If more than one source sets the same header, the last one wins.
func (ss Sources) SetHeader(req *http.Request) error {
	for _, s := range ss {
		hs, isHeaderSetter := s.(httpHeaderSetter)

		if !isHeaderSetter {
			logger.Debug("skipping source: cannot set headers: source_id=%q", s.ID())
			continue
		}

		if !hs.AppliesTo(req.URL) {
			logger.Debug("skipping source: not applicable: source_id=%q", s.ID())
			continue
		}

		if err := hs.SetHeader(req.Header); err != nil {
			return fmt.Errorf("source could not set header: %w source_id=%q", err, s.ID())
		}
	}

	return nil
}

func castOptSrcField[T any](values map[string]any, field string, fallback T) T {
	v := values[field]
	t, ok := v.(T)
	if !ok {
		if v == nil {
			return fallback
		}
		logger.Fatal("invalid source field type: field=%q expected_type=%T actual_type=%T", field, t, v)
	}
	return t
}

func castSrcField[T any](values map[string]any, field string) T {
	v := values[field]
	t, ok := v.(T)
	if !ok {
		if v == nil {
			logger.Fatal("missing required source field: field=%q", field)
		}
		logger.Fatal("invalid source field type: field=%q expected_type=%T actual_type=%T", field, t, v)
	}
	return t
}

func (ss *Sources) UnmarshalTOML(data any) error {
	values, ok := data.([]map[string]any)
	if !ok {
		return errors.New("sources must be a list of tables")
	}

	for i, value := range values {
		srcID := castSrcField[string](value, "id")
		kindName := castSrcField[string](value, "kind")
		kind, srcKindValid := KindFromName(kindName)
		if !srcKindValid {
			return fmt.Errorf("invalid source kind name: source_kind_name=%q source_index=%d", kindName, i)
		}

		switch kind {
		case AtlassianCloudAdminKind:
			*ss = append(*ss, &AtlassianCloudAdmin{
				id:      srcID,
				OrgID:   castSrcField[string](value, "org_id"),
				BaseURL: castOptSrcField[string](value, "base_url", "https://api.atlassian.com/admin"),
				BearerAuth: auths.BearerAuth{
					Token: castSrcField[string](value, "token"),
				},
			})
		case AtlassianCloudJiraKind:
			*ss = append(*ss, &AtlassianCloudJira{
				id:      srcID,
				BaseURL: castSrcField[string](value, "base_url"),
				BasicAuth: auths.BasicAuth{
					Username: castSrcField[string](value, "username"),
					Password: castSrcField[string](value, "password"),
				},
			})
		default:
			return fmt.Errorf("unknown source kind: %q index=%d", kind, i)
		}
	}

	return nil
}
