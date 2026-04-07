package patterns

import (
	"context"
	"encoding/json"
	"fmt"

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
	return getOrUpdate(
		ctx, c,
		&c.leaktkPatterns,
		&c.leaktkPatternsHash,
		"leaktk",
		c.config.LeakTK.LocalPath,
		c.config.LeakTK.Version,
		c.parseLeakTKConfig,
	)
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
