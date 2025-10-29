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

type Analyst struct {
	query rego.PreparedEvalQuery
	ctx   context.Context
}

// NewAnalyst initializes and prepares the Rego engine. This should be called only once.
func NewAnalyst(ctx context.Context, policyContent string) (*Analyst, error) {
	query, err := rego.New(
		rego.Query("data.analyze.analyzed_response"),
		rego.Module("analyze.rego", policyContent),
	).PrepareForEval(ctx)

	if err != nil {
		return nil, fmt.Errorf("could not prepare Rego query: %w", err)
	}

	return &Analyst{
		query: query,
		ctx:   ctx,
	}, nil
}

func (a *Analyst) Analyze(response *proto.Response) (*proto.Response, error) {
	// Marshal the struct into JSON bytes to serve as OPA input
	inputBytes, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response struct for OPA input: %w", err)
	}

	// Unmarshal into a generic type for Rego
	var opaInput any
	if err := json.Unmarshal(inputBytes, &opaInput); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON into generic type for OPA: %w", err)
	}

	// Evaluate the Rego policy
	results, err := a.query.Eval(a.ctx, rego.EvalInput(opaInput))
	if err != nil {
		return nil, fmt.Errorf("could not evaluate query for response ID %s: %w", response.ID, err)
	}

	if len(results) == 0 || results[0].Expressions == nil || len(results[0].Expressions) == 0 {
		// If Rego produced no output, return the original response without analysis
		logger.Info("Analyze: OPA policy produced no analysis for response ID %s", response.ID)
		return response, nil
	}

	// Extract the OPA output (the enriched JSON structure)
	opaOutput := results[0].Expressions[0].Value

	// Marshal the enriched structure back to JSON bytes
	outputBytes, err := json.Marshal(opaOutput)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal OPA output to JSON: %w", err)
	}

	// Unmarshal the enriched JSON back into a new Response struct
	var enrichedResponse proto.Response
	if err := json.Unmarshal(outputBytes, &enrichedResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal enriched JSON back into Response struct: %w", err)
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
		enrichedResponse, err := a.Analyze(&response)
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
func AnalyzeCommand(ctx context.Context, policyPath string, inputPath string) error {
	// Read Policy Content
	policyContent, err := os.ReadFile(policyPath)
	if err != nil {
		return fmt.Errorf("could not read Rego policy file %s: %w", policyPath, err)
	}

	// Initialize the Analyst once
	analyst, err := NewAnalyst(ctx, string(policyContent))
	if err != nil {
		return fmt.Errorf("failed to initialize analyst: %w", err)
	}

	// Determine the input reader (File or Stdin)
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
	defer func() {
		if err := closeFunc(); err != nil {
			logger.Error("AnalyzeCommand: failed to close input reader: %v", err)
		}
	}()

	// Start processing the JSONL stream
	return AnalyzeStream(analyst, r, os.Stdout)
}
