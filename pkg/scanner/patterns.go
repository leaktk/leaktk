package scanner

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	gitleaksconfig "github.com/zricethezav/gitleaks/v8/config"

	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/leaktk/leaktk/pkg/scanner/gitleaks"
)

// Patterns acts as an abstraction for fetching different scanner patterns
// and keeping them up to date and cached
type Patterns struct {
	client             *http.Client
	config             *config.Patterns
	gitleaksConfigHash [32]byte
	gitleaksConfig     *gitleaksconfig.Config
	mutex              sync.Mutex
}

// NewPatterns returns a configured instance of Patterns
func NewPatterns(cfg *config.Patterns, client *http.Client) *Patterns {
	return &Patterns{
		client: client,
		config: cfg,
	}
}

func (p *Patterns) fetchGitleaksConfig(ctx context.Context) (string, error) {
	logger.Info("fetching gitleaks patterns")
	patternURL, err := url.JoinPath(
		p.config.Server.URL, "patterns", "gitleaks", p.config.Gitleaks.Version,
	)

	logger.Debug("patterns url: url=%q", patternURL)
	if err != nil {
		return "", err
	}

	request, err := http.NewRequestWithContext(ctx, "GET", patternURL, nil)
	if err != nil {
		return "", err
	}

	if len(p.config.Server.AuthToken) > 0 {
		logger.Debug("setting authorization header")
		request.Header.Add(
			"Authorization",
			"Bearer "+p.config.Server.AuthToken,
		)
	}

	response, err := p.client.Do(request)
	if err != nil {
		return "", err
	}

	defer (func() {
		if err := response.Body.Close(); err != nil {
			logger.Debug("error closing pattern response body: %v", err)
		}
	})()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: status_code=%d", response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}

	return string(body), err
}

// gitleaksConfigModTimeExceeds returns true if the file is older than
// `modTimeLimit` seconds
func (p *Patterns) gitleaksConfigModTimeExceeds(modTimeLimit int) bool {
	// When modTimeLimit is 0, expiration checking is effectively disabled
	// and gitleaksConfigModTimeExceeds returns false in this case.
	if modTimeLimit == 0 {
		return false
	}

	if fileInfo, err := os.Stat(p.config.Gitleaks.LocalPath); err == nil {
		return int(time.Since(fileInfo.ModTime()).Seconds()) > modTimeLimit
	}

	return true
}

// Gitleaks returns a Gitleaks config object if it's able to
func (p *Patterns) Gitleaks(ctx context.Context) (*gitleaksconfig.Config, error) {
	// Lock since this updates the value of p.gitleaksConfig on the fly
	// and updates files on the filesystem
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.config.Autofetch && p.gitleaksConfigModTimeExceeds(p.config.RefreshAfter) {
		rawConfig, err := p.fetchGitleaksConfig(ctx)
		if err != nil {
			return p.gitleaksConfig, err
		}

		p.gitleaksConfig, err = gitleaks.ParseConfig(rawConfig)
		if err != nil {
			logger.Debug("fetched config:\n%s", rawConfig)

			return p.gitleaksConfig, fmt.Errorf("could not parse config: error=%q", err)
		}

		if err := os.MkdirAll(filepath.Dir(p.config.Gitleaks.LocalPath), 0700); err != nil {
			return p.gitleaksConfig, fmt.Errorf("could not create config dir: error=%q", err)
		}

		// only write the config after parsing it, that way we don't break a good
		// existing config if the server returns an invalid response
		if err := os.WriteFile(p.config.Gitleaks.LocalPath, []byte(rawConfig), 0600); err != nil {
			return p.gitleaksConfig, fmt.Errorf("could not write config: path=%q error=%q", p.config.Gitleaks.LocalPath, err)
		}

		if hash := sha256.Sum256([]byte(rawConfig)); p.gitleaksConfigHash != hash {
			p.gitleaksConfigHash = hash
			logger.Info("updated gitleaks patterns: hash=%s", p.GitleaksConfigHash())
		}
	} else if p.gitleaksConfig == nil {
		if p.gitleaksConfigModTimeExceeds(p.config.ExpiredAfter) {
			return nil, fmt.Errorf(
				"gitleaks config is expired and autofetch is disabled: config_path=%q",
				p.config.Gitleaks.LocalPath,
			)
		}

		rawConfig, err := os.ReadFile(p.config.Gitleaks.LocalPath)
		if err != nil {
			return p.gitleaksConfig, err
		}

		p.gitleaksConfig, err = gitleaks.ParseConfig(string(rawConfig))
		if err != nil {
			logger.Debug("loaded config:\n%s\n", rawConfig)

			return p.gitleaksConfig, fmt.Errorf("could not parse config: error=%q", err)
		}

		if hash := sha256.Sum256(rawConfig); p.gitleaksConfigHash != hash {
			p.gitleaksConfigHash = hash
		}
	}

	return p.gitleaksConfig, nil
}

// GitleaksConfigHash returns the sha256 hash for the current gitleaks config
func (p *Patterns) GitleaksConfigHash() string {
	return fmt.Sprintf("%x", p.gitleaksConfigHash)
}
