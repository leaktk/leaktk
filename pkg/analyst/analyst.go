package analyst

import (
	"context"
	"fmt"

	"github.com/open-policy-agent/opa/v1/rego"

	"github.com/go-viper/mapstructure/v2"

	"github.com/leaktk/leaktk/pkg/patterns"
	"github.com/leaktk/leaktk/pkg/proto"
)

type Analyst struct {
	patterns *patterns.Patterns
}

// NewAnalyst initializes the Analyst with patterns.
func NewAnalyst(p *patterns.Patterns) *Analyst {
	return &Analyst{
		patterns: p,
	}
}

func (a *Analyst) Analyze(ctx context.Context, response *proto.Response) (*proto.Response, error) {
	// Get the latest LeakTK patterns
	leaktkPatterns, err := a.patterns.LeakTK(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not get leaktk patterns: %w", err)
	}

	// Evaluate the Rego policy
	results, err := leaktkPatterns.RegoQuery.Eval(ctx, rego.EvalInput(response))
	if err != nil {
		return nil, fmt.Errorf("could not evaluate rego query: %w", err)
	}

	// Convert the analyzed response to a proto.Response
	if len(results) == 0 || results[0].Expressions == nil || len(results[0].Expressions) == 0 {
		return nil, fmt.Errorf("could not analyze response: %w", err)
	}
	analyzedResponse := new(proto.Response)
	if err := mapstructure.Decode(results[0].Expressions[0].Value, &analyzedResponse); err != nil {
		return nil, fmt.Errorf("could not bind analyzed response: %w", err)
	}

	return analyzedResponse, nil
}
