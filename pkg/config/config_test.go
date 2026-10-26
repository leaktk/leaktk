package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leaktk/leaktk/internal/sources"
	"github.com/leaktk/leaktk/internal/urlutil"
)

const flatFieldUpdates = `
[scanner]
workdir = "/tmp/leaktk/scanner"

[scanner.patterns.server]
url = "https://example.com/leaktk/patterns/main/target"
`

const loadFromPathConfigTestTmpl = `
[scanner.patterns.gitleaks]
version = "%s"
`

const sourcesConfig = `
[[sources]]
kind = "AtlassianCloudJira"
id = "cloud-jira"
base_url = "https://example.atlassian.net"
username = "jimbo"
password = "..." # notsecret

[[sources]]
kind = "AtlassianCloudAdmin"
id = "cloud-admin"
token = "..." # notsecret
org_id = "1"
`

const additionalSourcesConfig = `
[[sources]]
kind = "AtlassianCloudJira"
id = "cloud-jira-stage"
base_url = "https://example.stage.atlassian.net"
username = "johnny"
password = "..." # notsecret
`
const updateAdditionalSourcesConfig = `
[[sources]]
kind = "AtlassianCloudJira"
id = "cloud-jira-stage"
base_url = "https://example.stage.atlassian.net"
username = "james"
password = "..." # notsecret
`

