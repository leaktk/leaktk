package patterns

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/leaktk/leaktk/pkg/analyst/ai"
	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/open-policy-agent/opa/rego"
)

type LeakTKPatterns struct {
	ModelsConfig []ai.MLModelsConfig `json:"models"`
	Rego         *rego.Rego          `json:"opa_policy"`
}

// LeakTK returns the LeakTKPatterns object, handling fetch/caching/update.
func (c *Patterns) LeakTK(ctx context.Context) (*LeakTKPatterns, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	cfg := c.patternsConfig
	localPath := cfg.LeakTK.LocalPath
	modTimeExceeds := c.configModTimeExceeds(localPath, cfg.RefreshAfter)

	if cfg.Autofetch && modTimeExceeds {
		logger.Info("fetching combined LeakTK models and OPA policy config")

		configURL, err := c.LeakTKFetchURL()
		if err != nil {
			return c.leaktkConfig, err
		}

		rawConfig, err := c.fetchConfig(ctx, configURL, cfg.Server.AuthToken)
		if err != nil {
			return c.leaktkConfig, err
		}

		// Parse the single file into the combined struct
		newleaktkConfig, err := c.parseleaktkConfig(rawConfig)
		if err != nil {
			logger.Debug("fetched config:\n%s", rawConfig)
			return c.leaktkConfig, fmt.Errorf("could not parse combined config: error=%q", err)
		}
		c.leaktkConfig = newleaktkConfig

		// Write the single merged JSON file to the Models local path
		if err := c.updateLocalConfig(localPath, rawConfig); err != nil {
			return c.leaktkConfig, err
		}

		// NOTE: Add hash update here if needed, similar to Gitleaks

		logger.Info("updated combined models and OPA policy config: path=%q", localPath)

	} else if c.leaktkConfig == nil {
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
		newleaktkConfig, err := c.parseleaktkConfig(string(rawConfig))
		if err != nil {
			logger.Debug("loaded config:\n%s\n", rawConfig)
			return nil, fmt.Errorf("could not parse combined config: error=%q", err)
		}
		c.leaktkConfig = newleaktkConfig

		// NOTE: Add hash update here if needed, similar to Gitleaks
	}

	return c.leaktkConfig, nil
}

// parseleaktkConfig is the only needed custom parser, handling the single combined file.
func (c *Patterns) parseleaktkConfig(rawConfig string) (*LeakTKPatterns, error) {
	var leaktkConfig LeakTKPatterns
	if err := leaktkConfig.UnmarshalJSON([]byte(rawConfig)); err != nil {
		return nil, fmt.Errorf("failed to unmarshal combined configuration: %w", err)
	}
	return &leaktkConfig, nil
}

func (c *LeakTKPatterns) UnmarshalJSON(data []byte) error {
	var leaktkConfig struct {
		ModelsConfig []ai.MLModelsConfig `json:"models"`
		Rego         string              `json:"opa_policy"`
	}

	if err := json.Unmarshal(data, &leaktkConfig); err != nil {
		return fmt.Errorf("could not unmarshal LeakTK Patterns: %w", err)
	}

	compiled := rego.New(
		rego.Query("data.analyze.analyzed_response"),
		rego.Module("analyze.rego", leaktkConfig.Rego),
	)

	c.ModelsConfig = leaktkConfig.ModelsConfig
	c.Rego = compiled

	return nil
}
