package scanner

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leaktk/leaktk/pkg/config"
	httpclient "github.com/leaktk/leaktk/pkg/http"
)

const mockConfig = `
[allowlist]
paths = ['''testdata''']

[[rules]]
id = "test-rule"
description = "test-rule"
regex = '''test-rule'''
`

func setupPatterns(patternsCfg *config.Patterns, client *http.Client) *Patterns {
	// 1. Ensure fetch logic runs by forcing expiration/autofetch
	patternsCfg.Autofetch = true
	patternsCfg.RefreshAfter = 1

	// 2. Clean up any local files to ensure fetching occurs
	if len(patternsCfg.Gitleaks.LocalPath) > 0 {
		os.Remove(patternsCfg.Gitleaks.LocalPath)
	}

	// 3. Use the existing NewPatterns constructor
	return NewPatterns(patternsCfg, client)
}

func TestPatternsGitleaks(t *testing.T) {
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "GET", r.Method)
			assert.Equal(t, "/patterns/gitleaks/x.y.z", r.URL.Path)
			w.WriteHeader(http.StatusOK)
			_, err := io.WriteString(w, mockConfig)
			assert.NoError(t, err)
		}))
		ts.Start()
		defer ts.Close()

		cfg := config.DefaultConfig()
		cfg.Scanner.Patterns.Server.URL = ts.URL
		cfg.Scanner.Patterns.Gitleaks.Version = "x.y.z"

		client := httpclient.NewClient()
		// Use the helper to initialize the consolidated struct
		p := setupPatterns(&cfg.Scanner.Patterns, client)

		// Call the public method that now contains the fetch logic
		gitleaksCfg, err := p.Gitleaks(ctx)

		require.NoError(t, err)
		require.NotNil(t, gitleaksCfg)
	})

	t.Run("InvalidURL", func(t *testing.T) {
		cfg := config.DefaultConfig()
		// Invalid URL that will cause a network error during fetch
		cfg.Scanner.Patterns.Server.URL = "invalid-url"
		cfg.Scanner.Patterns.Gitleaks.Version = "x.y.z"

		client := httpclient.NewClient()
		p := setupPatterns(&cfg.Scanner.Patterns, client)

		// Call the public method
		_, err := p.Gitleaks(ctx)
		// The error will be a network-related error from the fetch logic
		require.Error(t, err)
	})

	t.Run("HTTPError", func(t *testing.T) {
		ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Server returns a non-200 status code
			w.WriteHeader(http.StatusInternalServerError)
		}))
		ts.Start()
		defer ts.Close()

		cfg := config.DefaultConfig()
		cfg.Scanner.Patterns.Server.URL = ts.URL
		cfg.Scanner.Patterns.Gitleaks.Version = "x.y.z"

		client := httpclient.NewClient()
		p := setupPatterns(&cfg.Scanner.Patterns, client)

		// Call the public method
		_, err := p.Gitleaks(ctx)
		require.Error(t, err)
		// The error will be the one returned by fetchConfig/Gitleaks for bad status
		assert.Contains(t, err.Error(), "unexpected status code")
	})

	t.Run("WithAuthToken", func(t *testing.T) {
		ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "GET", r.Method)
			assert.Equal(t, "/patterns/gitleaks/x.y.z", r.URL.Path)
			// Assert that the Authorization header was correctly set
			assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
			_, err := io.WriteString(w, mockConfig)
			assert.NoError(t, err)
		}))
		ts.Start()
		defer ts.Close()

		cfg := config.DefaultConfig()
		cfg.Scanner.Patterns.Server.URL = ts.URL
		cfg.Scanner.Patterns.Server.AuthToken = "test-token"
		cfg.Scanner.Patterns.Gitleaks.Version = "x.y.z"

		client := httpclient.NewClient()
		p := setupPatterns(&cfg.Scanner.Patterns, client)

		// Call the public method
		_, err := p.Gitleaks(ctx)
		require.NoError(t, err)
	})
}

func TestGitleaksConfigModTimeExceeds(t *testing.T) {
	t.Run("FileExistsAndOlderThanLimit", func(t *testing.T) {
		tempDir := t.TempDir()

		tempFilePath := filepath.Join(tempDir, "gitleaks.toml")
		err := os.WriteFile(tempFilePath, []byte{}, 0600)
		require.NoError(t, err)

		// Set the file's modification time to 10 seconds ago
		err = os.Chtimes(tempFilePath, time.Now().Add(-10*time.Second), time.Now().Add(-10*time.Second))
		require.NoError(t, err)

		// Create a Patterns instance with the temporary file path
		cfg := config.DefaultConfig()
		cfg.Scanner.Patterns.Gitleaks.LocalPath = tempFilePath

		patterns := &Patterns{
			patternsConfig: &cfg.Scanner.Patterns,
		}

		// Test with a modTimeLimit of 5 seconds
		assert.True(t, patterns.configModTimeExceeds(cfg.Scanner.Patterns.Gitleaks.LocalPath, 5))

		// Test with a modTimeLimit of 15 seconds
		assert.False(t, patterns.configModTimeExceeds(cfg.Scanner.Patterns.Gitleaks.LocalPath, 15))
	})

	t.Run("FileDoesNotExist", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.Scanner.Patterns.Gitleaks.LocalPath = "/path/to/nonexistent/file.toml"

		// Create a Patterns instance with a non-existent file path
		patterns := &Patterns{
			patternsConfig: &cfg.Scanner.Patterns,
		}

		// Test with any modTimeLimit
		assert.True(t, patterns.configModTimeExceeds(cfg.Scanner.Patterns.Gitleaks.LocalPath, 5))
		assert.True(t, patterns.configModTimeExceeds(cfg.Scanner.Patterns.Gitleaks.LocalPath, 15))
	})

	t.Run("FileExistsButErrorOnStat", func(t *testing.T) {
		// Create a Patterns instance with a file path that causes an error on Stat
		cfg := config.DefaultConfig()
		cfg.Scanner.Patterns.Gitleaks.LocalPath = "/dev/zero"

		patterns := &Patterns{
			patternsConfig: &cfg.Scanner.Patterns,
		}

		// Test with any modTimeLimit
		assert.True(t, patterns.configModTimeExceeds(cfg.Scanner.Patterns.Gitleaks.LocalPath, 5))
		assert.True(t, patterns.configModTimeExceeds(cfg.Scanner.Patterns.Gitleaks.LocalPath, 15))
	})
}
