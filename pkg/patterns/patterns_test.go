package patterns

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leaktk/leaktk/internal/httpclient"
	"github.com/leaktk/leaktk/pkg/config"
)

// Live repository endpoint holding real well-known, bundles, signatures, and public keys
const livePatternsServerURL = "https://raw.githubusercontent.com/leaktk/patterns/refs/heads/well-known/target"

func setupPatterns(t *testing.T, patternsCfg *config.Patterns, client *http.Client) *Patterns {
	patternsCfg.Autofetch = true
	patternsCfg.RefreshAfter = 1

	tmpDir := t.TempDir()
	patternsCfg.CacheDir = tmpDir

	if len(patternsCfg.Gitleaks.ConfigPath) == 0 {
		patternsCfg.Gitleaks.ConfigPath = filepath.Join(tmpDir, "patterns", "gitleaks", patternsCfg.Gitleaks.Version)
	}

	if len(patternsCfg.Gitleaks.ConfigPath) > 0 {
		_ = os.Remove(patternsCfg.Gitleaks.ConfigPath)
	}

	return NewPatterns(patternsCfg, client)
}

func TestConfigurePatterns(t *testing.T) {
	ctx := context.Background()

	t.Run("SuccessLiveFetch", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Scanner.Patterns.Server.URL = livePatternsServerURL
		cfg.Scanner.Patterns.Gitleaks.Version = "x.y.z"

		client := httpclient.NewClient()
		p := setupPatterns(t, &cfg.Scanner.Patterns, client)

		err := p.ConfigurePatterns(ctx)
		require.NoError(t, err)

		assert.NotNil(t, p.GitleaksPatterns)
		assert.NotNil(t, p.LeaktkPatterns)
		assert.NotEmpty(t, p.GitleaksConfigHash())
	})

	t.Run("InvalidURL", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Scanner.Patterns.Server.URL = "https://invalid-host-that-does-not-exist.example.com"
		cfg.Scanner.Patterns.Gitleaks.Version = "x.y.z"

		client := httpclient.NewClient()
		p := setupPatterns(t, &cfg.Scanner.Patterns, client)

		err := p.ConfigurePatterns(ctx)
		require.Error(t, err)
	})
}
