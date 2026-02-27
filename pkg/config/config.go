package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/adrg/xdg"

	"github.com/leaktk/leaktk/pkg/fs"
	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/leaktk/leaktk/pkg/version"
)

const nixGlobalConfigDir = "/etc/leaktk"

var localConfigDir string

func init() {
	localConfigDir = filepath.Join(xdg.ConfigHome, "leaktk")

	// The environment variables for the scan environment
	env := map[string]string{
		"GIT_CONFIG_GLOBAL":      "/dev/null",
		"GIT_TERMINAL_PROMPT":    "0",
		"GIT_NO_REPLACE_OBJECTS": "1",
		"GIT_CONFIG_NOSYSTEM":    "1",
		"GIT_HTTP_USER_AGENT":    version.GlobalUserAgent,
	}

	for key, value := range env {
		if err := os.Setenv(key, value); err != nil {
			logger.Error("could not set %s=%s: %v", key, value, err)
		}
	}
}

type (
	// Config provides a general structure to capture the config options
	// for the toolchain. This may be abstracted out to a common library in
	// the future as more components are added to the toolchain.
	Config struct {
		Logger    Logger    `toml:"logger"`
		Scanner   Scanner   `toml:"scanner"`
		Formatter Formatter `toml:"formatter"`
	}

	// Formatter provides a general output format config
	Formatter struct {
		Format string `toml:"format"`
	}

	// Logger provides general logger config
	Logger struct {
		Level string `toml:"level"`
	}

	// Scanner provides scanner specific config
	Scanner struct {
		AllowLocal           bool     `toml:"allow_local"`
		ScanTimeout          int      `toml:"scan_timeout"`
		MaxArchiveDepth      int      `toml:"max_archive_depth"`
		MaxDecodeDepth       int      `toml:"max_decode_depth"`
		MaxScanDepth         int      `toml:"max_scan_depth"`
		MaxScanQueueSize     int      `toml:"max_scan_queue_size"`
		MaxResponseQueueSize int      `toml:"max_response_queue_size"`
		Patterns             Patterns `toml:"patterns"`
		ScanWorkers          int      `toml:"scan_workers"`
		Workdir              string   `toml:"workdir"`
	}

	// Patterns provides configuration for managing pattern updates
	Patterns struct {
		Autofetch    bool          `toml:"autofetch"`
		ExpiredAfter int           `toml:"expired_after"`
		Gitleaks     Gitleaks      `toml:"gitleaks"`
		RefreshAfter int           `toml:"refresh_after"`
		Server       PatternServer `toml:"server"`
	}

	// Gitleaks holds version and config information for the Betterleaks scanner
	Gitleaks struct {
		Version    string `toml:"version"`
		ConfigPath string `toml:"config_path"`
	}

	// PatternServer provides pattern server configuration settings for the scanner
	PatternServer struct {
		AuthToken string `toml:"auth_token"`
		URL       string `toml:"url"`
	}
)

// Make sure that any config returned to the code goes through this function
func setMissingValues(cfg *Config) *Config {
	envLoggerLevel := os.Getenv("LEAKTK_LOGGER_LEVEL")
	if len(envLoggerLevel) > 0 {
		cfg.Logger.Level = envLoggerLevel
	}

	urlFromEnvVar := os.Getenv("LEAKTK_PATTERN_SERVER_URL")
	if len(urlFromEnvVar) != 0 {
		cfg.Scanner.Patterns.Server.URL = urlFromEnvVar
	}

	cfg.Scanner.Patterns.Autofetch = stringToBool(
		os.Getenv("LEAKTK_SCANNER_AUTOFETCH"),
		cfg.Scanner.Patterns.Autofetch,
	)

	// It's better to have the auth token out of the config file to make it
	// easier to write via the login command and to minimize secrets in the
	// config file. But it's still supported in the config file in case it's
	// desirable to generate one big config file for server based deployments.
	if authToken := loadPatternServerAuthToken(); len(authToken) != 0 {
		cfg.Scanner.Patterns.Server.AuthToken = authToken
	}

	if len(cfg.Scanner.Patterns.Gitleaks.ConfigPath) == 0 {
		cfg.Scanner.Patterns.Gitleaks.ConfigPath = filepath.Join(
			cfg.Scanner.Workdir, "patterns", "gitleaks",
			cfg.Scanner.Patterns.Gitleaks.Version,
		)
	}

	return cfg
}

