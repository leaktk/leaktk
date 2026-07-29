package collector

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leaktk/leaktk/pkg/config"
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
const sourcesConfig = `
[[sources]]
kind = "AtlassianCloudAdmin"
id = "cloud-admin"
token = "placeholder-cloud-admin-token" # notsecret
org_id = "1"

[[sources]]
kind = "AtlassianCloudJira"
id = "cloud-jira"
site_url = "https://example.atlassian.net"
username = "jimbo"
password = "placeholder-cloud-jira-password" # notsecret
`

func TestAtlassian(t *testing.T) {
	defer func(level logger.LogLevel) { _ = logger.SetLoggerLevel(level.String()) }(logger.GetLoggerLevel())
	_ = logger.SetLoggerLevel(logger.DEBUG.String())

	var cfg struct {
		Sources config.Sources `toml:"sources"`
	}

	_, err := toml.Decode(sourcesConfig, &cfg)
	require.NoError(t, err)
	cloudAdmin := cfg.Sources[0].(*config.AtlassianCloudAdminSource)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+cloudAdmin.Token {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message": "unauthorized"}`))
			return
		}
		if r.Method == "GET" && r.URL.Path == fmt.Sprintf("/v2/orgs/%s/directories", cloudAdmin.OrgID) {
			query := r.URL.Query()
			switch query.Get("cursor") {
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
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprintf(w, "%#v", r) // #nosec:G705
	}))
	defer ts.Close()

	t.Run("atlassianCloudAdminDirIDs", func(t *testing.T) {
		ids, err := atlassianCloudAdminDirIDs(t.Context(), ts.URL, cloudAdmin)
		require.NoError(t, err)
		assert.Equal(t, []string{"12345678-1234-1234-1234-123456789012", "12345678-1234-1234-1234-123456789013"}, ids)
	})
}
