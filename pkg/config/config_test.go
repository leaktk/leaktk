package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPartialLoadConfigFromFile(t *testing.T) {
	require.NoError(t, os.Setenv("LEAKTK_PATTERN_SERVER_AUTH_TOKEN", "x"))
	require.NoError(t, os.Unsetenv("LEAKTK_PATTERN_SERVER_URL"))
	cfg, err := LoadConfigFromFile("../../testdata/partial-config.toml")

	if err != nil {
		// If there are config issues fail fast
		assert.FailNowf(t, "Failed to load config file", "Load returned an error %s", err)
	}

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
			expected: 43200,
			actual:   cfg.Scanner.Patterns.RefreshAfter,
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
}

func TestLocateAndLoadConfig(t *testing.T) {
	// Set the env var here to prove the provided path overrides it
	localConfigDir = "../../testdata/locator-test/leaktk"

	t.Run("LoadFromFile", func(t *testing.T) {
		require.NoError(t, os.Setenv("LEAKTK_CONFIG_PATH", "../../testdata/locator-test/leaktk/config.2.toml"))
		cfg, err := LocateAndLoadConfig("../../testdata/locator-test/leaktk/config.1.toml")
		require.NoError(t, err)
		assert.Equal(t, "test-1", cfg.Scanner.Patterns.Gitleaks.Version)
	})

	t.Run("LoadFromEnvVar", func(t *testing.T) {
		require.NoError(t, os.Setenv("LEAKTK_CONFIG_PATH", "../../testdata/locator-test/leaktk/config.2.toml"))
		cfg, err := LocateAndLoadConfig("")
		require.NoError(t, err)
		assert.Equal(t, "test-2", cfg.Scanner.Patterns.Gitleaks.Version)
	})

	t.Run("FallBackOnDefault", func(t *testing.T) {
		require.NoError(t, os.Unsetenv("LEAKTK_CONFIG_PATH"))
		cfg, err := LocateAndLoadConfig("")
		require.NoError(t, err)
		assert.Equal(t, "test-3", cfg.Scanner.Patterns.Gitleaks.Version)
	})

}

func TestLoadSources(t *testing.T) {
	cfg, err := LoadConfigFromFile("../../testdata/sources-config.toml")
	require.NoError(t, err)

	ss := cfg.Sources
	require.Len(t, ss, 2)
	require.IsType(t, &AtlassianCloudJiraSource{}, ss[0])
	require.IsType(t, &AtlassianCloudAdminSource{}, ss[1])

	jira := ss[0].(*AtlassianCloudJiraSource)
	assert.Equal(t, "cloud-jira", jira.ID())
	assert.Equal(t, "jimbo", jira.Username)
	assert.Equal(t, "...", jira.Password)

	admin := ss[1].(*AtlassianCloudAdminSource)
	assert.Equal(t, "cloud-admin", admin.ID())
	assert.Equal(t, "...", admin.Token)
	assert.Equal(t, "1", admin.OrgID)
}
