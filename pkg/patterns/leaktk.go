package patterns

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/open-policy-agent/opa/ast"
	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/leaktk/leaktk/pkg/proto"

	"github.com/leaktk/leaktk/pkg/ai"
)

type LeakTKPatterns struct {
	ModelsConfig []ai.MLModelsConfig
	RegoQuery    rego.PreparedEvalQuery
}

// LeakTKConfigHash returns the sha256 hash for the current leaktk config.
func (p *Patterns) LeakTKConfigHash() string {
	return fmt.Sprintf("%x", p.leaktkPatternsHash)
}

// LeakTK returns the LeakTKPatterns object, handling fetch/caching/u date.
func (p *Patterns) LeakTK(ctx context.Context) (*LeakTKPatterns, error) {
	logger.Info("LeakTK Run")
	return getOrUpdate(
		ctx, p,
		&p.leaktkPatterns,
		&p.leaktkPatternsHash,
		"leaktk",
		p.config.LeakTK.LocalPath,
		p.config.LeakTK.Version,
		p.parseLeakTKConfig,
	)
}

// parseLeakTKConfig parses the LeakTK patterns config and compiles the Rego policy.
func (p *Patterns) parseLeakTKConfig(ctx context.Context, rawPatterns string) (any, error) {
	var uncompiledLeakTKPatterns struct {
		ModelsConfig []ai.MLModelsConfig `json:"models"`
		Rego         string              `json:"opa_policy"`
	}

	if err := json.Unmarshal([]byte(rawPatterns), &uncompiledLeakTKPatterns); err != nil {
		return nil, fmt.Errorf("failed to unmarshal leaktk patterns: %w", err)
	}

	runModelProvider := func(bctx rego.BuiltinContext, arg1 *ast.Term, arg2 *ast.Term) (*ast.Term, error) {
		var modelName string
		var findingRaw interface{}

		if err := ast.As(arg1.Value, &modelName); err != nil {
			return nil, fmt.Errorf("leaktk.ai.RunModel: invalid first argument: %w", err)
		}
		if err := ast.As(arg2.Value, &findingRaw); err != nil {
			return nil, fmt.Errorf("leaktk.ai.RunModel: invalid second argument: %w", err)
		}

		findingBytes, err := json.Marshal(findingRaw)
		if err != nil { 
			return nil, fmt.Errorf("leaktk.ai.RunModel: failed to marshal finding: %w", err)
		}

		var result proto.Result
		if err := json.Unmarshal(findingBytes, &result); err != nil {
			return nil, fmt.Errorf("leaktk.ai.RunModel: failed to parse finding into proto.Result: %w", err)
		}


		analyst := ai.NewAnalyst(uncompiledLeakTKPatterns.ModelsConfig)
		analysis, err := analyst.Analyze(modelName, &result)
		if err != nil {
			return nil, fmt.Errorf("leaktk.ai.RunModel: analysis failed: %w", err)
		}

		resVal, err := ast.InterfaceToValue(map[string]interface{}{
			"probability": analysis.PredictedSecretProbability,
		})
		if err != nil {
			return nil, fmt.Errorf("leaktk.ai.RunModel: failed to create a return value: %w", err)
		}
		return ast.NewTerm(resVal), nil
	}

	query, err := rego.New(
		rego.Query("data.leaktk.analyst.analyzed_response"),
		rego.Module("leaktk.analyst.rego", uncompiledLeakTKPatterns.Rego),
		rego.Function2(ai.RunModelBuiltIn, runModelProvider),
	).PrepareForEval(ctx)

	if err != nil {
		return nil, fmt.Errorf("could not compile rego query: %w", err)
	}

	leaktkPatterns := &LeakTKPatterns{
		ModelsConfig: uncompiledLeakTKPatterns.ModelsConfig,
		RegoQuery:    query,
	}
	return leaktkPatterns, nil
}
