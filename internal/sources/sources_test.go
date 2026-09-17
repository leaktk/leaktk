package sources

import (
	"net/http"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const configText = `
[[sources]]
id = 'org1-jira'
kind = 'AtlassianCloudJira'
username = 'org1-user'
password = 'org1-pass' # notsecret
base_url = 'https://org1.example.com'

[[sources]]
id = 'org1-atlassian-admin'
kind = 'AtlassianCloudAdmin'
base_url = 'https://api.example.com/admin'
org_id = 'org1-org1-org1-org1'
token = 'org1-token' # notsecret

[[sources]]
id = 'org2-jira'
kind = 'AtlassianCloudJira'
base_url = 'https://org2.example.com'
password = 'org2-pass' # notsecret
username = 'org2-user'

[[sources]]
id = 'org2-atlassian-admin'
kind = 'AtlassianCloudAdmin'
base_url = 'https://api.example.com/admin'
org_id = 'org2-org2-org2-org2'
token = 'org2-token' # notsecret
`

func TestSources(t *testing.T) {
	var cfg struct {
		Sources Sources
	}
	_, err := toml.Decode(configText, &cfg)
	require.NoError(t, err)
	require.Len(t, cfg.Sources, 4)

	t.Run("SetHeader", func(t *testing.T) {
		tests := []struct {
			name            string
			url             string
			expectedHeaders map[string]string
		}{
			{
				name: "Org1JiraAuth",
				url:  "https://org1.example.com/secure/content/1",
				expectedHeaders: map[string]string{
					"Authorization": "Basic b3JnMS11c2VyOm9yZzEtcGFzcw==", // notsecret
				},
			},
			{
				name: "Org1AtlassianAdminAuth",
				url:  "https://api.example.com/admin/v2/orgs/org1-org1-org1-org1/directories",
				expectedHeaders: map[string]string{
					"Authorization": "Bearer org1-token", // notsecret
				},
			},
			{
				name: "Org2JiraAuth",
				url:  "https://org2.example.com/secure/content/1",
				expectedHeaders: map[string]string{
					"Authorization": "Basic b3JnMi11c2VyOm9yZzItcGFzcw==", // notsecret
				},
			},
			{
				name: "Org2AtlassianAdminAuth",
				url:  "https://api.example.com/admin/v2/orgs/org2-org2-org2-org2/directories",
				expectedHeaders: map[string]string{
					"Authorization": "Bearer org2-token", // notsecret
				},
			},
			{
				name: "NonApplicableResource",
				url:  "https://www.example.com/cats.jpg",
				expectedHeaders: map[string]string{
					"Authorization": "", // notsecret
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				req, err := http.NewRequestWithContext(t.Context(), "GET", tt.url, nil)
				require.NoError(t, err)
				for header, value := range tt.expectedHeaders {
					assert.Empty(t, req.Header.Get(header), header)
					require.NoError(t, cfg.Sources.SetHeader(req))
					assert.Equal(t, value, req.Header.Get(header), header)
				}
			})
		}
	})
}
