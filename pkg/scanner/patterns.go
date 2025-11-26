package scanner

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	gitleaksconfig "github.com/zricethezav/gitleaks/v8/config"

	"github.com/leaktk/leaktk/pkg/analyst/ai"
	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/leaktk/leaktk/pkg/scanner/gitleaks"
)

// Patterns manages fetching, caching, and updating configuration for
// both gitleaks patterns and LeakTK ML models.
type Patterns struct {
	client *http.Client
	mutex  sync.Mutex

	// Gitleaks Patterns fields
	patternsConfig     *config.Patterns
	gitleaksConfigHash [32]byte
	gitleaksConfig     *gitleaksconfig.Config

	// LeakTK Models fields
	modelsConfig *ai.MLModelsConfig
}

// NewConfigFetcher returns a configured instance of Patterns.
func NewPatterns(patternsCfg *config.Patterns, client *http.Client) *Patterns {
	return &Patterns{
		client:         client,
		patternsConfig: patternsCfg,
	}
}

// --- Generic Helpers ---

// configModTimeExceeds returns true if the local configuration file at 'path'
// is older than 'modTimeLimit' seconds.
func (c *Patterns) configModTimeExceeds(path string, modTimeLimit int) bool {
	// When modTimeLimit is 0, expiration checking is effectively disabled.
	if modTimeLimit == 0 {
		return false
	}

	if fileInfo, err := os.Stat(path); err == nil {
		return int(time.Since(fileInfo.ModTime()).Seconds()) > modTimeLimit
	}

	// If os.Stat fails (e.g., file doesn't exist), assume we need to fetch.
	return true
}

// fetchConfig fetches the raw config file from the server.
func (c *Patterns) fetchConfig(ctx context.Context, configURL string, authToken string) (string, error) {
	logger.Debug("config url: url=%q", configURL)

	request, err := http.NewRequestWithContext(ctx, "GET", configURL, nil)
	if err != nil {
		return "", err
	}

	if len(authToken) > 0 {
		logger.Debug("setting authorization header")
		request.Header.Add(
			"Authorization",
			"Bearer "+authToken,
		)
	}

	response, err := c.client.Do(request)
	if err != nil {
		return "", err
	}

	defer (func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			logger.Debug("error closing pattern response body: %v", closeErr)
		}
	})()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: status_code=%d", response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func (p *Patterns) fetchGitleaksConfig(ctx context.Context) (string, error) {
	logger.Info("fetching gitleaks patterns")
	patternURL, err := url.JoinPath(
		p.config.Server.URL, "patterns", "gitleaks", p.config.Gitleaks.Version,
	)

	logger.Debug("patterns url: url=%q", patternURL)
	if err != nil {
		return "", err
	}

	request, err := http.NewRequestWithContext(ctx, "GET", patternURL, nil)
	if err != nil {
		return "", err
	}

	if len(p.config.Server.AuthToken) > 0 {
		logger.Debug("setting authorization header")
		request.Header.Add(
			"Authorization",
			"Bearer "+p.config.Server.AuthToken,
		)
	}

	response, err := p.client.Do(request)
	if err != nil {
		return "", err
	}

	defer (func() {
		if err := response.Body.Close(); err != nil {
			logger.Debug("error closing pattern response body: %v", err)
		}
	})()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: status_code=%d", response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}

	return string(body), err
}

// --- Gitleaks Config Methods ---

func (c *Patterns) gitleaksFetchURL() (string, error) {
	return url.JoinPath(
		c.patternsConfig.Server.URL, "patterns", "gitleaks", c.patternsConfig.Gitleaks.Version,
	)
}

// Gitleaks returns a Gitleaks config object, fetching/caching/updating as necessary.
func (c *Patterns) Gitleaks(ctx context.Context) (*gitleaksconfig.Config, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	cfg := c.patternsConfig
	localPath := cfg.Gitleaks.LocalPath
	modTimeExceeds := c.configModTimeExceeds(localPath, cfg.RefreshAfter)

	if cfg.Autofetch && modTimeExceeds {
		logger.Info("fetching gitleaks patterns")
		patternURL, err := c.gitleaksFetchURL()
		if err != nil {
			return c.gitleaksConfig, err
		}

		rawConfig, err := c.fetchConfig(ctx, patternURL, cfg.Server.AuthToken)
		if err != nil {
			return c.gitleaksConfig, err
		}

		newConfig, err := gitleaks.ParseConfig(rawConfig)
		if err != nil {
			logger.Debug("fetched config:\n%s", rawConfig)
			return c.gitleaksConfig, fmt.Errorf("could not parse gitleaks config: error=%q", err)
		}
		c.gitleaksConfig = newConfig

		if err := c.updateLocalConfig(localPath, rawConfig); err != nil {
			return c.gitleaksConfig, err
		}

		if hash := sha256.Sum256([]byte(rawConfig)); c.gitleaksConfigHash != hash {
			c.gitleaksConfigHash = hash
			logger.Info("updated gitleaks patterns: hash=%s", c.GitleaksConfigHash())
		}
	} else if c.gitleaksConfig == nil {
		if c.configModTimeExceeds(localPath, cfg.ExpiredAfter) {
			return nil, fmt.Errorf(
				"gitleaks config is expired and autofetch is disabled: config_path=%q",
				localPath,
			)
		}

		rawConfig, err := os.ReadFile(localPath)
		if err != nil {
			return c.gitleaksConfig, err
		}

		newConfig, err := gitleaks.ParseConfig(string(rawConfig))
		if err != nil {
			logger.Debug("loaded config:\n%s\n", rawConfig)
			return c.gitleaksConfig, fmt.Errorf("could not parse gitleaks config: error=%q", err)
		}
		c.gitleaksConfig = newConfig

		if hash := sha256.Sum256(rawConfig); c.gitleaksConfigHash != hash {
			c.gitleaksConfigHash = hash
		}
	}

	return c.gitleaksConfig, nil
}

