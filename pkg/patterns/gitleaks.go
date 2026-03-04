package patterns

import (
	"context"
	"crypto/sha256"
	"fmt"

	betterleaksconfig "github.com/betterleaks/betterleaks/config"

	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/leaktk/leaktk/pkg/scanner/betterleaks"
)

// Gitleaks returns a Gitleaks config object, fetching/caching/updating as necessary.
func (c *Patterns) Gitleaks(ctx context.Context) (*betterleaksconfig.Config, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	cfg := c.config
	localPath := cfg.Gitleaks.LocalPath
	modTimeExceeds := c.fileModTimeExceeds(localPath, cfg.RefreshAfter)

	if cfg.Autofetch && modTimeExceeds {
		logger.Info("fetching gitleaks patterns")

		fetchURL, err := c.fetchURLFor("gitleaks", cfg.Gitleaks.Version)
		if err != nil {
			return c.gitleaksPatterns, err
		}

		rawPatterns, hashChanged, err := c.fetchAndUpdate(ctx, fetchURL, localPath, c.gitleaksPatternsHash)
		if err != nil {
			return c.gitleaksPatterns, err
		}

		// Only parse and update if hash changed
		if hashChanged {
			newConfig, err := betterleaks.ParseConfig(rawPatterns)
			if err != nil {
				logger.Debug("fetched config:\n%s", rawPatterns)
				return c.gitleaksPatterns, fmt.Errorf("could not parse gitleaks config: error=%q", err)
			}
			c.gitleaksPatterns = newConfig
			c.gitleaksPatternsHash = sha256.Sum256([]byte(rawPatterns))
			logger.Info("updated gitleaks patterns: hash=%s", c.GitleaksConfigHash())
		}
	} else if c.gitleaksPatterns == nil {
		if c.fileModTimeExceeds(localPath, cfg.ExpiredAfter) {
			return nil, fmt.Errorf(
				"gitleaks config is expired and autofetch is disabled: config_path=%q",
				localPath,
			)
		}

		rawPatterns, err := c.loadFromDisk(localPath)
		if err != nil {
			return c.gitleaksPatterns, err
		}

		newConfig, err := betterleaks.ParseConfig(rawPatterns)
		if err != nil {
			logger.Debug("loaded config:\n%s\n", rawPatterns)
			return c.gitleaksPatterns, fmt.Errorf("could not parse gitleaks config: error=%q", err)
		}
		c.gitleaksPatterns = newConfig
		c.gitleaksPatternsHash = sha256.Sum256([]byte(rawPatterns))
	}

	return c.gitleaksPatterns, nil
}

// GitleaksConfigHash returns the sha256 hash for the current gitleaks config.
func (c *Patterns) GitleaksConfigHash() string {
	return fmt.Sprintf("%x", c.gitleaksPatternsHash)
}
