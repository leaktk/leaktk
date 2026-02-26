package analyst

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/leaktk/leaktk/pkg/analyst/ai"
	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/leaktk/leaktk/pkg/patterns"
	"github.com/leaktk/leaktk/pkg/proto"
	"github.com/open-policy-agent/opa/v1/rego"
)

type Analyst struct {
	patterns *patterns.LeakTKPatterns
}

// NewAnalyst initializes and prepares the Rego engine. This should be called only once.
func NewAnalyst(incomingPatterns *patterns.Patterns) (*Analyst, error) {
	outgoingPatterns, err := incomingPatterns.LeakTK(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to extract LeakTK patterns: %w", err)
	}

	return &Analyst{
		patterns: outgoingPatterns,
	}, nil
}

func (a *Analyst) Analyze(ctx context.Context, response *proto.Response) (*proto.Response, error) {

	// Evaluate the Rego policy
	query, err := a.patterns.Rego.PrepareForEval(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not prepare query for evaluation %s: %w", response.ID, err)
	}
	results, err := query.Eval(ctx, rego.EvalInput(response))

	if err != nil {
		return nil, fmt.Errorf("could not evaluate query for response ID %s: %w", response.ID, err)
	}

	rego_results := true
	if len(results) == 0 || results[0].Expressions == nil || len(results[0].Expressions) == 0 {
		// If Rego produced no output, return the original response without analysis
		rego_results = false
		logger.Info("Analyze: OPA policy produced no analysis for response ID %s", response.ID)
	}

	var enrichedResponse proto.Response

	if rego_results == true {
		// Extract the OPA output (the enriched JSON structure)
		opaOutput := results[0].Expressions[0].Value
		// Marshal the enriched structure back to JSON bytes
		outputBytes, err := json.Marshal(opaOutput)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal OPA output to JSON: %w", err)
		}

		// Unmarshal the enriched JSON back into a new Response struct
		if err := json.Unmarshal(outputBytes, &enrichedResponse); err != nil {
			return nil, fmt.Errorf("failed to unmarshal enriched JSON back into Response struct: %w", err)
		}
	} else {
		enrichedResponse = *response
	}

	ai_analyst := ai.NewAnalyst(a.patterns.ModelsConfig)

	for _, result := range enrichedResponse.Results {
		if ai_analyst != nil {
			model := "LogisticRegression"
			analysis, err := ai_analyst.Analyze(model, a.patterns.ModelsConfig, result)
			if err != nil {
				logger.Fatal("Could not apply model to result: %v", err)
			} else {
				if result.Analysis == nil {
					result.Analysis = make(map[string]any)
				}
				result.Analysis["predicted_secret_probability"] = analysis.PredictedSecretProbability
			}
		} else {
			logger.Fatal("analyst is nil")
			result.Analysis["predicted_secret_probability"] = "fail"
		}
	}

	return &enrichedResponse, nil
}

// AnalyzeStream processes a JSONL stream of proto.Response structs from r,
// analyzes each one using the provided Analyst, and writes the enriched
// JSONL stream to w.
func AnalyzeStream(a *Analyst, r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// Unmarshal the raw JSON line into a Response struct
		var response proto.Response
		if err := json.Unmarshal(line, &response); err != nil {
			logger.Error("AnalyzeStream: could not unmarshal line to proto.Response: %v, line: %s", err, string(line))
			continue
		}

		// Run the struct through the decoupled analysis method
		enrichedResponse, err := a.Analyze(context.Background(), &response)
		if err != nil {
			logger.Error("AnalyzeStream: failed to analyze response ID %s: %v", response.ID, err)
			continue
		}

		// Write the final enriched Response JSON object followed by a newline (JSONL)
		outBytes, err := json.Marshal(enrichedResponse)
		if err != nil {
			logger.Error("AnalyzeStream: could not marshal enriched Response: %v", err)
			continue
		}

		w.Write(outBytes)
		w.Write([]byte{'\n'})
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading from input stream: %w", err)
	}

	return nil
}

// AnalyzeCommand is the entry point for the CLI subcommand.
// It sets up the Analyst and passes the input stream to AnalyzeStream.
// func AnalyzeCommand(ctx context.Context, inputPath string) error {
// 	policyContent, err := os.ReadFile("pkg/analyst/policy.rego")
// 	if err != nil {
// 		return fmt.Errorf("could not read Rego policy file %s: %w", "pkg/analyst/policy.rego", err)
// 	}

// 	analyst, err := NewAnalyst(ctx, string(policyContent))
// 	if err != nil {
// 		return fmt.Errorf("failed to initialize analyst: %w", err)
// 	}

// 	var r io.Reader
// 	var closer func() error

// 	if inputPath != "" {
// 		f, err := os.Open(inputPath)
// 		if err != nil {
// 			return fmt.Errorf("could not open input file %s: %w", inputPath, err)
// 		}
// 		r = f
// 		closer = f.Close
// 	} else {
// 		r = os.Stdin
// 		closer = func() error { return nil }
// 	}

// 	defer func() {
// 		if err := closer(); err != nil && closer != nil {
// 			logger.Error("AnalyzeCommand: failed to close input reader: %v", err)
// 		}
// 	}()

// 	return AnalyzeStream(analyst, r, os.Stdout)
// }
