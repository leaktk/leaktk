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
	"sync"
	"time"

	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/logger"
	// Assuming 'client' is an http.Client available on a struct or passed in
)

type Models struct {
	client       *http.Client
	config       *config.Patterns
	modelsConfig *MLModelsConfig
	mutex        sync.Mutex
}

func NewModels(cfg *config.Patterns, client *http.Client) *Models {
	return &Models{
		client: client,
		config: cfg,
	}
}

func (m *Models) fetchModels(ctx context.Context) (string, error) {
	logger.Info("fetching AI model configuration")
	modelURL, err := url.JoinPath(
		m.config.Server.URL, // Use m.config instead of f.config
		"patterns",
		"leaktk",
		"1",
		"models.json",
	)

	logger.Debug("model config url: url=%q", modelURL)
	if err != nil {
		return "", fmt.Errorf("failed to construct model URL: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, "GET", modelURL, nil)
	if err != nil {
		return "", err
	}

	if len(m.config.Server.AuthToken) > 0 {
		logger.Debug("setting authorization header")
		request.Header.Add(
			"Authorization",
			"Bearer "+m.config.Server.AuthToken,
		)
	}

	response, err := m.client.Do(request)
	if err != nil {
		return "", err
	}

	defer func() {
		if err := response.Body.Close(); err != nil {
			logger.Debug("error closing model response body: %v", err)
		}
	}()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code fetching model config: status_code=%d", response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	return string(body), nil
}

// leakTKConfigModTimeExceeds returns true if the file is older than
// `modTimeLimit` seconds
func (m *Models) leakTKConfigModTimeExceeds(modTimeLimit int) bool {
	// When modTimeLimit is 0, expiration checking is effectively disabled
	// and leakTKConfigModTimeExceeds returns false in this case.
	if modTimeLimit == 0 {
		return false
	}

	if fileInfo, err := os.Stat(m.config.LeakTK.LocalPath); err == nil {
		return int(time.Since(fileInfo.ModTime()).Seconds()) > modTimeLimit
	}

	return true
}

// LeakTK returns a LeakTK Models config object if it's able to
func (m *Models) LeakTK(ctx context.Context) (*MLModelsConfig, error) {
	// Lock since this updates the value of m.modelsConfig on the fly
	// and updates files on the filesystem
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.config.Autofetch && m.leakTKConfigModTimeExceeds(m.config.RefreshAfter) {
		rawConfig, err := m.fetchModels(ctx)
		if err != nil {
			return m.modelsConfig, err

		}

		m.modelsConfig, err = m.parseConfig(rawConfig)
		if err != nil {
			logger.Debug("fetched config:\n%s", rawConfig)

			return m.modelsConfig, fmt.Errorf("could not parse config: error=%q", err)
		}

		if err := os.MkdirAll(filepath.Dir(m.config.LeakTK.LocalPath), 0700); err != nil {
			return m.modelsConfig, fmt.Errorf("could not create config dir: error=%q", err)
		}

		// only write the config after parsing it, that way we don't break a good
		// existing config if the server returns an invalid response
		if err := os.WriteFile(m.config.LeakTK.LocalPath, []byte(rawConfig), 0600); err != nil {
			return m.modelsConfig, fmt.Errorf("could not write config: path=%q error=%q", m.config.LeakTK.LocalPath, err)
		}

		// if hash := sha256.Sum256([]byte(rawConfig)); p.gitleaksConfigHash != hash {
		// 	p.gitleaksConfigHash = hash
		// 	logger.Info("updated gitleaks patterns: hash=%s", p.GitleaksConfigHash())
		// }
	} else if m.modelsConfig == nil {
		if m.leakTKConfigModTimeExceeds(m.config.ExpiredAfter) {
			return nil, fmt.Errorf(
				"leaktk config is expired and autofetch is disabled: config_path=%q",
				m.config.LeakTK.LocalPath,
			)
		}

		rawConfig, err := os.ReadFile(m.config.LeakTK.LocalPath)
		if err != nil {
			return m.modelsConfig, err
		}

		m.modelsConfig, err = m.parseConfig(string(rawConfig))
		if err != nil {
			logger.Debug("loaded config:\n%s\n", rawConfig)

			return m.modelsConfig, fmt.Errorf("could not parse config: error=%q", err)
		}

		// if hash := sha256.Sum256(rawConfig); p.gitleaksConfigHash != hash {
		// 	p.gitleaksConfigHash = hash
		// }
	}

	return m.modelsConfig, nil
}

func (m *Models) parseConfig(rawConfig string) (*MLModelsConfig, error) {
	var modelsConfig MLModelsConfig
	err := json.Unmarshal([]byte(rawConfig), &modelsConfig)
	if err != nil {
		return nil, err
	}
	logger.Info("successfully parsed %d models", len(modelsConfig.Models))
	return &modelsConfig, nil
}
