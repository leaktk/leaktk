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

	// Individual LeakTK fields (Cached parsed results)
	modelsConfig *ai.MLModelsConfig
	opaConfig    *analyst.OPAConfig

	// Combined Configuration Field (The result of the LeakTK method)
	combinedConfig *CombinedModelsConfig
}

// CombinedModelsConfig holds the parsed AI models configuration and the fetched OPA policy data.
type CombinedModelsConfig struct {
	ModelsConfig *ai.MLModelsConfig // The existing AI models config
	OPA          *analyst.OPAConfig // The new OPA policy data
}

// NewPatterns returns a configured instance of Patterns.
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

// updateLocalConfig writes the raw config content to the specified local file path.
// It creates the directory if it does not exist.
func (c *Patterns) updateLocalConfig(localPath, rawConfig string) error {
	if err := os.MkdirAll(filepath.Dir(localPath), 0700); err != nil {
		return fmt.Errorf("could not create config dir: error=%q", err)
	}

	// Only write the config after successful parsing/marshalling
	if err := os.WriteFile(localPath, []byte(rawConfig), 0600); err != nil {
		return fmt.Errorf("could not write config: path=%q error=%q", localPath, err)
	}
	return nil
}

// --- Gitleaks Config Methods ---

func (c *Patterns) gitleaksFetchURL() (string, error) {
	return url.JoinPath(
		c.patternsConfig.Server.URL, "patterns", "gitleaks", c.patternsConfig.Gitleaks.Version,
	)
}

// Gitleaks returns a Gitleaks config object, fetching/caching/updating as necessary.
func (c *Patterns) Gitleaks(ctx context.Context) (*gitleaksconfig.Config, error) {
	// ... (Gitleaks implementation remains unchanged) ...
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

// --- LeakTK Combined Config Methods ---

func (c *Patterns) leakTKFetchURL() (string, error) {
	return url.JoinPath(
		c.patternsConfig.Server.URL, "patterns", "leaktk", c.patternsConfig.LeakTK.Version, "models.json",
	)
}

func (c *Patterns) opaFetchURL() (string, error) {
	// NOTE: Assuming OPA Version field exists on the config.Patterns struct
	return url.JoinPath(
		c.patternsConfig.Server.URL, "patterns", "leaktk", c.patternsConfig.LeakTK.Version, "opa_policy.json",
	)
}

func (c *Patterns) LeakTK(ctx context.Context) (*CombinedModelsConfig, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Use the Models LocalPath as the single path for the combined config
	localPath := c.patternsConfig.LeakTK.LocalPath
	cfg := c.patternsConfig

	// Check if the single local file needs refreshing
	modTimeExceeds := c.configModTimeExceeds(localPath, cfg.RefreshAfter)

	// --- 2. Conditional Fetch and Merge ---
	if cfg.Autofetch && modTimeExceeds {
		logger.Info("fetching and combining LeakTK models and OPA policy")

		// --- A. Fetch LeakTK Models ---
		modelsConfig, err := c.fetchAndParseModels(ctx)
		if err != nil {
			// Return cached combined config on error
			return c.combinedConfig, err
		}
		c.modelsConfig = modelsConfig

		// --- B. Fetch OPA Policy ---
		opaConfig, err := c.fetchAndParseOPA(ctx)
		if err != nil {
			// Return cached combined config on error
			return c.combinedConfig, err
		}
		c.opaConfig = opaConfig

		// --- C. Merge and Save ---
		newCombinedConfig, err := c.mergeAndReturn()
		if err != nil {
			return c.combinedConfig, err
		}

		rawCombinedConfig, err := json.Marshal(newCombinedConfig)
		if err != nil {
			return c.combinedConfig, fmt.Errorf("failed to marshal combined config: %w", err)
		}

		if err := c.updateLocalConfig(localPath, string(rawCombinedConfig)); err != nil {
			return c.combinedConfig, err
		}

		c.combinedConfig = newCombinedConfig
		logger.Info("updated combined models and OPA policy config: path=%q", localPath)

	} else if c.combinedConfig == nil {
		// --- 3. Load from Local File (If Not Fetched) ---

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

		newCombinedConfig, err := c.parseCombinedConfig(string(rawConfig))
		if err != nil {
			logger.Debug("loaded config:\n%s\n", rawConfig)
			return nil, fmt.Errorf("could not parse combined config: error=%q", err)
		}

		c.modelsConfig = newCombinedConfig.ModelsConfig
		c.opaConfig = newCombinedConfig.OPA
		c.combinedConfig = newCombinedConfig
	}

	return c.combinedConfig, nil
}

// **MISSING HELPER 2/3:** Fetch and Parse Models
func (c *Patterns) fetchAndParseModels(ctx context.Context) (*ai.MLModelsConfig, error) {
	cfg := c.patternsConfig
	modelURL, err := c.leakTKFetchURL()
	if err != nil {
		return nil, err
	}
	rawConfig, err := c.fetchConfig(ctx, modelURL, cfg.Server.AuthToken)
	if err != nil {
		return nil, err
	}
	return c.parseMLModelsConfig(rawConfig)
}

// **MISSING HELPER 3/3:** Fetch and Parse OPA
func (c *Patterns) fetchAndParseOPA(ctx context.Context) (*analyst.OPAConfig, error) {
	cfg := c.patternsConfig
	opaURL, err := c.opaFetchURL()
	if err != nil {
		return nil, err
	}
	rawConfig, err := c.fetchConfig(ctx, opaURL, cfg.Server.AuthToken)
	if err != nil {
		return nil, err
	}
	return c.parseOPAConfig(rawConfig)
}

func (c *Patterns) parseMLModelsConfig(rawConfig string) (*ai.MLModelsConfig, error) {
	// FIX: Use the exported function name, likely ParseMLModelsConfig,
	// assuming it exists in the ai package.
	config, err := ai.ParseConfig(rawConfig)
	if err != nil {
		// Adds context-specific error wrapping.
		return nil, fmt.Errorf("failed to parse LeakTK models config: %w", err)
	}
	return config, nil
}

func (c *Patterns) parseOPAConfig(rawConfig string) (*analyst.OPAConfig, error) {
	var config analyst.OPAConfig
	// Assuming JSON for the OPA config
	if err := json.Unmarshal([]byte(rawConfig), &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal OPA policy configuration: %w", err)
	}
	// FIX: Return the parsed struct pointer
	return &config, nil
}

// New helper to parse the single file saved to disk
func (c *Patterns) parseCombinedConfig(rawConfig string) (*CombinedModelsConfig, error) {
	var combinedConfig CombinedModelsConfig
	// Assuming JSON for the combined config
	if err := json.Unmarshal([]byte(rawConfig), &combinedConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal combined configuration: %w", err)
	}
	return &combinedConfig, nil
}

func (c *Patterns) mergeAndReturn() (*CombinedModelsConfig, error) {
	if c.modelsConfig == nil {
		return nil, fmt.Errorf("failed to load required AI models configuration")
	}
	if c.opaConfig == nil {
		return nil, fmt.Errorf("failed to load required OPA policy configuration")
	}

	return &CombinedModelsConfig{
		ModelsConfig: c.modelsConfig,
		OPA:          c.opaConfig,
	}, nil
}
