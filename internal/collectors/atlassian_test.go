package collectors

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leaktk/leaktk/internal/facts"
	"github.com/leaktk/leaktk/internal/sources"
	"github.com/leaktk/leaktk/pkg/logger"
)

// Source: https://developer.atlassian.com/cloud/admin/organization/rest/api-group-directory/#api-v2-orgs-orgid-directories-get
const mockAtlassianDirResponsePage1 = `
{
  "data": [
    {
      "directoryId": "12345678-1234-1234-1234-123456789012",
      "name": "Primary Directory",
      "icon": "https://icon1.example.com/icon1.png"
    }
  ],
  "links": {
    "self": "ObSbZxpM1f1fzia2_GnuJw",
    "prev": "LIZFEbzCT2pCCkQhPIUgIQ",
    "next": "kloHX1ZQVasDAkx_P48NYQ"
  }
}
`
const mockAtlassianDirResponsePage2 = `
{
  "data": [
    {
      "directoryId": "12345678-1234-1234-1234-123456789013",
      "name": "Secondary Directory",
      "icon": "https://icon2.example.com/icon2.png"
    }
  ],
  "links": {
    "self": "kloHX1ZQVasDAkx_P48NYQ",
    "prev": "ObSbZxpM1f1fzia2_GnuJw"
  }
}
`
const mockAtlassianSearchResponsePage1 = `
{
  "data": [
    {
      "accountId": "12345678-1234-1234-1234-123456789012",
      "accountType": "atlassian",
      "status": "active",
      "accountStatus": "active",
      "membershipStatus": "active",
      "name": "John Doe",
      "email": "john@example.com",
      "emailVerified": true
    }
  ],
  "links": {
    "self": "ObSbZxpM1f1fzia2_GnuJw",
    "prev": "LIZFEbzCT2pCCkQhPIUgIQ",
    "next": "kloHX1ZQVasDAkx_P48NYQ"
  }
}
`
const mockAtlassianSearchResponsePage2 = `
{
  "data": [
    {
      "accountId": "12345678-1234-1234-1234-123456789013",
      "accountType": "atlassian",
      "status": "active",
      "accountStatus": "active",
      "membershipStatus": "active",
      "name": "Jane Doe",
      "nickname": "Janey",
      "email": "jane@example.com",
      "emailVerified": true
    }
  ],
  "links": {
    "self": "kloHX1ZQVasDAkx_P48NYQ",
    "prev": "ObSbZxpM1f1fzia2_GnuJw"
  }
}
`
const sourcesConfig = `
[[sources]]
kind = "AtlassianCloudAdmin"
id = "cloud-admin"
token = "placeholder-cloud-admin-token" # notsecret
org_id = "1"

[[sources]]
kind = "AtlassianCloudJira"
id = "cloud-jira"
base_url = "https://example.atlassian.net"
username = "jimbo"
password = "placeholder-cloud-jira-password" # notsecret
`

