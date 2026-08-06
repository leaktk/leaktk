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
	"github.com/leaktk/leaktk/pkg/wellknown"
)

type parseFunc func(context.Context, string) (any, error)
type hashDgst [32]byte

// Patterns manages fetching, caching, and updating configuration for
// both gitleaks patterns and LeakTK ML models.
type Patterns struct {
	client *http.Client
	config *config.Patterns
	wellKnownClient *wellknown.Client
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
		wellKnownClient: wellknown.NewClient(patternsCfg.Server.URL, client, patternsCfg.Server.AuthToken),
	}
}

// fetchPatternFromBundle attempts to download the bundle archive and extract <provider>/<vX_Y_Z>/data.json
func (p *Patterns) fetchPatternFromBundle(ctx context.Context, bundleURL, provider, version string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bundleURL, nil)
	if err != nil {
		return "", err
	}
	if len(p.config.Server.AuthToken) > 0 {
		req.Header.Add("Authorization", "Bearer "+p.config.Server.AuthToken)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bundle fetch failed with status: %d", resp.StatusCode)
	}

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read gzip stream: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)

	// Rego/OPA package paths replace periods with underscores (e.g. 8.27.0 -> v8_27_0)
	versionKey := "v" + strings.ReplaceAll(strings.TrimPrefix(version, "v"), ".", "_")
	targetPath := fmt.Sprintf("%s/%s/data.json", provider, versionKey)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		if filepath.Clean(header.Name) == targetPath {
			var jsonData map[string]interface{}
			if err := json.NewDecoder(tr).Decode(&jsonData); err != nil {
				return "", fmt.Errorf("failed to parse json in bundle: %w", err)
			}

			tomlBytes, err := toml.Marshal(jsonData)
			if err != nil {
				return "", fmt.Errorf("failed to encode pattern to TOML: %w", err)
			}

			return string(tomlBytes), nil
		}
	}

	return "", fmt.Errorf("pattern %s/%s not found in bundle", provider, version)
}

func (p *Patterns) resolveAndFetch(ctx context.Context, provider, version string) (string, error) {
	wk := p.wellKnownClient.Fetch(ctx)

	if bundleURL, ok := p.wellKnownClient.BundleURL(wk, "latest", "bundle.tar.gz"); ok {
		rawTOML, err := p.fetchPatternFromBundle(ctx, bundleURL, provider, version)
		if err == nil {
			logger.Debug("extracted pattern %s/%s from bundle", provider, version)
			return rawTOML, nil
		}
		logger.Debug("bundle extraction skipped/failed: %v", err)
	}

	patternURL := p.wellKnownClient.PatternURL(wk, provider, version)
	return fetchPatterns(ctx, p.client, patternURL, p.config.Server.AuthToken)
}

// fetchAndUpdate fetches patterns, checks hash, and updates if changed.
func (p *Patterns) fetchAndUpdate(ctx context.Context, parse parseFunc, provider, version, localPath string, currentHash *hashDgst) (any, *hashDgst, error) {
	rawPatterns, err := p.resolveAndFetch(ctx, provider, version)
	if err != nil {
		return nil, nil, err
	}

	newHash := hashDgst(sha256.Sum256([]byte(rawPatterns)))

	if newHash == *currentHash {
		logger.Debug("skipping update: patterns hash unchanged")
		return nil, nil, nil
	}

	newPatterns, err := parse(ctx, rawPatterns)
	if err != nil {
		return nil, nil, fmt.Errorf("could not parse patterns: %w", err)
	}

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

	if cfg.Autofetch && modTimeExceeds || cfg.Refresh {
		logger.Info("fetching %s patterns", resourceName)

		newPatterns, newHash, err := p.fetchAndUpdate(ctx, parse, resourceName, version, localPath, cachedHash)
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

func fileModTimeExceeds(path string, modTimeLimit int) bool {
	if modTimeLimit == 0 {
		return false
	}

	if fileInfo, err := os.Stat(path); err == nil {
		return int(time.Since(fileInfo.ModTime()).Seconds()) > modTimeLimit
	}

	return true
}

func updateLocalPatterns(localPath, rawPatterns string) error {
	if err := os.MkdirAll(filepath.Dir(localPath), 0700); err != nil {
		return fmt.Errorf("could not create patterns dir: %v", err)
	}

	patternsFile, err := os.OpenFile(localPath, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("could not open patterns file: %v path=%q", err, localPath)
	}

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

	if fs.FileLockSupported {
		logger.Debug("locking patterns file for writes: path=%q", localPath)
		if err = fs.LockFile(patternsFile); err != nil {
			return fmt.Errorf("could not establish a file lock: %w path=%s", err, localPath)
		}
	}

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

func fetchPatterns(ctx context.Context, client *http.Client, patternsURL string, authToken string) (string, error) {
	logger.Debug("fetching resource: url=%q", patternsURL)

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, patternsURL, nil)
	if err != nil {
		return "", err
	}

	if len(authToken) > 0 {
		logger.Debug("setting authorization header")
		request.Header.Add("Authorization", "Bearer "+authToken)
	}

	response, err := client.Do(request)
	if err != nil {
		return "", err
	}

	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			logger.Debug("error closing response body: %v", closeErr)
		}
	}()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: status_code=%d", response.StatusCode)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}