// GitleaksConfigHash returns the sha256 hash for the current gitleaks config.
func (c *Patterns) GitleaksConfigHash() string {
	return fmt.Sprintf("%x", c.gitleaksConfigHash)
}

// --- LeakTK Models Methods ---

func (c *Patterns) leakTKFetchURL() (string, error) {
	return url.JoinPath(
		c.patternsConfig.Server.URL, "models", "leaktk", c.patternsConfig.LeakTK.Version,
	)
}

// LeakTK returns a LeakTK Models config object, fetching/caching/updating as necessary.
func (c *Patterns) LeakTK(ctx context.Context) (*ai.MLModelsConfig, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	cfg := c.patternsConfig
	localPath := cfg.LeakTK.LocalPath
	modTimeExceeds := c.configModTimeExceeds(localPath, cfg.RefreshAfter)

	if cfg.Autofetch && modTimeExceeds {
		logger.Info("fetching leaktk models")
		modelURL, err := c.leakTKFetchURL()
		if err != nil {
			return c.modelsConfig, err
		}

		rawConfig, err := c.fetchConfig(ctx, modelURL, cfg.Server.AuthToken)
		if err != nil {
			return c.modelsConfig, err
		}

		newConfig, err := c.parseMLModelsConfig(rawConfig)
		if err != nil {
			logger.Debug("fetched config:\n%s", rawConfig)
			return c.modelsConfig, fmt.Errorf("could not parse leaktk models config: error=%q", err)
		}
		c.modelsConfig = newConfig

		if err := c.updateLocalConfig(localPath, rawConfig); err != nil {
			return c.modelsConfig, err
		}

		// NOTE: The original code for LeakTK did not update a hash, but you could add one here if needed.
		// if hash := sha256.Sum256([]byte(rawConfig)); c.modelsConfigHash != hash {
		// 	c.modelsConfigHash = hash
		// 	logger.Info("updated leaktk models")
		// }
	} else if c.modelsConfig == nil {
		if c.configModTimeExceeds(localPath, cfg.ExpiredAfter) {
			return nil, fmt.Errorf(
				"leaktk config is expired and autofetch is disabled: config_path=%q",
				localPath,
			)
		}

		rawConfig, err := os.ReadFile(localPath)
		if err != nil {
			return c.modelsConfig, err
		}

		newConfig, err := c.parseMLModelsConfig(string(rawConfig))
		if err != nil {
			logger.Debug("loaded config:\n%s\n", rawConfig)
			return c.modelsConfig, fmt.Errorf("could not parse leaktk models config: error=%q", err)
		}
		c.modelsConfig = newConfig

		// NOTE: Same as above, add hash update if necessary.
	}

	return c.modelsConfig, nil
}

// updateLocalConfig writes the raw config content to the specified local file path.
// It creates the directory if it does not exist.
func (c *Patterns) updateLocalConfig(localPath, rawConfig string) error {
	if err := os.MkdirAll(filepath.Dir(localPath), 0700); err != nil {
		return fmt.Errorf("could not create config dir: error=%q", err)
	}

	// Only write the config after successful parsing
	if err := os.WriteFile(localPath, []byte(rawConfig), 0600); err != nil {
		return fmt.Errorf("could not write config: path=%q error=%q", localPath, err)
	}
	return nil
}

func (c *Patterns) parseMLModelsConfig(rawConfig string) (*ai.MLModelsConfig, error) {
	// Calls the external parser from the 'ai' package.
	config, err := ai.ParseConfig(rawConfig)
	if err != nil {
		// Adds context-specific error wrapping.
		return nil, fmt.Errorf("failed to parse LeakTK models config: %w", err)
	}
	return config, nil
}
