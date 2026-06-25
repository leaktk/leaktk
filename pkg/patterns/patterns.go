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
	"sync"
	"time"

	betterleaksconfig "github.com/betterleaks/betterleaks/config"

	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/fs"
	"github.com/leaktk/leaktk/pkg/logger"
)

type parseFunc func(context.Context, string) (any, error)
type hashDgst [32]byte

// Patterns manages fetching, caching, and updating configuration for
// both gitleaks patterns and LeakTK ML models.
type Patterns struct {
	client *http.Client
	config *config.Patterns
	mutex  sync.Mutex

	// Gitleaks Patterns fields
	gitleaksPatterns     *betterleaksconfig.Config
	gitleaksPatternsHash hashDgst

	// LeakTK Models fields
	leaktkPatterns     *LeakTKPatterns
	leaktkPatternsHash hashDgst
}

// NewPatterns returns a configured instance of Patterns.
func NewPatterns(patternsCfg *config.Patterns, client *http.Client) *Patterns {
	return &Patterns{
		client: client,
		config: patternsCfg,
	}
}

// fetchURLFor constructs the fetch URL for a given provider and version.
func (p *Patterns) fetchURLFor(provider, version string) (string, error) {
	return url.JoinPath(p.config.Server.URL, "patterns", provider, version)
}

// fetchAndUpdate fetches patterns from server, checks hash, and updates if changed.
func (p *Patterns) fetchAndUpdate(ctx context.Context, parse parseFunc, fetchURL, localPath string, currentHash *hashDgst) (any, *hashDgst, error) {
	logger.Info("fetchURL:", fetchURL)
	rawPatterns, err := fetchPatterns(ctx, p.client, fetchURL, p.config.Server.AuthToken)
	if err != nil {
		return nil, nil, err
	}

	// Calculate hash of fetched content
	newHash := hashDgst(sha256.Sum256([]byte(rawPatterns)))

	// Only update if hash changed
	if newHash == *currentHash {
		logger.Debug("skipping update: patterns hash unchanged")
		return nil, nil, nil
	}

	// Parse before writing to disk to confirm they're good
	newPatterns, err := parse(ctx, rawPatterns)
	if err != nil {
		return nil, nil, fmt.Errorf("could not parse patterns: %w", err)
	}

	// Hash changed, write to disk
	if err := updateLocalPatterns(localPath, rawPatterns); err != nil {
		return newPatterns, nil, err
	}

	return newPatterns, &newHash, nil
}

// loadFromDisk loads patterns from local file path.
func (p *Patterns) loadFromDisk(localPath string) (string, error) {
	rawPatterns, err := os.ReadFile(filepath.Clean(localPath))
	if err != nil {
		return "", err
	}
	return string(rawPatterns), nil
}

func getOrUpdate[T any](
	ctx context.Context,
	p *Patterns,
	cachedPatterns **T,
	cachedHash *hashDgst,
	resourceName string,
	localPath string,
	version string,
	parse parseFunc,
) (*T, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	cfg := p.config
	modTimeExceeds := fileModTimeExceeds(localPath, cfg.RefreshAfter)

	if cfg.Autofetch && modTimeExceeds || cfg.Refresh == true {
		logger.Info("fetching %s patterns", resourceName)

		fetchURL, err := p.fetchURLFor(resourceName, version)
		if err != nil {
			return *cachedPatterns, err
		}

		newPatterns, newHash, err := p.fetchAndUpdate(ctx, parse, fetchURL, localPath, cachedHash)
		if err != nil {
			return *cachedPatterns, err
		}

		if newHash != nil {
			*cachedPatterns = newPatterns.(*T)
			*cachedHash = *newHash
			logger.Info("updated %s patterns", resourceName)
		}
	} else if cachedPatterns == nil || *cachedPatterns == nil {
		if fileModTimeExceeds(localPath, cfg.ExpiredAfter) {
			return nil, fmt.Errorf("%s config is expired and autofetch is disabled: path=%q", resourceName, localPath)
		}

		rawPatterns, err := p.loadFromDisk(localPath)
		if err != nil {
			return nil, err
		}

		newConfig, err := parse(ctx, rawPatterns)
		if err != nil {
			logger.Debug("loaded config:\n%s\n", rawPatterns)
			return nil, fmt.Errorf("could not parse %s config: error=%q", resourceName, err)
		}
		*cachedPatterns = newConfig.(*T)
		*cachedHash = hashDgst(sha256.Sum256([]byte(rawPatterns)))
	}

	return *cachedPatterns, nil
}

// fileModTimeExceeds returns true if the local configuration file at 'path' is
// older than 'modTimeLimit' seconds.
func fileModTimeExceeds(path string, modTimeLimit int) bool {
	if modTimeLimit == 0 {
		return false
	}

	if fileInfo, err := os.Stat(path); err == nil {
		return int(time.Since(fileInfo.ModTime()).Seconds()) > modTimeLimit
	}

	return true
}

// updateLocalPatterns writes the raw patterns content to the specified local
// file path. It creates the directory if it does not exist.
func updateLocalPatterns(localPath, rawPatterns string) error {
	if err := os.MkdirAll(filepath.Dir(localPath), 0700); err != nil {
		return fmt.Errorf("could not create patterns dir: %v", err)
	}

	// Open the patterns file, creating it if it doesn't already exist, but don't truncate yet
	patternsFile, err := os.OpenFile(localPath, os.O_RDWR|os.O_CREATE, 0600) // #nosec G304
	if err != nil {
		return fmt.Errorf("could not open patterns file: %v path=%q", err, localPath)
	}

	// Defer the close and add logging around it since we're adding locks
	defer func() {
		if err := patternsFile.Close(); err != nil {
			logger.Error("could not close patterns file: %v path=%q", err, localPath)
			if fs.FileLockSupported {
				if err := fs.UnlockFile(patternsFile); err != nil {
					logger.Error("error releasing patterns file lock: %v path=%q", err, localPath)
				}
			}
		}
	}()

	// Establish a file lock to avoid different instances of the scanner writing to the file
	if fs.FileLockSupported {
		logger.Debug("locking patterns file for writes: path=%q", localPath)
		if err = fs.LockFile(patternsFile); err != nil {
			return fmt.Errorf("could not establish a file lock: %w path=%s", err, localPath)
		}
	}

	// Now that a lock's established if it's supported, seek to the beginning to be safe, truncate and write the file
	if _, err := patternsFile.Seek(0, 0); err != nil {
		return fmt.Errorf("could not seek to the beginning of the patterns file: %w path=%s", err, localPath)
	}
	if err := patternsFile.Truncate(0); err != nil {
		return fmt.Errorf("could not truncate existing patterns file: %w path=%s", err, localPath)
	}
	if _, err := patternsFile.WriteString(rawPatterns); err != nil {
		return fmt.Errorf("could not write patterns: path=%q error=%q", localPath, err)
	}
	return nil
}

// fetchPatterns fetches the raw patterns from the server.
func fetchPatterns(ctx context.Context, client *http.Client, patternsURL string, authToken string) (string, error) {
	logger.Debug("fetching patterns: url=%q", patternsURL)

	request, err := http.NewRequestWithContext(ctx, "GET", patternsURL, nil)
	if err != nil {
		return "", err
	}

	if len(authToken) > 0 {
		logger.Debug("setting authorization header")
		request.Header.Add("Authorization", "Bearer "+authToken)
	}

	response, err := client.Do(request) // #nosec G704
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
