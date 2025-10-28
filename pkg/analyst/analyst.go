package analyst

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/open-policy-agent/opa/v1/rego"
)

type AnalyzedResult struct {
	ID        string `json:"id"`
	RequestID string `json:"request_id"`
	Analysis  any    `json:"analysis,omitempty"`
}

func (a AnalyzedResult) String() string {
	out, err := json.Marshal(a)
	if err != nil {
		return fmt.Sprintf("Error marshaling analysis: %v", err)
	}
	return string(out)
}

// AnalyzeStream reads proto.Response JSONL from r, evaluates it against the policy,
// and writes AnalyzedResult JSONL to w.
func AnalyzeResponse(ctx context.Context, r io.Reader, w io.Writer, policyContent string) error {
	// 1. Read the entire input body from the reader
	fullInputBytes, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("could not read full input body: %w", err)
	}
	if len(fullInputBytes) == 0 {
		return nil // No input, nothing to do
	}

	// 2. Unmarshal the full input body into a generic type for Rego input
	var fullInput any
	if err := json.Unmarshal(fullInputBytes, &fullInput); err != nil {
		// Log the error and return, as the input is invalid JSON
		return fmt.Errorf("could not unmarshal full input body as JSON: %w", err)
	}

	// 3. Prepare Rego query
	query, err := rego.New(
		rego.Query("data.analyze.analyzed_response"),
		rego.Module("analyze.rego", policyContent),
	).PrepareForEval(ctx)

	if err != nil {
		return fmt.Errorf("could not prepare Rego query: %w", err)
	}

	// 4. Evaluate the Rego policy with the full input
	// The policy's 'input' variable will contain the unmarshaled 'fullInput' data.
	results, err := query.Eval(ctx, rego.EvalInput(fullInput))
	if err != nil {
		return fmt.Errorf("could not evaluate query against full input: %w", err)
	}

	// 5. Process the OPA result
	if len(results) > 0 && results[0].Expressions != nil && len(results[0].Expressions) > 0 {
		// OPA returns an array of results; we take the first expression's value
		opaOutput := results[0].Expressions[0].Value

		// The Rego output is expected to be the final JSON structure, so we just marshal it directly.
		outBytes, err := json.Marshal(opaOutput)
		if err != nil {
			return fmt.Errorf("could not marshal OPA output: %w", err)
		}

		// Write the final analyzed result JSON object followed by a newline
		w.Write(outBytes)
		w.Write([]byte{'\n'})

	} else {
		logger.Info("AnalyzeFullResponse: OPA policy produced no analysis for the full response")
	}

	return nil
}

// AnalyzeCommand is the entry point for the CLI subcommand.
func AnalyzeCommand(ctx context.Context, policyPath string, inputPath string) error {
	policyContent, err := os.ReadFile(policyPath)
	if err != nil {
		return fmt.Errorf("could not read Rego policy file %s: %w", policyPath, err)
	}

	// Determine the input reader
	var r io.Reader = os.Stdin
	var closeFunc func() error = func() error { return nil }

	if inputPath != "" {
		f, err := os.Open(inputPath)
		if err != nil {
			return fmt.Errorf("could not open input file %s: %w", inputPath, err)
		}
		r = f
		closeFunc = f.Close
	}
	// Ensure the file is closed if we opened one
	defer func() {
		if err := closeFunc(); err != nil {
			logger.Error("AnalyzeCommand: failed to close input reader: %v", err)
		}
	}()

	// Use the new function that analyzes the entire response as a single object.
	return AnalyzeResponse(ctx, r, os.Stdout, string(policyContent))
}
