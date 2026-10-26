package scanner

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	blconfig "github.com/betterleaks/betterleaks/config"

	"github.com/leaktk/leaktk/internal/betterleaks"
	"github.com/leaktk/leaktk/internal/fs"
	"github.com/leaktk/leaktk/internal/sources"
	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/logger"
)

// Patterns acts as an abstraction for fetching different scanner patterns
// and keeping them up to date and cached
type Patterns struct {
	client             *http.Client
	config             *config.Patterns
	sources            sources.Sources
	gitleaksConfigHash [32]byte
	gitleaksConfig     *blconfig.Config
	mutex              sync.Mutex
}

// NewPatterns returns a configured instance of Patterns
func NewPatterns(cfg *config.Config, client *http.Client) *Patterns {
	return &Patterns{
		client:  client,
		config:  &cfg.Scanner.Patterns,
		sources: cfg.Sources,
	}
}

func (p *Patterns) fetchGitleaksConfig(ctx context.Context) (string, error) {
	var err error
	logger.Info("fetching gitleaks patterns")

	glVersion := p.config.Gitleaks.Version
	patternsURL := p.config.Server.URL.JoinPath("patterns", "gitleaks", glVersion).String()

	logger.Debug("patterns url: url=%q", patternsURL)
	doFetch := func(attempt int) (resp *http.Response, err error) {
		var req *http.Request

		logger.Debug("requesting patterns: attempt=%d", attempt)
		req, err = http.NewRequestWithContext(ctx, "GET", patternsURL, nil)
		if err != nil {
			return nil, err
		}

		if err = p.sources.SetHeader(ctx, req); err != nil {
			return nil, fmt.Errorf("set header: %w", err)
		}

		resp, err = p.client.Do(req) // #nosec G704
		if err != nil {
			return nil, err
		}
		return resp, nil

	}

	resp, err := doFetch(1)
	if err != nil {
		return "", err
	}

	switch resp.StatusCode {
	case http.StatusOK:
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			err = fmt.Errorf("read body: %w", err)
		}
		return string(body), err
	case http.StatusUnauthorized:
		if !p.config.Autologin {
			return "", fmt.Errorf(
				"request failed: status=%q status_code=%d",
				resp.Status, resp.StatusCode,
			)
		}

		logger.Debug("attempting autologin")
		if err = p.sources.Login(ctx, &p.config.Server.URL); err != nil {
			return "", fmt.Errorf("autologin: %w", err)
		}

		resp, err = doFetch(2)
		if err != nil {
			return "", fmt.Errorf("post-autologin retry: %w", err)
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf(
				"post-autologin retry: %w status=%q status_code=%d",
				err, resp.Status, resp.StatusCode,
			)
		}

		return string(body), nil
	}

	return "", fmt.Errorf("request failed: status=%q status_code=%d", resp.Status, resp.StatusCode)
}

// gitleaksConfigModTimeExceeds returns true if the file is older than
// `modTimeLimit` seconds
func (p *Patterns) gitleaksConfigModTimeExceeds(modTimeLimit int) bool {
	// When modTimeLimit is 0, expiration checking is effectively disabled
	// and gitleaksConfigModTimeExceeds returns false in this case.
	if modTimeLimit == 0 {
		return false
	}

	if fileInfo, err := os.Stat(p.config.Gitleaks.ConfigPath); err == nil {
		return int(time.Since(fileInfo.ModTime()).Seconds()) > modTimeLimit
	}

	return true
}

// Refresh refreshes all of the local patterns caches
func (p *Patterns) Refresh(ctx context.Context) error {
	return p.refreshGitleaks(ctx)
}

func (p *Patterns) parseGitleaksConfig(rawConfig string) (*blconfig.Config, error) {
	c, err := betterleaks.ParseConfig(rawConfig)

	if err != nil {
		return p.gitleaksConfig, err
	}

	excludeTags := p.config.ExcludeTags
	if len(excludeTags) > 0 {
		for id, rule := range c.Rules {
			for _, excludeTag := range excludeTags {
				if slices.Contains(rule.Tags, excludeTag) {
					logger.Debug("excluding rule by tag: rule_id=%q exclude_tag=%q", id, excludeTag)
					delete(c.Rules, id)
					break
				}
			}
		}
	}

	return c, err
}

