package patterns

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/leaktk/leaktk/pkg/logger"

	"github.com/open-policy-agent/opa/v1/rego"
)

type LeakTKPatterns struct {
	RegoQuery rego.PreparedEvalQuery
}

// LeakTKConfigHash returns the sha256 hash for the current leaktk config.
func (c *Patterns) LeakTKConfigHash() string {
	return fmt.Sprintf("%x", c.leaktkPatternsHash)
}

// LeakTK returns the LeakTKPatterns object, handling fetch/caching/update.
func (c *Patterns) LeakTK(ctx context.Context) (*LeakTKPatterns, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	cfg := c.config
	localPath := cfg.LeakTK.LocalPath
	modTimeExceeds := c.fileModTimeExceeds(localPath, cfg.RefreshAfter)

	if cfg.Autofetch && modTimeExceeds {
		logger.Info("fetching combined LeakTK models and OPA policy config")

		fetchURL, err := c.fetchURLFor("leaktk", cfg.LeakTK.Version)
		if err != nil {
			return c.leaktkPatterns, err
		}

		rawPatterns, hashChanged, err := c.fetchAndUpdate(ctx, fetchURL, localPath, c.leaktkPatternsHash)
		if err != nil {
			return c.leaktkPatterns, err
		}

		// Only parse and update if hash changed
		if hashChanged {
			newConfig, err := c.parseLeakTKConfig(ctx, rawPatterns)
			if err != nil {
				logger.Debug("fetched config:\n%s", rawPatterns)
				return c.leaktkPatterns, fmt.Errorf("could not parse combined config: error=%q", err)
			}
			c.leaktkPatterns = newConfig
			c.leaktkPatternsHash = sha256.Sum256([]byte(rawPatterns))
			logger.Info("updated combined models and OPA policy config: hash=%s", c.LeakTKConfigHash())
		}
	} else if c.leaktkPatterns == nil {
		if c.fileModTimeExceeds(localPath, cfg.ExpiredAfter) {
			return nil, fmt.Errorf(
				"combined config is expired and autofetch is disabled: config_path=%q",
				localPath,
			)
		}

		rawPatterns, err := c.loadFromDisk(localPath)
		if err != nil {
			return nil, err
		}

		newConfig, err := c.parseLeakTKConfig(ctx, rawPatterns)
		if err != nil {
			logger.Debug("loaded config:\n%s\n", rawPatterns)
			return nil, fmt.Errorf("could not parse combined config: error=%q", err)
		}
		c.leaktkPatterns = newConfig
		c.leaktkPatternsHash = sha256.Sum256([]byte(rawPatterns))
	}

	return c.leaktkPatterns, nil
}

// parseLeakTKConfig parses the LeakTK patterns config and compiles the Rego policy.
func (c *Patterns) parseLeakTKConfig(ctx context.Context, rawPatterns string) (*LeakTKPatterns, error) {
	var uncompiledLeakTKPatterns struct {
		Rego string `json:"opa_policy"`
	}

	if err := json.Unmarshal([]byte(rawPatterns), &uncompiledLeakTKPatterns); err != nil {
		return nil, fmt.Errorf("failed to unmarshal leaktk patterns: %w", err)
	}

	query, err := rego.New(
		rego.Query("data.leaktk.analyst.analyzed_response"),
		rego.Module("leaktk.analyst.rego", uncompiledLeakTKPatterns.Rego),
	).PrepareForEval(ctx)

	if err != nil {
		return nil, fmt.Errorf("could not compile rego query: %w", err)
	}

	leaktkPatterns := &LeakTKPatterns{
		RegoQuery: query,
	}

	return leaktkPatterns, nil
}
