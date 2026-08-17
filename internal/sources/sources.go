package sources

import (
	"errors"
	"fmt"

	"github.com/leaktk/leaktk/pkg/logger"
)

type Sources []Source

type Source interface {
	ID() string
	Kind() SourceKind
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
		case AtlassianCloudAdmin:
			*ss = append(*ss, &AtlassianCloudAdmin{
				id:      srcID,
				OrgID:   castSrcField[string](value, "org_id"),
				BaseURL: castOptSrcField[string](value, "base_url", "https://api.atlassian.com/admin"),
				BearerAuth: BearerAuth{
					Token: castSrcField[string](value, "token"),
				},
			})
		case AtlassianCloudJira:
			*ss = append(*ss, &AtlassianCloudJira{
				id:      srcID,
				BaseURL: castSrcField[string](value, "base_url"),
				BasicAuth: BasicAuth{
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
