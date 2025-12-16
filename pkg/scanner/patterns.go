package scanner

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	gitleaksconfig "github.com/zricethezav/gitleaks/v8/config"

	"github.com/leaktk/leaktk/pkg/analyst"
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
	patternsConfig     *config.Patterns // Holds the config settings (URLs, paths, versions)
	gitleaksConfigHash [32]byte
	gitleaksConfig     *gitleaksconfig.Config

	// LeakTK Combined Configuration Field
	// This now holds the configuration fetched from a single source.
	combinedConfig *CombinedModelsConfig
}

type CombinedModelsConfig struct {
	ModelsConfig *ai.MLModelsConfig `json:"models"`
	OPA          *analyst.OPAConfig `json:"opa_policy"`
}

// NewPatterns returns a configured instance of Patterns.
func NewPatterns(patternsCfg *config.Patterns, client *http.Client) *Patterns {
	return &Patterns{
		client:         client,
		patternsConfig: patternsCfg,
	}
}

// configModTimeExceeds returns true if the local configuration file at 'path'
// is older than 'modTimeLimit' seconds.
func (c *Patterns) configModTimeExceeds(path string, modTimeLimit int) bool {
	if modTimeLimit == 0 {
		return false
	}

	if fileInfo, err := os.Stat(path); err == nil {
		return int(time.Since(fileInfo.ModTime()).Seconds()) > modTimeLimit
	}

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

// updateLocalConfig writes the raw config content to the specified local file path.
// It creates the directory if it does not exist.
func (c *Patterns) updateLocalConfig(localPath, rawConfig string) error {
	if err := os.MkdirAll(filepath.Dir(localPath), 0700); err != nil {
		return fmt.Errorf("could not create config dir: error=%q", err)
	}

	if err := os.WriteFile(localPath, []byte(rawConfig), 0600); err != nil {
		return fmt.Errorf("could not write config: path=%q error=%q", localPath, err)
	}
	return nil
}

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

// LeakTKFetchURL now points to the single combined config file.
func (c *Patterns) LeakTKFetchURL() (string, error) {
	return url.JoinPath(
		c.patternsConfig.Server.URL, "patterns", "leaktk", c.patternsConfig.LeakTK.Version,
	)
}

// LeakTK returns the CombinedModelsConfig object, handling fetch/caching/update.
// This logic is now a direct copy of the Gitleaks logic, using the CombinedModelsConfig struct.
func (c *Patterns) LeakTK(ctx context.Context) (*CombinedModelsConfig, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	cfg := c.patternsConfig
	localPath := cfg.LeakTK.LocalPath
	modTimeExceeds := c.configModTimeExceeds(localPath, cfg.RefreshAfter)

	if cfg.Autofetch && modTimeExceeds {
		logger.Info("fetching combined LeakTK models and OPA policy config")

		configURL, err := c.LeakTKFetchURL()
		if err != nil {
			return c.combinedConfig, err
		}

		rawConfig, err := c.fetchConfig(ctx, configURL, cfg.Server.AuthToken)
		if err != nil {
			return c.combinedConfig, err
		}

		// Parse the single file into the combined struct
		newCombinedConfig, err := c.parseCombinedConfig(rawConfig)
		if err != nil {
			logger.Debug("fetched config:\n%s", rawConfig)
			return c.combinedConfig, fmt.Errorf("could not parse combined config: error=%q", err)
		}
		c.combinedConfig = newCombinedConfig

		// Write the single merged JSON file to the Models local path
		if err := c.updateLocalConfig(localPath, rawConfig); err != nil {
			return c.combinedConfig, err
		}

		// NOTE: Add hash update here if needed, similar to Gitleaks

		logger.Info("updated combined models and OPA policy config: path=%q", localPath)

	} else if c.combinedConfig == nil {
		if c.configModTimeExceeds(localPath, cfg.ExpiredAfter) {
			return nil, fmt.Errorf(
				"combined config is expired and autofetch is disabled: config_path=%q",
				localPath,
			)
		}

		rawConfig, err := os.ReadFile(localPath)
		if err != nil {
			return nil, err
		}

		// Parse the single file into the combined struct
		newCombinedConfig, err := c.parseCombinedConfig(string(rawConfig))
		if err != nil {
			logger.Debug("loaded config:\n%s\n", rawConfig)
			return nil, fmt.Errorf("could not parse combined config: error=%q", err)
		}
		c.combinedConfig = newCombinedConfig

		// NOTE: Add hash update here if needed, similar to Gitleaks
	}

	return c.combinedConfig, nil
}

// parseCombinedConfig is the only needed custom parser, handling the single combined file.
func (c *Patterns) parseCombinedConfig(rawConfig string) (*CombinedModelsConfig, error) {
	var combinedConfig CombinedModelsConfig
	if err := json.Unmarshal([]byte(rawConfig), &combinedConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal combined configuration: %w", err)
	}
	return &combinedConfig, nil
}
