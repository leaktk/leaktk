package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leaktk/leaktk/internal/sources"
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
		require.NoError(t, os.Setenv("LEAKTK_PATTERN_SERVER_AUTH_TOKEN", "x"))
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
					expected: "https://example.com/leaktk/patterns/main/target",
					actual:   cfg.Scanner.Patterns.Server.URL,
				},
				{
					expected: "x",
					actual:   cfg.Scanner.Patterns.Server.AuthToken,
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
			require.Len(t, ss, 2)

			assert.Equal(t, "cloud-jira", ss[0].ID())
			assert.Equal(t, "cloud-admin", ss[1].ID())

			require.NoError(t, os.WriteFile(configPath, []byte(additionalSourcesConfig), 0o600))
			require.NoError(t, UpdateConfigFromFile(cfg, configPath))

			ss = cfg.Sources
			require.Len(t, ss, 3)
			assert.Equal(t, "cloud-jira", ss[0].ID())
			assert.Equal(t, "cloud-admin", ss[1].ID())
			assert.Equal(t, "cloud-jira-stage", ss[2].ID())
			assert.Equal(t, "johnny", ss[2].(*sources.AtlassianCloudJira).Username)

			// the updated config has the same ID as the other cloud-jira-stage
			// so this should update the existing one rather than just
			// appending
			require.NoError(t, os.WriteFile(configPath, []byte(updateAdditionalSourcesConfig), 0o600))
			require.NoError(t, UpdateConfigFromFile(cfg, configPath))

			ss = cfg.Sources
			require.Len(t, ss, 3)
			assert.Equal(t, "james", ss[2].(*sources.AtlassianCloudJira).Username)
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

		t.Run("FromParam", func(t *testing.T) {
			// param should take precedence over the env var
			require.NoError(t, os.Setenv("LEAKTK_CONFIG_PATH", writeConfig("config-env")))
			cfg, err := LocateAndLoadConfig(writeConfig("config-param"))
			require.NoError(t, err)
			assert.Equal(t, "config-param", cfg.Scanner.Patterns.Gitleaks.Version)
		})
		t.Run("FromEnvVar", func(t *testing.T) {
			// with the param empty it should pick the env var
			require.NoError(t, os.Setenv("LEAKTK_CONFIG_PATH", writeConfig("config-env")))
			cfg, err := LocateAndLoadConfig("")
			require.NoError(t, err)
			assert.Equal(t, "config-env", cfg.Scanner.Patterns.Gitleaks.Version)
		})
		t.Run("FromDefault", func(t *testing.T) {
			require.NoError(t, os.Unsetenv("LEAKTK_CONFIG_PATH"))
			assert.Equal(t, filepath.Join(userConfigDir, "config.toml"), writeConfig(""))
			cfg, err := LocateAndLoadConfig("")
			require.NoError(t, err)
			assert.Equal(t, "config-no-id", cfg.Scanner.Patterns.Gitleaks.Version)
		})
		t.Run("Sources", func(t *testing.T) {
			require.NoError(t, os.WriteFile(configPath, []byte(sourcesConfig), 0o600))
			cfg, err := LocateAndLoadConfig(configPath)
			require.NoError(t, err)

			// Making sure it loads sources properly given the custom loading logic
			ss := cfg.Sources
			require.Len(t, ss, 2)
			require.IsType(t, &sources.AtlassianCloudJira{}, ss[0])
			require.IsType(t, &sources.AtlassianCloudAdmin{}, ss[1])

			jira := ss[0].(*sources.AtlassianCloudJira)
			assert.Equal(t, "cloud-jira", jira.ID())
			assert.Equal(t, "jimbo", jira.Username)
			assert.Equal(t, "...", jira.Password)

			admin := ss[1].(*sources.AtlassianCloudAdmin)
			assert.Equal(t, "cloud-admin", admin.ID())
			assert.Equal(t, "...", admin.Token)
			assert.Equal(t, "1", admin.OrgID)
		})
	})
}

func TestAutologinDefault(t *testing.T) {
	require.NoError(t, os.Unsetenv("LEAKTK_AUTOLOGIN"))
	cfg := setMissingValues(DefaultConfig())
	assert.False(t, cfg.Scanner.Patterns.Autologin)
}

func TestAutologinEnvVar(t *testing.T) {
	t.Run("True", func(t *testing.T) {
		t.Setenv("LEAKTK_AUTOLOGIN", "true")
		cfg := setMissingValues(DefaultConfig())
		assert.True(t, cfg.Scanner.Patterns.Autologin)
	})

	t.Run("One", func(t *testing.T) {
		t.Setenv("LEAKTK_AUTOLOGIN", "1")
		cfg := setMissingValues(DefaultConfig())
		assert.True(t, cfg.Scanner.Patterns.Autologin)
	})

	t.Run("False", func(t *testing.T) {
		t.Setenv("LEAKTK_AUTOLOGIN", "false")
		cfg := setMissingValues(DefaultConfig())
		assert.False(t, cfg.Scanner.Patterns.Autologin)
	})
}
