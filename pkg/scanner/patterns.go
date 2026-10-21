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
	"strings"
	"sync"
	"time"

	blconfig "github.com/betterleaks/betterleaks/config"

	"github.com/leaktk/leaktk/internal/betterleaks"
	"github.com/leaktk/leaktk/internal/fs"
	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/logger"
)

// Patterns acts as an abstraction for fetching different scanner patterns
// and keeping them up to date and cached
type Patterns struct {
	client             *http.Client
	config             *config.Patterns
	gitleaksConfigHash [32]byte
	gitleaksConfig     *blconfig.Config
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
	var (
		err  error
		resp *http.Response
		req  *http.Request
		body []byte
	)

	logger.Info("fetching gitleaks patterns")

	patternsURL := p.config.Server.URL
	patternsURL = strings.TrimRight(patternsURL, "/")
	patternsURL = fmt.Sprintf("%s/patterns/gitleaks/%s", patternsURL, p.config.Gitleaks.Version)

	logger.Debug("patterns url: url=%q", patternsURL)

	// allow a second attempt if www-authenticate header is present
	attempt := 1
	for {
		logger.Debug("requesting patterns: attempt=%d", attempt)
		req, err = http.NewRequestWithContext(ctx, "GET", patternsURL, nil)
		if err != nil {
			return "", err
		}

		if t := p.config.Server.Auth.Token; len(t) > 0 {
			logger.Debug("setting authorization header")
			req.Header.Set("Authorization", "Bearer "+t)
		}

		resp, err = p.client.Do(req) // #nosec G704
		if err != nil {
			return "", err
		}
		body, err = io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if err != nil {
			return "", fmt.Errorf("read response: %v status_code=%d", err, resp.StatusCode)
		}

		switch resp.StatusCode {
		case http.StatusOK:
			// save the token if it was provided from a successful autologin
			if attempt == 2 {
				logger.Info("saving pattern server auth: url=%s", p.config.Server.URL)
				err := config.SavePatternServerAuth(
					p.config.Server.URL, p.config.Server.Auth.Token,
				)
				if err != nil {
					logger.Error("could not save token: %v", err)
				}
			}
			return string(body), err
		case http.StatusUnauthorized:
			// see if we can do an auto login
			h := resp.Header.Get("WWW-Authenticate")
			if p.config.Server.Auth.Autologin && len(h) != 0 {
				logger.Debug("attempting auto login")
				if authErr := p.config.Server.Auth.FromHeader(h); authErr != nil {
					logger.Debug("auth config from header: %v", authErr)
				} else if loginErr := p.config.Server.Auth.Login(ctx); loginErr != nil {
					logger.Error("auth autologin: %v", loginErr)
				} else if attempt == 1 {
					attempt++
					continue // give it another attempt
				}
			} else {
				logger.Debug(
					"not attempting auto login: autologin_enabled=%v www_auth_header_len=%d",
					p.config.Server.Auth.Autologin, len(h),
				)
			}
		}

		return "", fmt.Errorf("%s: status_code=%d", strings.ToLower(resp.Status), resp.StatusCode)
	}
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
