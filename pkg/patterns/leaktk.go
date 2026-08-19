package patterns

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/v1/rego"
	"github.com/pelletier/go-toml/v2"

	"github.com/leaktk/leaktk/pkg/ai"
	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/leaktk/leaktk/pkg/proto"
)

type LeakTKPatterns struct {
	ModelsConfig []ai.MLModelsConfig
	RegoQuery    rego.PreparedEvalQuery
}

// LeakTK returns the LeakTKPatterns object, handling fetch/caching/update.
func (p *Patterns) LeakTK(ctx context.Context) (*LeakTKPatterns, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	rawPatterns, err := p.fetchLeakTKPatterns(ctx)
	if err != nil {
		return p.leaktkPatterns, err
	}

	config, err := p.parseLeakTKConfig(ctx, rawPatterns)
	if err != nil {
		return nil, fmt.Errorf("could not parse leaktk patterns: %w", err)
	}

	p.leaktkPatterns = config
	logger.Info("fetched leaktk patterns")

	return p.leaktkPatterns, nil
}

func (p *Patterns) fetchLeakTKPatterns(ctx context.Context) (string, error) {
	wk := p.wellKnownClient.Fetch(ctx)
	bundleURL, ok := p.wellKnownClient.BundleURL(wk, "latest", "bundle.tar.gz")
	if !ok {
		return "", fmt.Errorf("failed to locate bundle URL for leaktk")
	}

	bundlePath := filepath.Join(p.config.CacheDir, "bundle.tar.gz")
	if _, err := os.Stat(bundlePath); os.IsNotExist(err) {
		if err := p.downloadBundle(ctx, bundleURL, bundlePath); err != nil {
			return "", fmt.Errorf("failed to download bundle: %w", err)
		}
	}

	f, err := os.Open(bundlePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()

	version := p.config.LeakTK.Version
	versionKey := "v" + strings.ReplaceAll(strings.TrimPrefix(version, "v"), ".", "_")
	regoPath := fmt.Sprintf("/src/leaktk/%s/analyst/policy.rego", versionKey)

	var modelsData interface{}
	var regoContent string

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		cleanName := path.Clean(header.Name)

		if cleanName == "data.json" || cleanName == "/data.json" {
			var rootData map[string]interface{}
			if err := json.NewDecoder(tr).Decode(&rootData); err == nil {
				if provMap, ok := rootData["leaktk"].(map[string]interface{}); ok {
					for _, val := range provMap {
						modelsData = val
						break
					}
				}
			}
		}

		if cleanName == regoPath {
			b, err := io.ReadAll(tr)
			if err == nil {
				regoContent = string(b)
			}
		}
	}

	if modelsData == nil || regoContent == "" {
		return "", fmt.Errorf("could not find both models and rego policy for leaktk %s in bundle", version)
	}

	combinedMap := map[string]interface{}{
		"models":     sanitizeJSONMap(modelsData),
		"opa_policy": regoContent,
	}

	tomlBytes, err := toml.Marshal(combinedMap)
	if err != nil {
		return "", fmt.Errorf("failed to marshal leaktk config: %w", err)
	}

	return string(tomlBytes), nil
}
func (p *Patterns) parseLeakTKConfig(ctx context.Context, rawPatterns string) (*LeakTKPatterns, error) {
	// Mirror the nested TOML structure
	var uncompiled struct {
		Models struct {
			Analyst struct {
				Models struct {
					Models []ai.MLModelsConfig `toml:"models"`
				} `toml:"models"`
			} `toml:"analyst"`
		} `toml:"models"`
		Rego string `toml:"opa_policy"`
	}

	if err := toml.Unmarshal([]byte(rawPatterns), &uncompiled); err != nil {
		return nil, fmt.Errorf("failed to unmarshal leaktk patterns TOML: %w", err)
	}

	modelsConfig := uncompiled.Models.Analyst.Models.Models

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

		analyst := ai.NewAnalyst(modelsConfig)
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
		rego.Module("leaktk.analyst.rego", uncompiled.Rego),
		rego.Function2(ai.RunModelBuiltIn, runModelProvider),
	).PrepareForEval(ctx)

	if err != nil {
		return nil, fmt.Errorf("could not compile rego query: %w", err)
	}

	return &LeakTKPatterns{
		ModelsConfig: modelsConfig,
		RegoQuery:    query,
	}, nil
}
