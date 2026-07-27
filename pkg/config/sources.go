package config

import (
	"errors"
	"fmt"

	"github.com/leaktk/leaktk/pkg/logger"
)

type Sources []Source

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
		var kind SourceKind
		kindStr, ok := value["kind"].(string)
		if !ok {
			return fmt.Errorf("source kind must be a string index=%d", i)
		}

		if err := kind.UnmarshalText([]byte(kindStr)); err != nil {
			return fmt.Errorf("%v index=%d", err, i)
		}

		switch kind {
		case AtlassianCloudAdminSourceKind:
			*ss = append(*ss, &AtlassianCloudAdminSource{
				id:    castSrcField[string](value, "id"),
				OrgID: castSrcField[string](value, "org_id"),
				BearerAuth: BearerAuth{
					Token: castSrcField[string](value, "token"),
				},
			})
		case AtlassianCloudJiraSourceKind:
			*ss = append(*ss, &AtlassianCloudJiraSource{
				id: castSrcField[string](value, "id"),
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
