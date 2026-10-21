package sources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/leaktk/leaktk/internal/auths"
	"github.com/leaktk/leaktk/internal/httpclient"
	"github.com/leaktk/leaktk/pkg/logger"
)

type Sources []Source

// Merge merges new sources (o) with existing sources (ss) and then dedups and
// merges the combined list by ID. A nil other source can be provided to dedup
// the current list by merging duplicates together.
func (ss *Sources) Merge(o Sources) error {
	merged := make(Sources, 0, len(*ss))
	seen := make(map[string]Source, len(*ss))

	// merge sources with matching IDs
	for _, s := range append(*ss, o...) {
		if existing := seen[s.ID()]; existing != nil {
			if err := existing.Merge(s); err != nil {
				return fmt.Errorf("%v id=%s", err, s.ID())
			}
		} else {
			seen[s.ID()] = s
			merged = append(merged, s)
		}
	}

	*ss = merged
	return nil
}

type Source interface {
	ID() string
	Kind() Kind
	Merge(o Source) error
}

type httpHeaderSetter interface {
	SetHeader(ctx context.Context, h http.Header) error
	AppliesTo(url *url.URL) bool
}

// SetHeader runs source.SetHeader(context.Context, req.Header) for each
// applicable source in order that they apper in the config.
// NOTE: If more than one source sets the same header, the last one wins.
func (ss Sources) SetHeader(ctx context.Context, req *http.Request) error {
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

		if err := hs.SetHeader(ctx, req.Header); err != nil {
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
				id:        srcID,
				OrgID:     castSrcField[string](value, "org_id"),
				BaseURL:   strings.TrimRight(castOptSrcField[string](value, "base_url", "https://api.atlassian.com/admin"), "/"),
				RateLimit: httpclient.NewRateLimit(),
				BearerAuth: auths.BearerAuth{
					Token: castSrcField[string](value, "token"),
				},
			})
		case AtlassianCloudJiraKind:
			*ss = append(*ss, &AtlassianCloudJira{
				id:        srcID,
				BaseURL:   strings.TrimRight(castSrcField[string](value, "base_url"), "/"),
				RateLimit: httpclient.NewRateLimit(),
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
