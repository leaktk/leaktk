package analyst

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/leaktk/leaktk/pkg/proto"
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
func AnalyzeStream(ctx context.Context, r io.Reader, w io.Writer, policyContent string) error {
	scanner := bufio.NewScanner(r)

	query, err := rego.New(
		rego.Query("data.analyze.analyzed_response"),
		rego.Module("analyze.rego", policyContent),
	).PrepareForEval(ctx)

	if err != nil {
		return fmt.Errorf("could not prepare Rego query: %w", err)
	}

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var response proto.Response
		if err := json.Unmarshal(line, &response); err != nil {
			logger.Error("AnalyzeStream: could not unmarshal line to proto.Response: %v, line: %s", err, string(line))
			continue
		}

		results, err := query.Eval(ctx, rego.EvalInput(response))
		if err != nil {
			logger.Error("AnalyzeStream: could not evaluate query for response ID %s: %v", response.ID, err)
			continue
		}

		if len(results) > 0 && results[0].Expressions != nil && len(results[0].Expressions) > 0 {
			// OPA returns an array of results; we take the first expression's value
			opaOutput := results[0].Expressions[0].Value

			// Construct the final analyzed result
			analyzed := AnalyzedResult{
				ID:        response.ID,
				RequestID: response.RequestID,
				Analysis:  opaOutput, // This is the value of 'data.analyze.analyzed_response'
			}

			// Write the final AnalyzedResult JSON object followed by a newline (JSONL)
			outBytes, err := json.Marshal(analyzed)
			if err != nil {
				logger.Error("AnalyzeStream: could not marshal AnalyzedResult: %v", err)
				continue
			}

			w.Write(outBytes)
			w.Write([]byte{'\n'})
		} else {
			logger.Info("AnalyzeStream: OPA policy produced no analysis for response ID %s", response.ID)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading from input stream: %w", err)
	}

	return nil
}

// AnalyzeCommand is the entry point for the CLI subcommand.
func AnalyzeCommand(ctx context.Context, policyPath string) error {
	policyContent, err := os.ReadFile(policyPath)
	if err != nil {
		return fmt.Errorf("could not read Rego policy file %s: %w", policyPath, err)
	}

	return AnalyzeStream(ctx, os.Stdin, os.Stdout, string(policyContent))
}
