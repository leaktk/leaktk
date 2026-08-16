package betterleaks

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var mockAllowlistOnlyConfig = []byte(`
[allowlist]
paths = ['''testdata''']
`)

var mockConfig = []byte(`
[allowlist]
paths = ['''testdata''']

[[rules]]
id = "test-rule"
description = "test-rule"
regex = '''test-rule'''
`)

var mockPrefilterOnlyConfig = []byte(`
prefilter = 'matchesAny(get(attributes, "path", ""), ["testdata"])'
`)

var mockPrefilterWithRulesConfig = []byte(`
prefilter = 'matchesAny(get(attributes, "path", ""), ["testdata"])'
filter = 'get(finding, "secret", "") == "REDACTED"'

[[rules]]
id = "test-rule"
description = "test-rule"
regex = '''test-rule'''
`)

var mockPerRuleFilterConfig = []byte(`
[[rules]]
id = "test-rule"
description = "test-rule"
regex = '''test-rule'''
filter = 'matchesAny(get(attributes, "path", ""), ["vendor"])'
`)

func TestParseConfig(t *testing.T) {
	t.Run("ValidConfig", func(t *testing.T) {
		cfg, err := ParseConfig(mockConfig)
		require.NoError(t, err)
		assert.NotNil(t, cfg)
		assert.Contains(t, cfg.Rules, "test-rule")
		assert.Contains(t, cfg.Prefilter, "testdata")
	})

	t.Run("AllowlistOnlyConfig", func(t *testing.T) {
		cfg, err := ParseConfig(mockAllowlistOnlyConfig)
		require.NoError(t, err)
		assert.NotNil(t, cfg)
		assert.Empty(t, cfg.Rules)
		assert.Contains(t, cfg.Prefilter, "testdata")
	})

	t.Run("PrefilterOnlyConfig", func(t *testing.T) {
		cfg, err := ParseConfig(mockPrefilterOnlyConfig)
		require.NoError(t, err)
		assert.NotNil(t, cfg)
		assert.Empty(t, cfg.Rules)
		assert.Contains(t, cfg.Prefilter, "testdata")
	})

	t.Run("PrefilterWithRulesConfig", func(t *testing.T) {
		cfg, err := ParseConfig(mockPrefilterWithRulesConfig)
		require.NoError(t, err)
		assert.NotNil(t, cfg)
		assert.Contains(t, cfg.Rules, "test-rule")
		assert.Contains(t, cfg.Prefilter, "testdata")
		assert.Contains(t, cfg.Filter, "REDACTED")
	})

	t.Run("PerRuleFilterConfig", func(t *testing.T) {
		cfg, err := ParseConfig(mockPerRuleFilterConfig)
		require.NoError(t, err)
		assert.NotNil(t, cfg)
		require.Contains(t, cfg.Rules, "test-rule")
		assert.Contains(t, cfg.Rules["test-rule"].Filter, "vendor")
	})

	t.Run("InvalidConfig", func(t *testing.T) {
		_, err := ParseConfig([]byte("\ninvalid_key = \"value\"\n"))
		require.Error(t, err)
	})

	t.Run("EmptyConfig", func(t *testing.T) {
		_, err := ParseConfig([]byte(""))
		assert.Error(t, err)
	})
}
