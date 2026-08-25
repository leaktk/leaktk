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
	Patterns *patterns.Patterns
}

// NewAnalyst initializes the Analyst with patterns.
func NewAnalyst(p *patterns.Patterns) *Analyst {
	return &Analyst{
		Patterns: p,
	}
}

func (a *Analyst) Analyze(ctx context.Context, response *proto.Response, patterns *patterns.LeakTKPatterns) (*proto.Response, error) {
	// Evaluate the Rego policy
	results, err := patterns.RegoQuery.Eval(ctx, rego.EvalInput(response))
	if err != nil {
		return nil, fmt.Errorf("could not evaluate rego query: %w", err)
	}

	if len(results) == 0 || results[0].Expressions == nil || len(results[0].Expressions) == 0 {
		return nil, fmt.Errorf("could not analyze response: %w", err)
	}
	if err = mapstructure.Decode(results[0].Expressions[0].Value.(map[string]interface{})["results"], &response.Results); err != nil {
		return nil, err
	}

	return response, nil
}