func loadPatternServerAuthTokenFromFile(path string) string {
	path = filepath.Clean(path)
	logger.Debug("loading pattern-server-auth-token: path=%q", path)
	authTokenBytes, err := os.ReadFile(path)

	if err != nil {
		logger.Fatal("could not load pattern server auth token: error=%q", err)
	}

	return strings.TrimSpace(string(authTokenBytes))
}

func patternServerAuthTokenPath(configDir string) string {
	return filepath.Join(configDir, "pattern-server-auth-token")
}

func loadPatternServerAuthToken() string {
	authTokenFromEnvVar := os.Getenv("LEAKTK_PATTERN_SERVER_AUTH_TOKEN")

	if len(authTokenFromEnvVar) > 0 {
		logger.Debug("loading pattern-server-auth-token from env var")

		return authTokenFromEnvVar
	}

	path := patternServerAuthTokenPath(localConfigDir)
	if fs.FileExists(path) {
		return loadPatternServerAuthTokenFromFile(path)
	}

	path = patternServerAuthTokenPath(nixGlobalConfigDir)
	if fs.FileExists(path) {
		return loadPatternServerAuthTokenFromFile(path)
	}

	return ""
}

func stringToBool(value string, defaultValue bool) bool {
	if len(value) == 0 {
		return defaultValue
	}

	if value == "true" || value == "1" {
		return true
	}

	return false
}

// DefaultConfig provides a fully usable instance of Config with default
// values provided
func DefaultConfig() *Config {
	return &Config{
		Formatter: Formatter{
			Format: "JSON",
		},
		Logger: Logger{
			Level: "INFO",
		},
		Scanner: Scanner{
			AllowLocal:      true,
			ScanTimeout:     0,
			MaxScanDepth:    0,
			ScanWorkers:     1,
			Workdir:         filepath.Join(xdg.CacheHome, "leaktk", "scanner"),
			MaxArchiveDepth: 8,
			MaxDecodeDepth:  8,
			Patterns: Patterns{
				Autofetch:    true,
				ExpiredAfter: 60 * 60 * 12 * 14, // 7 days
				RefreshAfter: 60 * 60 * 12,      // 12 hours
				Gitleaks: Gitleaks{
					Version: "8.27.0",
				},
				Server: PatternServer{
					URL: "https://raw.githubusercontent.com/leaktk/patterns/main/target",
				},
			},
		},
	}
}

// LoadConfigFromFile provides a config object with default values set plus any
// custom values pulled in from the config file
func LoadConfigFromFile(path string) (*Config, error) {
	path = filepath.Clean(path)
	logger.Debug("loading config: path=%q", path)
	cfg := DefaultConfig()
	_, err := toml.DecodeFile(path, cfg)

	if err != nil {
		return nil, err
	}

	return setMissingValues(cfg), err
}

// LocateAndLoadConfig looks through the possible places for the config
// favoring the provided path if it is set
func LocateAndLoadConfig(path string) (*Config, error) {
	if len(path) > 0 {
		return LoadConfigFromFile(path)
	}

	if path = os.Getenv("LEAKTK_CONFIG_PATH"); len(path) > 0 {
		return LoadConfigFromFile(path)
	}

	if len(path) > 0 {
		logger.Debug("loading config: path=%q", path)
	} else {
		logger.Debug("using default config")
	}

	path = filepath.Join(localConfigDir, "config.toml")
	if fs.FileExists(path) {
		return LoadConfigFromFile(path)
	}

	path = filepath.Join(nixGlobalConfigDir, "config.toml")
	if fs.FileExists(path) {
		return LoadConfigFromFile(path)
	}

	return setMissingValues(DefaultConfig()), nil
}

// SavePatternServerAuthToken saves the token in the path where it should go
func SavePatternServerAuthToken(authToken string) error {
	if !fs.PathExists(localConfigDir) {
		if err := os.MkdirAll(localConfigDir, 0700); err != nil {
			return fmt.Errorf("could not create dir: path=%q", localConfigDir)
		}
	}

	path := patternServerAuthTokenPath(localConfigDir)
	if err := os.WriteFile(path, []byte(strings.TrimSpace(authToken)), 0600); err != nil {
		return err
	}

	return nil
}

// RemovePatternServerAuthToken deletes the auth token
func RemovePatternServerAuthToken() error {
	path := patternServerAuthTokenPath(localConfigDir)

	if fs.FileExists(path) {
		if err := os.Remove(path); err != nil {
			return err
		}
	}

	return nil
}
