package sources

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"

	"github.com/leaktk/leaktk/internal/facts"
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

func cleanLDAPURL(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		logger.Fatal("could not parse LDAP url: %v", err)
	}
	return fmt.Sprintf("%s://%s", u.Scheme, u.Host)
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
		case LDAP:
			// [[sources.extractors.socialURL]]
			//   entity_kind = "GitHubAccount"
			//   patterns = [
			//      '''https:\/\/github.com\/(?<GitHubAccount:Username>[^\/]+)''',
			//   ]
			extractors := make(facts.Extractors, 0)
			if em, exists := value["extractors"]; exists {
				es, ok := em.(map[string][]map[string]any)
				if !ok {
					return fmt.Errof("source extractors must be a table of tables: source_id=%q source_index=%d", srcID, i)
				}
				for name, e := range es {
					patterns := castSrcField[[]string](e, "patterns")
					patternRegexps := make([]*regexp.Regexp, len(patterns))
					for i, p := range patterns {
						patternRegexps[i] = regexp.MustCompile(p)
					}
					extractors[name] = facts.Extractor{
						Patterns: patternRegexps,
					}
				}
			}
			*ss = append(*ss, &LDAP{
				id:         srcID,
				URL:        cleanLDAPURL(castSrcField[string](value, "url")),
				Username:   castSrcField[string](value, "username"),
				Password:   castSrcField[string](value, "password"),
				BaseDN:     castSrcField[string](value, "base_dn"),
				Filter:     castOptSrcField(value, "filter", "(objectClass=*)"),
				Scope:      castOptSrcField(value, "scope", "sub"),
				Mapper:     castSrcField[facts.Mapper](value, "mapper"),
				Extractors: extractors,
			})
		default:
			return fmt.Errorf("unknown source kind: %q index=%d", kind, i)
		}
	}

	return nil
}