func TestConfig(t *testing.T) {
	/* Make sure that the layers of config merge as expected given the layers of config:
	 *   - system
	 *   - user
	 *   - env vars
	 *   - cli args
	 * Expected behavior:
	 *   - flat fields should be overwritten
	 *   - by default lists should be appended to
	 *   - sources with the same ID should be merged
	 */
	t.Run("Merge", func(t *testing.T) {
		// TODO: require.NoError(t, os.Setenv("LEAKTK_PATTERN_SERVER_AUTH_TOKEN", "x"))
		require.NoError(t, os.Unsetenv("LEAKTK_PATTERN_SERVER_URL"))

		t.Run("FlatFields", func(t *testing.T) {
			tempDir := t.TempDir()
			configPath := filepath.Join(tempDir, "config.toml")
			require.NoError(t, os.WriteFile(configPath, []byte(flatFieldUpdates), 0o600))
			cfg := DefaultConfig()
			require.NoError(t, UpdateConfigFromFile(cfg, configPath))

			// Check values
			tests := []struct {
				expected any
				actual   any
			}{
				{
					expected: "8.27.0",
					actual:   cfg.Scanner.Patterns.Gitleaks.Version,
				},
				{
					expected: "/tmp/leaktk/scanner",
					actual:   cfg.Scanner.Workdir,
				},
				{
					expected: urlutil.MustParseURL("https://example.com/leaktk/patterns/main/target"),
					actual:   &cfg.Scanner.Patterns.Server.URL,
				},
				{
					expected: "INFO",
					actual:   cfg.Logger.Level,
				},
				{
					expected: 0,
					actual:   cfg.Scanner.MaxScanDepth,
				},
			}

			for _, test := range tests {
				assert.Equal(t, test.expected, test.actual)
			}
		})
		t.Run("Lists", func(t *testing.T) {
			tempDir := t.TempDir()
			configPath := filepath.Join(tempDir, "config.toml")
			require.NoError(t, os.WriteFile(configPath, []byte(sourcesConfig), 0o600))
			cfg := DefaultConfig()
			require.NoError(t, UpdateConfigFromFile(cfg, configPath))

			ss := cfg.Sources
			// 2 above + the LeakTKServerEnvSourceID one
			require.Equal(t, 3, ss.Len())

			assert.NotNil(t, ss.Get("cloud-jira"))
			assert.NotNil(t, ss.Get("cloud-admin"))
			assert.NotNil(t, ss.Get(LeakTKServerEnvSourceID))

			require.NoError(t, os.WriteFile(configPath, []byte(additionalSourcesConfig), 0o600))
			require.NoError(t, UpdateConfigFromFile(cfg, configPath))

			ss = cfg.Sources
			// 3 above + the LeakTKServerEnvSourceID one
			require.Equal(t, 4, ss.Len())
			assert.NotNil(t, ss.Get("cloud-jira"))
			assert.NotNil(t, ss.Get("cloud-admin"))
			assert.NotNil(t, ss.Get("cloud-jira-stage"))
			assert.NotNil(t, ss.Get(LeakTKServerEnvSourceID))
			assert.Equal(t, "johnny", ss.Get("cloud-jira-stage").(*sources.AtlassianCloudJira).Username)

			// the updated config has the same ID as the other cloud-jira-stage
			// so this should update the existing one rather than just
			// appending
			require.NoError(t, os.WriteFile(configPath, []byte(updateAdditionalSourcesConfig), 0o600))
			require.NoError(t, UpdateConfigFromFile(cfg, configPath))

			ss = cfg.Sources
			require.Equal(t, 4, ss.Len())
			assert.Equal(t, "james", ss.Get("cloud-jira-stage").(*sources.AtlassianCloudJira).Username)
		})
	})
	t.Run("Load", func(t *testing.T) {
		configDir := t.TempDir()
		leaktkConfigDir := filepath.Join(configDir, "leaktk")
		require.NoError(t, os.MkdirAll(leaktkConfigDir, 0o700))
		// set the config.userConfigDir dir back after testing
		defer func(c string) { userConfigDir = c }(userConfigDir)
		userConfigDir = leaktkConfigDir

		configPath := filepath.Join(userConfigDir, "config.toml")

		writeConfig := func(id string) string {
			cfgPath := ""
			cfgData := ""

			if len(id) == 0 {
				cfgPath = configPath
				cfgData = fmt.Sprintf(loadFromPathConfigTestTmpl, "config-no-id")
			} else {
				cfgPath = filepath.Join(leaktkConfigDir, id+".config.toml")
				cfgData = fmt.Sprintf(loadFromPathConfigTestTmpl, id)
			}

			require.NoError(t, os.WriteFile(cfgPath, []byte(cfgData), 0o600))
			return cfgPath
		}

		t.Run("SetUserConfig", func(t *testing.T) {
			require.NoError(t, SetUserConfigPath(writeConfig("config-param")))
			cfg, err := Load()
			require.NoError(t, err)
			assert.Equal(t, "config-param", cfg.Scanner.Patterns.Gitleaks.Version)
		})
		t.Run("Sources", func(t *testing.T) {
			require.NoError(t, os.WriteFile(configPath, []byte(sourcesConfig), 0o600))
			require.NoError(t, SetUserConfigPath(configPath))
			cfg, err := Load()
			require.NoError(t, err)

			// Making sure it loads sources properly given the custom loading logic
			ss := cfg.Sources
			require.Equal(t, 3, ss.Len())
			require.IsType(t, &sources.AtlassianCloudJira{}, ss.Get("cloud-jira"))
			require.IsType(t, &sources.AtlassianCloudAdmin{}, ss.Get("cloud-admin"))
			require.IsType(t, &sources.LeakTKServer{}, ss.Get(LeakTKServerEnvSourceID))

			jira := ss.Get("cloud-jira").(*sources.AtlassianCloudJira)
			assert.Equal(t, "jimbo", jira.Username)
			assert.Equal(t, "...", jira.Password)

			admin := ss.Get("cloud-admin").(*sources.AtlassianCloudAdmin)
			assert.Equal(t, "...", admin.Token)
			assert.Equal(t, "1", admin.OrgID)
		})
	})
	t.Run("EnvVars", func(t *testing.T) {
		cfg := DefaultConfig()
		tests := []struct {
			name      string
			field     *bool
			values    []string
			expecteds []bool
		}{
			{
				name:      "LEAKTK_AUTOLOGIN",
				field:     &cfg.Scanner.Patterns.Autologin,
				values:    []string{"", "true", "0", "1"},
				expecteds: []bool{false, true, false, true},
			},
			{
				name:      "LEAKTK_AUTOFETCH",
				field:     &cfg.Scanner.Patterns.Autofetch,
				values:    []string{"", "true", "0", "1"},
				expecteds: []bool{true, true, false, true},
			},
			{
				name:      "LEAKTK_SCANNER_AUTOFETCH",
				field:     &cfg.Scanner.Patterns.Autofetch,
				values:    []string{"", "true", "0", "1"},
				expecteds: []bool{true, true, false, true},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				for i, value := range tt.values {
					t.Setenv(tt.name, value)
					expected := tt.expecteds[i]
					setMissingValues(cfg)
					assert.Equal(t, expected, *tt.field)
				}
			})
		}
	})
}
