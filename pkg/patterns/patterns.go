package patterns

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	betterleaksconfig "github.com/betterleaks/betterleaks/config"
	//"github.com/jedisct1/go-minisign"

	"github.com/leaktk/leaktk/internal/fs"
	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/leaktk/leaktk/pkg/wellknown"
)

type hashDgst [32]byte

type Patterns struct {
	client          *http.Client
	config          *config.Patterns
	wellKnownClient *wellknown.Client
	mutex           sync.Mutex

	GitleaksPatterns     *betterleaksconfig.Config
	gitleaksPatternsHash hashDgst

	LeaktkPatterns *LeakTKPatterns
}

func NewPatterns(patternsCfg *config.Patterns, client *http.Client) *Patterns {
	return &Patterns{
		client:          client,
		config:          patternsCfg,
		wellKnownClient: wellknown.NewClient(patternsCfg.Server.URL, client, patternsCfg.Server.AuthToken),
	}
}

func sanitizeJSONMap(val interface{}) interface{} {
	switch v := val.(type) {
	case map[string]interface{}:
		for k, child := range v {
			v[k] = sanitizeJSONMap(child)
		}
		return v
	case []interface{}:
		for i, child := range v {
			v[i] = sanitizeJSONMap(child)
		}
		return v
	case float64:
		if v == float64(int64(v)) {
			return int64(v)
		}
		return v
	default:
		return v
	}
}

func (p *Patterns) ConfigurePatterns(ctx context.Context) error {
	cfg := p.config
	wk := p.wellKnownClient.Fetch(ctx)
	if bundleURL, ok := p.wellKnownClient.BundleURL(wk, "latest", "bundle.tar.gz"); ok {
		bundlePath := filepath.Join(p.config.CacheDir, "bundle.tar.gz")
		// sigURL, _ := p.wellKnownClient.BundleSignatureURL(wk, "latest", "bundle.tar.gz")
		// pubKeyURL := p.wellKnownClient.PublicKeyURL(wk)

		modTimeExceeds := fileModTimeExceeds(bundlePath, cfg.RefreshAfter)
		if cfg.Autofetch && modTimeExceeds {
			if err := p.downloadBundle(ctx, bundleURL, bundlePath); err != nil {
				return fmt.Errorf("failed to download bundle: %w", err)
			}
		}
	}

	var err error

	p.GitleaksPatterns, err = p.gitleaks(ctx)
	if err != nil {
		return fmt.Errorf("could not load gitleaks config: %w", err)
	}

	p.LeaktkPatterns, err = p.leakTK(ctx)
	if err != nil {
		return fmt.Errorf("could not load leaktk config: %w", err)
	}

	return nil
}

func (p *Patterns) downloadBundle(ctx context.Context, bundleURL, bundlePath string) error {
	//pubKeyStr, err := fetchPatterns(ctx, p.client, pubKeyURL, p.config.Server.AuthToken)
	//if err != nil {
	//	return fmt.Errorf("failed to fetch public key: %w", err)
	//}

	//pubKey, err := minisign.NewPublicKey(pubKeyStr)
	//if err != nil {
	//	return fmt.Errorf("invalid public key format: %w", err)
	//}

	//sigStr, err := fetchPatterns(ctx, p.client, sigURL, p.config.Server.AuthToken)
	//if err != nil {
	//	return fmt.Errorf("failed to fetch signature: %w", err)
	//}

	//sig, err := minisign.DecodeSignature(sigStr)
	//if err != nil {
	//	return fmt.Errorf("invalid signature format: %w", err)
	//}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bundleURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create bundle request: %w", err)
	}

	if len(p.config.Server.AuthToken) > 0 {
		req.Header.Add("Authorization", "Bearer "+p.config.Server.AuthToken)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download bundle: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bundle download failed with status: %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(bundlePath), 0750); err != nil {
		return fmt.Errorf("failed to create cache dir: %w", err)
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(bundlePath), "bundle-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp bundle file: %w", err)
	}
	tmpName := tmpFile.Name()
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
	}()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return fmt.Errorf("failed to write bundle payload: %w", err)
	}
	_ = tmpFile.Close()

	//bundleBytes, err := os.ReadFile(tmpName)
	//if err != nil {
	//	return fmt.Errorf("failed to read temp bundle for verification: %w", err)
	//}

	//valid, err := pubKey.Verify(bundleBytes, sig)
	//if err != nil || !valid {
	//	return fmt.Errorf("minisign verification failed: bundle payload is tampered or corrupted")
	//}

	return os.Rename(tmpName, bundlePath)
}

func fetchPatterns(ctx context.Context, client *http.Client, patternsURL, authToken string) (string, error) {
	logger.Debug("fetching resource: url=%q", patternsURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, patternsURL, nil)
	if err != nil {
		return "", fmt.Errorf("unable to generate request: %w", err)
	}

	if len(authToken) > 0 {
		req.Header.Add("Authorization", "Bearer "+authToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not get response from client: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("could not read response body: %w", err)
	}

	return string(body), nil
}

func updateLocalPatterns(localPath, rawPatterns string) error {
	if err := os.MkdirAll(filepath.Dir(localPath), 0700); err != nil {
		return fmt.Errorf("could not create patterns dir: %w", err)
	}

	cleanPath := filepath.Clean(localPath)
	patternsFile, err := os.OpenFile(cleanPath, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("could not open patterns file: %w path=%q", err, localPath)
	}

	defer func() {
		if fs.FileLockSupported {
			if err := fs.UnlockFile(patternsFile); err != nil {
				logger.Error("error releasing patterns file lock: %v path=%q", err, localPath)
			}
		}
		if err := patternsFile.Close(); err != nil {
			logger.Error("could not close patterns file: %v path=%q", err, localPath)
		}
	}()

	if fs.FileLockSupported {
		logger.Debug("locking patterns file for writes: path=%q", localPath)
		if err = fs.LockFile(patternsFile); err != nil {
			return fmt.Errorf("could not establish a file lock: %w path=%s", err, localPath)
		}
	}

	if _, err := patternsFile.Seek(0, 0); err != nil {
		return fmt.Errorf("could not seek to beginning of patterns file: %w path=%s", err, localPath)
	}
	if err := patternsFile.Truncate(0); err != nil {
		return fmt.Errorf("could not truncate existing patterns file: %w path=%s", err, localPath)
	}
	if _, err := patternsFile.WriteString(rawPatterns); err != nil {
		return fmt.Errorf("could not write patterns: path=%q error=%q", localPath, err)
	}
	return nil
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