func (p *Patterns) refreshGitleaks(ctx context.Context) error {
	rawConfig, err := p.fetchGitleaksConfig(ctx)
	if err != nil {
		return err
	}

	p.gitleaksConfig, err = p.parseGitleaksConfig(rawConfig)
	if err != nil {
		logger.Debug("fetched config:\n%s", rawConfig)
		return fmt.Errorf("could not parse config: error=%q", err)
	}

	if err := os.MkdirAll(filepath.Dir(p.config.Gitleaks.ConfigPath), 0700); err != nil {
		return fmt.Errorf("could not create config dir: error=%q", err)
	}

	// Open the config file, creating it if it doesn't already exist, but don't truncate yet
	configFile, err := os.OpenFile(p.config.Gitleaks.ConfigPath, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("could not open config file: %w path=%q", err, p.config.Gitleaks.ConfigPath)
	}

	// defer the close and add logging around it since we're adding locks
	defer func() {
		if err := configFile.Close(); err != nil {
			logger.Error("could not close config file: %v path=%q", err, p.config.Gitleaks.ConfigPath)
			if err := fs.UnlockFile(configFile); err != nil {
				logger.Error("error releasing config file lock: %v path=%q", err, p.config.Gitleaks.ConfigPath)
			}
		}
	}()

	// Establish a file lock to avoid different instances of the scanner writing to the config
	if fs.FileLockSupported {
		logger.Debug("locking config file for writes: path=%q", p.config.Gitleaks.ConfigPath)
		if err = fs.LockFile(configFile); err != nil {
			return fmt.Errorf("could not establish a file lock: %w path=%s", err, p.config.Gitleaks.ConfigPath)
		}
	}

	// Now that a lock's established if it's supported, seek to the beginning to be safe, truncate and write the file
	if _, err := configFile.Seek(0, 0); err != nil {
		return fmt.Errorf("could not seek to the beginning of the config file: %w path=%s", err, p.config.Gitleaks.ConfigPath)
	}
	if err := configFile.Truncate(0); err != nil {
		return fmt.Errorf("could not truncate existing config file: %w path=%s", err, p.config.Gitleaks.ConfigPath)
	}

	// only write the config after parsing it, that way we don't break a good
	// existing config if the server returns an invalid response
	if _, err := configFile.WriteString(rawConfig); err != nil {
		return fmt.Errorf("could not write config: path=%q error=%q", p.config.Gitleaks.ConfigPath, err)
	}

	if hash := sha256.Sum256([]byte(rawConfig)); p.gitleaksConfigHash != hash {
		p.gitleaksConfigHash = hash
		logger.Info("refreshed gitleaks patterns: hash=%s", p.GitleaksConfigHash())
	}

	return nil
}

// Gitleaks returns a Gitleaks config object if it's able to
func (p *Patterns) Gitleaks(ctx context.Context) (*blconfig.Config, error) {
	// Lock since this updates the value of p.gitleaksConfig on the fly
	// and updates files on the filesystem
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.config.Autofetch && p.gitleaksConfigModTimeExceeds(p.config.RefreshAfter) {
		err := p.refreshGitleaks(ctx)
		return p.gitleaksConfig, err
	} else if p.gitleaksConfig == nil {
		if p.gitleaksConfigModTimeExceeds(p.config.ExpiredAfter) {
			return nil, fmt.Errorf(
				"gitleaks config is expired and autofetch is disabled: config_path=%q",
				p.config.Gitleaks.ConfigPath,
			)
		}

		rawConfig, err := os.ReadFile(p.config.Gitleaks.ConfigPath)
		if err != nil {
			return p.gitleaksConfig, err
		}

		p.gitleaksConfig, err = p.parseGitleaksConfig(string(rawConfig))
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
