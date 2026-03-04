package analyst

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/open-policy-agent/opa/v1/rego"

	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/leaktk/leaktk/pkg/patterns"
	"github.com/leaktk/leaktk/pkg/proto"
)

type Analyst struct {
	patterns *patterns.Patterns
}

// NewAnalyst initializes the Analyst with patterns.
func NewAnalyst(p *patterns.Patterns) (*Analyst, error) {
	return &Analyst{
		patterns: p,
	}, nil
}

func (a *Analyst) Analyze(ctx context.Context, response *proto.Response) (*proto.Response, error) {
	// Get the latest LeakTK patterns
	leaktkPatterns, err := a.patterns.LeakTK(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get leaktk patterns: %w", err)
	}

	// Prepare the Rego query
	query, err := leaktkPatterns.Rego.PrepareForEval(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare Rego query: %w", err)
	}

	// Evaluate the Rego policy
	results, err := query.Eval(ctx, rego.EvalInput(response))
	if err != nil {
		return nil, fmt.Errorf("could not evaluate query for response: %w id=%q", err, response.ID)
	}

	if len(results) == 0 || results[0].Expressions == nil || len(results[0].Expressions) == 0 {
		// If Rego produced no output, return the original response without analysis
		logger.Info("policy produced no analysis for response: id=%q", response.ID)
		return response, nil
	}

	// Extract the OPA output (the analyzed JSON structure)
	opaOutput := results[0].Expressions[0].Value

	// Marshal the analyzed structure back to JSON bytes
	outputBytes, err := json.Marshal(opaOutput)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal OPA output to JSON: %w", err)
	}

	// Unmarshal the analyzed JSON back into a new Response struct
	var analyzedResponse proto.Response
	if err := json.Unmarshal(outputBytes, &analyzedResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal analyzed response: %w", err)
	}

	return &analyzedResponse, nil
}