func TestAtlassian(t *testing.T) {
	defer func(level logger.LogLevel) { _ = logger.SetLoggerLevel(level.String()) }(logger.GetLoggerLevel())
	_ = logger.SetLoggerLevel(logger.DEBUG.String())
	var cfg struct {
		Sources sources.Sources `toml:"sources"`
	}

	_, err := toml.Decode(sourcesConfig, &cfg)
	require.NoError(t, err)
	cloudAdmin := cfg.Sources[0].(*sources.AtlassianCloudAdmin)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+cloudAdmin.Token {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message": "unauthorized"}`))
			return
		}

		switch r.Method {
		case "GET":
			if r.URL.Path == "/v2/orgs/1/directories" {
				switch r.URL.Query().Get("cursor") {
				case "":
					_, _ = w.Write([]byte(mockAtlassianDirResponsePage1))
				case "kloHX1ZQVasDAkx_P48NYQ":
					_, _ = w.Write([]byte(mockAtlassianDirResponsePage2))
				default:
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"message": "invalid cursor"}`))
				}
				return
			}
		case "POST":
			searchUrls := []string{
				"/v2/orgs/1/directories/12345678-1234-1234-1234-123456789012/users/search",
				"/v2/orgs/1/directories/12345678-1234-1234-1234-123456789013/users/search",
			}
			if slices.Contains(searchUrls, r.URL.Path) {
				var values struct {
					Cursor string `json:"cursor"`
					Limit  int    `json:"limit"`
				}

				assert.NoError(t, json.NewDecoder(r.Body).Decode(&values))
				assert.Equal(t, 100, values.Limit)

				switch values.Cursor {
				case "":
					_, _ = w.Write([]byte(mockAtlassianSearchResponsePage1))
				case "kloHX1ZQVasDAkx_P48NYQ":
					_, _ = w.Write([]byte(mockAtlassianSearchResponsePage2))
				default:
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"message": "invalid cursor"}`))
				}
				return

			}
		}

		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprintf(w, "%#v", r) // #nosec:G705
	}))
	defer ts.Close()

	// Update the sources with the base_url of the test server
	cloudAdmin.BaseURL = ts.URL

	t.Run("atlassianCloudAdminDirIDs", func(t *testing.T) {
		ids, err := atlassianCloudAdminDirIDs(t.Context(), cloudAdmin)
		require.NoError(t, err)
		assert.Equal(t, []string{"12345678-1234-1234-1234-123456789012", "12345678-1234-1234-1234-123456789013"}, ids)
	})

	t.Run("atlassianCloudAdminFacts", func(t *testing.T) {
		var collectedFacts []facts.Fact
		var actualKindNames []string

		err := NewCollector().Facts(t.Context(), cfg.Sources[0:1], func(f facts.Fact) error {
			// zero entity ID facts map the fact kinds so the strings aren't repeated over and over again
			if f.EntityID == 0 {
				actualKindNames = append(actualKindNames, f.Kind.String())
			} else {
				collectedFacts = append(collectedFacts, f)
			}
			return nil
		})

		require.NoError(t, err)
		require.Equal(t, facts.KindNames, actualKindNames)
		assert.Len(t, collectedFacts, 32)

		actual := make(map[int]map[string]string)
		for _, f := range collectedFacts {
			eid := f.EntityID
			emap, ok := actual[eid]
			if !ok {
				emap = make(map[string]string)
				actual[eid] = emap
			}
			emap[f.Kind.String()] = f.Value
		}

		john := map[string]string{
			facts.ActiveKind.String():               facts.FactValueTrue,
			facts.EmailAddressKind.String():         "john@example.com",
			facts.EmailAddressVerifiedKind.String(): facts.FactValueTrue,
			facts.EntityKindKind.String():           "AtlassianCloudUser",
			facts.IDKind.String():                   "12345678-1234-1234-1234-123456789012",
			facts.NameKind.String():                 "John Doe",
			facts.SourceIDKind.String():             "cloud-admin",
			facts.URLKind.String():                  "https://home.atlassian.com/o/1/people/12345678-1234-1234-1234-123456789012",
		}
		jane := map[string]string{
			facts.ActiveKind.String():               facts.FactValueTrue,
			facts.EmailAddressKind.String():         "jane@example.com",
			facts.EmailAddressVerifiedKind.String(): facts.FactValueTrue,
			facts.EntityKindKind.String():           "AtlassianCloudUser",
			facts.IDKind.String():                   "12345678-1234-1234-1234-123456789013",
			facts.NameKind.String():                 "Jane Doe",
			facts.SourceIDKind.String():             "cloud-admin",
			facts.URLKind.String():                  "https://home.atlassian.com/o/1/people/12345678-1234-1234-1234-123456789013",
		}

		expected := map[int]map[string]string{
			1: john, 2: jane,
			3: john, 4: jane,
		}

		assert.Equal(t, expected, actual)
	})
}
