package ai

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/leaktk/leaktk/pkg/proto"
)

type Analyst struct {
	models *Models
}

func NewAnalyst(m *Models) *Analyst {
	return &Analyst{models: m}
}

type ModelData struct {
	Kind         string             `json:"kind"`
	Coefficients map[string]float64 `json:"coefficients"`
	Keywords     []string           `json:"keywords"`
	Stopwords    []string           `json:"stopwords"`
	Dictwords    []string           `json:"dictwords"`
}

type MLModelsConfig struct {
	Models []ModelData `json:"models"`
}

type AnalysisResult struct {
	PredictedSecretProbability float64
}

type Coefficients struct {
	Intercept                    float64 `json:"intercept"`
	Entropy                      float64 `json:"entropy"`
	LineHasKeyword               float64 `json:"line_has_keyword"`
	NumNumbers                   float64 `json:"num_numbers"`
	MatchHasKeyword              float64 `json:"match_has_keyword"`
	LineHasConsecutiveTrigrams   float64 `json:"line_has_consecutive_trigrams"`
	MatchHasConsecutiveTrigrams  float64 `json:"match_has_consecutive_trigrams"`
	SecretHasConsecutiveTrigrams float64 `json:"secret_has_consecutive_trigrams"`
	NumSpecial                   float64 `json:"num_special"`
	SecretHasKeyword             float64 `json:"secret_has_keyword"`
	LineHasRepeatingTrigrams     float64 `json:"line_has_repeating_trigrams"`
	LineHasStopword              float64 `json:"line_has_stopword"`
	SecretLength                 float64 `json:"secret_length"`
	SecretHasRepeatingTrigrams   float64 `json:"secret_has_repeating_trigrams"`
	MatchHasRepeatingTrigrams    float64 `json:"match_has_repeating_trigrams"`
	SecretHasStopword            float64 `json:"secret_has_stopword"`
	MatchHasStopword             float64 `json:"match_has_stopword"`
	SecretHasDictionaryWord      float64 `json:"secret_has_dictionary_word"`
}

func (a *Analyst) Analyze(model string, modelsConfig *MLModelsConfig, result *proto.Result) (*AnalysisResult, error) {

	var modelData ModelData
	found := false
	for _, mData := range modelsConfig.Models {
		if mData.Kind == model {
			modelData = mData
			found = true
			break
		}
	}

	if !found {
		return nil, fmt.Errorf("model %q not found in configuration", model)
	}

	match := result.Match
	secret := result.Secret
	path := result.Location.Path
	startLine := result.Location.Start.Line

	features := NewFeaturesPipeline(
		match,
		secret,
		path,
		startLine,
		modelData.Keywords,
		modelData.Stopwords,
		modelData.Dictwords,
	)

	coefficients, err := convertCoefficients(modelData.Coefficients)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare model coefficients for scoring: %w", err)
	}

	predictedProbability := -1.0

	if modelData.Kind == "LogisticRegression" {
		predictedProbability = runLogisticRegression(features, coefficients)
	}

	return &AnalysisResult{
		PredictedSecretProbability: predictedProbability,
	}, nil

}

func runLogisticRegression(f *Features, c *Coefficients) float64 {
	z := c.Intercept +
		(f.Entropy/6.03598)*c.Entropy +
		f.LineHasKeyword*c.LineHasKeyword +
		((f.NumNumbers-1)/456)*c.NumNumbers +
		f.MatchHasKeyword*c.MatchHasKeyword +
		f.LineHasConsecutiveTrigrams*c.LineHasConsecutiveTrigrams +
		f.MatchHasConsecutiveTrigrams*c.MatchHasConsecutiveTrigrams +
		f.SecretHasConsecutiveTrigrams*c.SecretHasConsecutiveTrigrams +
		(f.NumSpecial/90)*c.NumSpecial +
		f.SecretHasKeyword*c.SecretHasKeyword +
		f.LineHasRepeatingTrigrams*c.LineHasRepeatingTrigrams +
		f.LineHasStopword*c.LineHasStopword +
		((f.SecretLength-1)/3495)*c.SecretLength +
		f.SecretHasRepeatingTrigrams*c.SecretHasRepeatingTrigrams +
		f.MatchHasRepeatingTrigrams*c.MatchHasRepeatingTrigrams +
		f.SecretHasStopword*c.SecretHasStopword +
		f.MatchHasStopword*c.MatchHasStopword +
		f.SecretHasDictionaryWord*c.SecretHasDictionaryWord
	return 1.0 / (1.0 + math.Exp(-z))
}

func convertCoefficients(coeffMap map[string]float64) (*Coefficients, error) {
	jsonBytes, err := json.Marshal(coeffMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal coefficients map: %w", err)
	}
	var c Coefficients
	if err := json.Unmarshal(jsonBytes, &c); err != nil {
		return nil, fmt.Errorf("failed to unmarshal coefficients JSON to struct: %w", err)
	}
	return &c, nil
}

func ParseConfig(rawConfig string) (*MLModelsConfig, error) {
	config := &MLModelsConfig{}
	err := json.Unmarshal([]byte(rawConfig), config)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal ML models config: %w", err)
	}
	return config, nil
}
