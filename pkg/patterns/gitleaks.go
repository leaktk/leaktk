package patterns

import (
	"context"
	"fmt"

	betterleaksconfig "github.com/betterleaks/betterleaks/config"

	"github.com/leaktk/leaktk/pkg/scanner/betterleaks"
)

// Gitleaks returns a Gitleaks config object, fetching/caching/updating as necessary.
func (c *Patterns) Gitleaks(ctx context.Context) (*betterleaksconfig.Config, error) {
	return getOrUpdate(
		ctx, c,
		&c.gitleaksPatterns,
		&c.gitleaksPatternsHash,
		"gitleaks",
		c.config.Gitleaks.LocalPath,
		c.config.Gitleaks.Version,
		func(_ context.Context, raw string) (*betterleaksconfig.Config, error) {
			return betterleaks.ParseConfig(raw)
		},
	)
}

// GitleaksConfigHash returns the sha256 hash for the current gitleaks config.
func (c *Patterns) GitleaksConfigHash() string {
	return fmt.Sprintf("%x", c.gitleaksPatternsHash)
}
