package ai

import (
	"context"
	"fmt"

	"github.com/leaktk/leaktk/pkg/proto"
)

type Analyst struct {
	models *Models
}

func NewAnalyst(m *Models) *Analyst {
	return &Analyst{models: m}
}

type MLModelsConfig struct {
	Models map[string]struct {
		Kind         string             `json:"kind"`
		Coefficients map[string]float64 `json:"coefficients"`
		Keywords     []string           `json:"keywords"`
		Stopwords    []string           `json:"stopwords"`
		Dictwords    []string           `json:"dictwords"`
	} `json:"models"` // Adjust the JSON key to what's actually in the file
}

func (a *Analyst) Analyze(model string, result *proto.Result) (*AnalysisResult, error) {
	modelsConfig, err := a.models.LeakTK(context.Background())
	if err != nil {
		return nil, err
	}

	modelData, ok := modelsConfig.Models[model]
	if !ok {
		return nil, fmt.Errorf("model %q not found", model)
	}

	match := result.Match
	secret := result.Secret

	// 3. Pass the keywords/stopwords to the pipeline
	features := NewFeaturesPipeline(
		match,
		secret,
		modelData.Keywords,
		modelData.Stopwords,
		modelData.Dictwords,
	)

	fmt.Println(features)

	return &AnalysisResult{}, nil

}

type AnalysisResult struct {
	PredictedSecretProbability float64
}
