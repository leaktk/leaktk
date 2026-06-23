package patterns

import (
	"context"
	"fmt"

	betterleaksconfig "github.com/betterleaks/betterleaks/config"

	"github.com/leaktk/leaktk/pkg/scanner/betterleaks"
)

func parseBetterleaksConfig(_ context.Context, rawPatterns string) (any, error) {
	return betterleaks.ParseConfig(rawPatterns)
}

// Gitleaks returns a Gitleaks config object, fetching/caching/updating as necessary.
func (p *Patterns) Gitleaks(ctx context.Context) (*betterleaksconfig.Config, error) {
	return getOrUpdate(
		ctx, p,
		&p.gitleaksPatterns,
		&p.gitleaksPatternsHash,
		"gitleaks",
		p.config.Gitleaks.LocalPath,
		p.config.Gitleaks.Version,
		parseBetterleaksConfig,
	)
}

// GitleaksConfigHash returns the sha256 hash for the current gitleaks config.
func (p *Patterns) GitleaksConfigHash() string {
	return fmt.Sprintf("%x", p.gitleaksPatternsHash)
}
