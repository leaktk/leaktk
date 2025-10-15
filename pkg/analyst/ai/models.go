package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/leaktk/leaktk/pkg/logger"
	// Assuming 'client' is an http.Client available on a struct or passed in
)

type ModelConfig struct {
	Version  string   `json:"version"`
	Features []string `json:"features"`
	// ... other fields
}

func (p *Models) cachePath() string {
	// Assuming p.config.CacheDir is available and contains the base path.
	return filepath.Join(p.cacheDir, "leaktk", "1", "models.json")
}

func (p *Models) parseConfig(rawConfig string) (*MLModelsConfig, error) {
	var modelsConfig MLModelsConfig
	err := json.Unmarshal([]byte(rawConfig), &modelsConfig)
	if err != nil {
		return nil, err
	}
	logger.Info("successfully parsed %d models", len(modelsConfig.Models))
	return &modelsConfig, nil
}

func (p *Models) modelsModTimeExceeds(modTimeLimit int) bool {
	// When modTimeLimit is 0, expiration checking is effectively disabled.
	if modTimeLimit == 0 {
		return false
	}

	localPath := p.cachePath()

	if fileInfo, err := os.Stat(localPath); err == nil {
		return int(time.Since(fileInfo.ModTime()).Seconds()) > modTimeLimit
	}

	// If os.Stat returns an error (e.g., file not found), treat it as expired/missing
	// to force a fetch or fail on expired check.
	return true
}

func (p *Models) GetModels(ctx context.Context) (*MLModelsConfig, error) {
	// Lock since this updates the value of p.mlModelsConfig on the fly
	// and updates files on the filesystem
	p.mutex.Lock()
	defer p.mutex.Unlock()

	localPath := p.cachePath()

	// 1. Autofetch and Refresh Logic
	if p.config.Autofetch && p.modelsModTimeExceeds(p.config.RefreshAfter) {
		// Fetch new data (we will use the existing p.fetchModels from analyst.go)
		rawConfig, err := p.fetchModels(ctx)
		if err != nil {
			// Return existing in-memory config on fetch failure
			return p.mlModelsConfig, fmt.Errorf("failed to fetch models config: %w", err)
		}

		// Parse the fetched config
		parsedConfig, err := p.parseConfig(rawConfig)
		if err != nil {
			logger.Debug("fetched config:\n%s", rawConfig)
			return p.mlModelsConfig, fmt.Errorf("could not parse fetched models config: %w", err)
		}

		// Cache the new config in memory
		p.mlModelsConfig = parsedConfig

		// Write the config to disk after successful parsing
		if err := os.MkdirAll(filepath.Dir(localPath), 0700); err != nil {
			return p.mlModelsConfig, fmt.Errorf("could not create config dir: %w", err)
		}
		if err := os.WriteFile(localPath, []byte(rawConfig), 0600); err != nil {
			return p.mlModelsConfig, fmt.Errorf("could not write config: path=%q error=%w", localPath, err)
		}

		logger.Info("updated ML models configuration")

		// 2. Initial Load or Expired Check
	} else if p.mlModelsConfig == nil { // Not in memory, try loading from disk

		// Check if the local file is expired before loading from disk
		if p.modelsModTimeExceeds(p.config.ExpiredAfter) {
			return nil, fmt.Errorf(
				"ML models config is expired and autofetch is disabled: config_path=%q",
				localPath,
			)
		}

		// Load config from disk
		rawConfig, err := os.ReadFile(localPath)
		if err != nil {
			// File not found or other read error
			return p.mlModelsConfig, fmt.Errorf("failed to read cached config: %w", err)
		}

		// Parse the loaded config
		parsedConfig, err := p.parseConfig(string(rawConfig))
		if err != nil {
			logger.Debug("loaded config:\n%s\n", rawConfig)
			return p.mlModelsConfig, fmt.Errorf("could not parse loaded models config: %w", err)
		}

		// Cache the config in memory
		p.mlModelsConfig = parsedConfig
	}

	// 3. Return the in-memory config
	return p.mlModelsConfig, nil
}

func (p *Models) fetchModels(ctx context.Context) (string, error) {
	logger.Info("fetching AI model configuration")

	// --- 1. Construct the URL ---
	// The path is "patterns/patterns/leaktk/1/models.json".
	modelURL, err := url.JoinPath(
		p.config.Server.URL, // Use p.config instead of f.config
		"patterns",
		"patterns",
		"leaktk",
		"1",
		"models.json",
	)

	logger.Debug("model config url: url=%q", modelURL)
	if err != nil {
		return "", fmt.Errorf("failed to construct model URL: %w", err)
	}

	// --- 2. Create the Request ---
	request, err := http.NewRequestWithContext(ctx, "GET", modelURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// --- 3. Add Authorization Header ---
	if len(p.config.Server.AuthToken) > 0 {
		logger.Debug("setting authorization header")
		request.Header.Add(
			"Authorization",
			"Bearer "+p.config.Server.AuthToken,
		)
	}

	// --- 4. Execute the Request ---
	// Use p.client instead of f.client
	response, err := p.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}

	// Ensure the response body is closed
	defer func() {
		if err := response.Body.Close(); err != nil {
			logger.Debug("error closing response body: %v", err)
		}
	}()

	// --- 5. Check Status Code ---
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code fetching model config: status_code=%d", response.StatusCode)
	}

	// --- 6. Read the Body (NO FILE WRITING) ---
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// Return the raw configuration string
	return string(body), nil
}
