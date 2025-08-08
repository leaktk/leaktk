package gitleaks

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mockAllowlistOnlyConfig = `
[allowlist]
paths = ['''testdata''']
`

const mockConfig = `
[allowlist]
paths = ['''testdata''']

[[rules]]
id = "test-rule"
description = "test-rule"
regex = '''test-rule'''
`

func TestParseConfig(t *testing.T) {
	t.Run("ValidConfig", func(t *testing.T) {
		cfg, err := ParseConfig(mockConfig)
		require.NoError(t, err)
		assert.NotNil(t, cfg)
		assert.Equal(t, "testdata", cfg.Allowlists[0].Paths[0].String())
	})

	t.Run("AllowlistOnlyConfig", func(t *testing.T) {
		cfg, err := ParseConfig(mockAllowlistOnlyConfig)
		require.NoError(t, err)
		assert.NotNil(t, cfg)
		assert.Equal(t, "testdata", cfg.Allowlists[0].Paths[0].String())
	})

	t.Run("InvalidConfig", func(t *testing.T) {
		rawConfig := "\ninvalid_key = \"value\"\n"
		_, err := ParseConfig(rawConfig)
		require.Error(t, err)
	})

	t.Run("EmptyConfig", func(t *testing.T) {
		rawConfig := ""
		_, err := ParseConfig(rawConfig)
		assert.Error(t, err)
	})
}
