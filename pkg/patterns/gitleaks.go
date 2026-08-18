package patterns

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"

	betterleaksconfig "github.com/betterleaks/betterleaks/config"
	"github.com/pelletier/go-toml/v2"

	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/leaktk/leaktk/pkg/scanner/betterleaks"
)

// Gitleaks returns a Gitleaks config object, fetching/caching/updating as necessary.
func (p *Patterns) Gitleaks(ctx context.Context) (*betterleaksconfig.Config, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	cfg := p.config
	localPath := cfg.Gitleaks.LocalPath
	modTimeExceeds := fileModTimeExceeds(localPath, cfg.RefreshAfter)

	if cfg.Autofetch && modTimeExceeds || cfg.Refresh {
		logger.Info("fetching gitleaks patterns")

		rawPatterns, err := p.fetchGitleaksPatterns(ctx)
		if err != nil {
			return p.gitleaksPatterns, err
		}

		newHash := hashDgst(sha256.Sum256([]byte(rawPatterns)))
		if newHash == p.gitleaksPatternsHash {
			logger.Debug("skipping update: gitleaks patterns hash unchanged")
			return p.gitleaksPatterns, nil
		}

		newConfig, err := p.parseGitleaksConfig(ctx, rawPatterns)
		if err != nil {
			return nil, fmt.Errorf("could not parse gitleaks patterns: %w", err)
		}

		if err := updateLocalPatterns(localPath, rawPatterns); err != nil {
			return newConfig, err
		}

		p.gitleaksPatterns = newConfig
		p.gitleaksPatternsHash = newHash
		logger.Info("updated gitleaks patterns")
	} else if p.gitleaksPatterns == nil {
		if fileModTimeExceeds(localPath, cfg.ExpiredAfter) {
			return nil, fmt.Errorf("gitleaks config is expired and autofetch is disabled: path=%q", localPath)
		}

		rawPatternsBytes, err := os.ReadFile(filepath.Clean(localPath))
		if err != nil {
			return nil, err
		}

		rawPatterns := string(rawPatternsBytes)
		newConfig, err := p.parseGitleaksConfig(ctx, rawPatterns)
		if err != nil {
			logger.Debug("loaded config:\n%s\n", rawPatterns)
			return nil, fmt.Errorf("could not parse gitleaks config: error=%q", err)
		}

		p.gitleaksPatterns = newConfig
		p.gitleaksPatternsHash = hashDgst(sha256.Sum256(rawPatternsBytes))
	}

	return p.gitleaksPatterns, nil
}

// GitleaksConfigHash returns the sha256 hash for the current gitleaks config.
func (p *Patterns) GitleaksConfigHash() string {
	return fmt.Sprintf("%x", p.gitleaksPatternsHash)
}

func (p *Patterns) parseGitleaksConfig(_ context.Context, rawPatterns string) (*betterleaksconfig.Config, error) {
	cfg, err := betterleaks.ParseConfig(rawPatterns)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func (p *Patterns) fetchGitleaksPatterns(ctx context.Context) (string, error) {
	wk := p.wellKnownClient.Fetch(ctx)
	version := p.config.Gitleaks.Version

	if bundleURL, ok := p.wellKnownClient.BundleURL(wk, "latest", "bundle.tar.gz"); ok {
		bundlePath := filepath.Join(p.config.CacheDir, "bundle.tar.gz")
		if _, err := os.Stat(bundlePath); os.IsNotExist(err) {
			_ = p.downloadBundle(ctx, bundleURL, bundlePath)
		}

		if f, err := os.Open(bundlePath); err == nil {
			if gz, err := gzip.NewReader(f); err == nil {
				tr := tar.NewReader(gz)
				for {
					header, err := tr.Next()
					if err != nil {
						break
					}
					cleanName := path.Clean(header.Name)
					if cleanName == "data.json" || cleanName == "/data.json" {
						var rootData map[string]interface{}
						if err := json.NewDecoder(tr).Decode(&rootData); err == nil {
							if provMap, ok := rootData["gitleaks"].(map[string]interface{}); ok {
								for _, val := range provMap {
									sanitized := sanitizeJSONMap(val)
									if b, err := toml.Marshal(sanitized); err == nil {
										_ = gz.Close()
										_ = f.Close()
										return string(b), nil
									}
								}
							}
						}
					}
				}
				_ = gz.Close()
			}
			_ = f.Close()
		}
	}

	patternURL := p.wellKnownClient.PatternURL(wk, "gitleaks", version)
	return fetchPatterns(ctx, p.client, patternURL, p.config.Server.AuthToken)
}
