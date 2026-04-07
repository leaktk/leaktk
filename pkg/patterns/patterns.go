package patterns

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	betterleaksconfig "github.com/betterleaks/betterleaks/config"

	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/logger"
)

// Patterns manages fetching, caching, and updating configuration for
// both gitleaks patterns and LeakTK ML models.
type Patterns struct {
	client *http.Client
	config *config.Patterns
	mutex  sync.Mutex

	// Gitleaks Patterns fields
	gitleaksPatterns     *betterleaksconfig.Config
	gitleaksPatternsHash [32]byte

	// LeakTK Models fields
	leaktkPatterns     *LeakTKPatterns
	leaktkPatternsHash [32]byte
}

// NewPatterns returns a configured instance of Patterns.
func NewPatterns(patternsCfg *config.Patterns, client *http.Client) *Patterns {
	return &Patterns{
		client: client,
		config: patternsCfg,
	}
}

// fileModTimeExceeds returns true if the local configuration file at 'path'
// is older than 'modTimeLimit' seconds.
func (c *Patterns) fileModTimeExceeds(path string, modTimeLimit int) bool {
	if modTimeLimit == 0 {
		return false
	}

	if fileInfo, err := os.Stat(path); err == nil {
		return int(time.Since(fileInfo.ModTime()).Seconds()) > modTimeLimit
	}

	return true
}

// fetchPatterns fetches the raw patterns from the server.
func (c *Patterns) fetchPatterns(ctx context.Context, patternsURL string, authToken string) (string, error) {
	logger.Debug("fetching patterns: url=%q", patternsURL)

	request, err := http.NewRequestWithContext(ctx, "GET", patternsURL, nil)
	if err != nil {
		return "", err
	}

	if len(authToken) > 0 {
		logger.Debug("setting authorization header")
		request.Header.Add("Authorization", "Bearer "+authToken)
	}

	response, err := c.client.Do(request) // #nosec G704
	if err != nil {
		return "", err
	}

	defer (func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			logger.Debug("error closing pattern response body: %v", closeErr)
		}
	})()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: status_code=%d", response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// updateLocalPatterns writes the raw patterns content to the specified local file path.
// It creates the directory if it does not exist.
func (c *Patterns) updateLocalPatterns(localPath, rawPatterns string) error {
	if err := os.MkdirAll(filepath.Dir(localPath), 0700); err != nil {
		return fmt.Errorf("could not create patterns dir: %v", err)
	}

	if err := os.WriteFile(localPath, []byte(rawPatterns), 0600); err != nil {
		return fmt.Errorf("could not write patterns: %v path=%q", err, localPath)
	}

	return nil
}

// fetchURLFor constructs the fetch URL for a given provider and version.
func (c *Patterns) fetchURLFor(provider, version string) (string, error) {
	return url.JoinPath(
		c.config.Server.URL, "patterns", provider, version,
	)
}

// fetchAndUpdate fetches patterns from server, checks hash, and updates if changed.
// Returns (rawPatterns, hashChanged, error).
func (c *Patterns) fetchAndUpdate(ctx context.Context, fetchURL, localPath string, currentHash [32]byte) (string, bool, error) {
	rawPatterns, err := c.fetchPatterns(ctx, fetchURL, c.config.Server.AuthToken)
	if err != nil {
		return "", false, err
	}

	// Calculate hash of fetched content
	newHash := sha256.Sum256([]byte(rawPatterns))

	// Only update if hash changed
	if newHash == currentHash {
		logger.Debug("skipping update: patterns hash unchanged")
		return rawPatterns, false, nil
	}

	// Hash changed, write to disk
	if err := c.updateLocalPatterns(localPath, rawPatterns); err != nil {
		return rawPatterns, false, err
	}

	return rawPatterns, true, nil
}

// loadFromDisk loads patterns from local file path.
func (c *Patterns) loadFromDisk(localPath string) (string, error) {
	rawPatterns, err := os.ReadFile(filepath.Clean(localPath))
	if err != nil {
		return "", err
	}
	return string(rawPatterns), nil
}

func getOrUpdate[T any](
	ctx context.Context,
	c *Patterns,
	cachedConfig **T,
	cachedHash *[32]byte,
	resourceName string,
	localPath string,
	version string,
	parseFunc func(ctx context.Context, raw string) (*T, error),
) (*T, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	cfg := c.config
	modTimeExceeds := c.fileModTimeExceeds(localPath, cfg.RefreshAfter)

	if cfg.Autofetch && modTimeExceeds {
		logger.Info("fetching %s patterns", resourceName)

		fetchURL, err := c.fetchURLFor(resourceName, version)
		if err != nil {
			return *cachedConfig, err
		}

		rawPatterns, hashChanged, err := c.fetchAndUpdate(ctx, fetchURL, localPath, *cachedHash)
		if err != nil {
			return *cachedConfig, err
		}

		if hashChanged {
			newConfig, err := parseFunc(ctx, rawPatterns)
			if err != nil {
				logger.Debug("fetched config:\n%s", rawPatterns)
				return *cachedConfig, fmt.Errorf("could not parse %s config: error=%q", resourceName, err)
			}
			*cachedConfig = newConfig
			*cachedHash = sha256.Sum256([]byte(rawPatterns))
			logger.Info("updated %s patterns", resourceName)
		}
	} else if any(cachedConfig) == nil || reflect.ValueOf(cachedConfig).Elem().IsNil() {
		if c.fileModTimeExceeds(localPath, cfg.ExpiredAfter) {
			return nil, fmt.Errorf("%s config is expired and autofetch is disabled: path=%q", resourceName, localPath)
		}

		rawPatterns, err := c.loadFromDisk(localPath)
		if err != nil {
			return nil, err
		}

		newConfig, err := parseFunc(ctx, rawPatterns)
		if err != nil {
			logger.Debug("loaded config:\n%s\n", rawPatterns)
			return nil, fmt.Errorf("could not parse %s config: error=%q", resourceName, err)
		}
		*cachedConfig = newConfig
		*cachedHash = sha256.Sum256([]byte(rawPatterns))
	}

	return *cachedConfig, nil
}
