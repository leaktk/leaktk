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

// TODO: swap this out with an embed
const policy = `
package main

analysis := input
`

type AnalyzedResult struct {
	ID        string `json:"id"`
	RequestID string `json:"request_id"`
	Results   any    `json:"results"` // Will hold the array of analyzed findings
}

func (a AnalyzedResult) String() string {
	out, err := json.Marshal(a)
	if err != nil {
		return fmt.Sprintf("Error marshaling analysis: %v", err)
	}
	return string(out)
}

func AnalyzeStream(ctx context.Context, r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)

	// 1. Prepare the Rego Query once for efficiency
	// We use data.analyze.analyzed_response as the query path
	query, err := rego.New(
		rego.Query("data.analyze.analyzed_response"),
		rego.Module("analyze.rego", policy),
	).PrepareForEval(ctx)

	if err != nil {
		return fmt.Errorf("could not create query: %w", err)
	}

	// 2. Iterate through the input stream, line by line (JSONL)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue // Skip empty lines
		}

		var result proto.Result
		if err := json.Unmarshal(line, &result); err != nil {
			logger.Error("AnalyzeStream: could not unmarshal line to proto.Result: %v, line: %s", err, string(line))
			// Skip to the next line on unmarshal error
			continue
		}

		// 3. Marshal the single proto.Result object into JSON for OPA input
		inputBytes, err := json.Marshal(result)
		if err != nil {
			logger.Error("AnalyzeStream: could not marshal proto.Result to JSON: %v", err)
			continue
		}

		// 4. Evaluate the query with the single Result object as input
		results, err := query.Eval(ctx, rego.EvalInput(string(inputBytes)))
		if err != nil {
			logger.Error("AnalyzeStream: could not evaluate query: %v", err)
			continue
		}

		// 5. Process OPA output and write to output stream (JSONL)
		if len(results) > 0 && results[0].Expressions != nil && len(results[0].Expressions) > 0 {
			// OPA returns an array of results; we take the first expression's value
			opaOutput := results[0].Expressions[0].Value

			// The output should conform to the structure you defined (analyzed_response)
			outBytes, err := json.Marshal(opaOutput)
			if err != nil {
				logger.Error("AnalyzeStream: could not marshal OPA output: %v", err)
				continue
			}

			// Write the final JSON object followed by a newline (JSONL)
			w.Write(outBytes)
			w.Write([]byte{'\n'})
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading from input stream: %w", err)
	}

	return nil
}

func AnalyzeCommand(ctx context.Context) error {
	// This is the function you would call in your main CLI logic
	// It reads from os.Stdin and writes to os.Stdout
	return AnalyzeStream(ctx, os.Stdin, os.Stdout)
}
