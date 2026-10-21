package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/adrg/xdg"

	"github.com/leaktk/leaktk/internal/auths"
	"github.com/leaktk/leaktk/internal/fs"
	"github.com/leaktk/leaktk/internal/sources"
	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/leaktk/leaktk/pkg/version"
)

// GitConfigGlobalOverride helps normalize things in the scanner
// it is a variable here so other code can check if it's been set
const GitConfigGlobalOverride = "/dev/null"
const systemConfigDir = "/etc/leaktk"

var userConfigDir = filepath.Join(xdg.ConfigHome, "leaktk")

func init() {
	// The environment variables for the scan environment
	env := map[string]string{
		"GIT_CONFIG_GLOBAL":      GitConfigGlobalOverride,
		"GIT_TERMINAL_PROMPT":    "0",
		"GIT_NO_REPLACE_OBJECTS": "1",
		"GIT_CONFIG_NOSYSTEM":    "1",
		"GIT_HTTP_USER_AGENT":    version.GlobalUserAgent,
	}

	for varname, value := range env {
		if err := os.Setenv(varname, value); err != nil {
			logger.Fatal("could not set %s=%s: %v", varname, value, err)
		}
	}

	unsetEnv := []string{
		// GIT_INDEX_FILE sets the path to the index file for non-bare
		// repositories.  It is being unset here because the commands here are
		// responsible for finding the index file themselves if needed and can
		// cause unexpected behavior for the sub-commands when used as hooks.
		"GIT_INDEX_FILE",
	}

	for _, varname := range unsetEnv {
		if err := os.Unsetenv(varname); err != nil {
			logger.Fatal("could not unset %s: %v", varname, err)
		}
	}
}

type (
	// Config provides a general structure to capture the config options
	// for the toolchain. This may be abstracted out to a common library in
	// the future as more components are added to the toolchain.
	Config struct {
		Formatter Formatter       `toml:"formatter"`
		Logger    Logger          `toml:"logger"`
		Redactor  Redactor        `toml:"Redactor"`
		Scanner   Scanner         `toml:"scanner"`
		Sources   sources.Sources `toml:"sources"`
	}

	// Formatter provides a general output format config
	Formatter struct {
		Format string `toml:"format"`
	}

	// Logger provides general logger config
	Logger struct {
		Level string `toml:"level"`
	}

	Redactor struct {
		RedactionMark string `toml:"redaction_mark"`
		RedactionWord string `toml:"redaction_word"`
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
		ExcludeTags  []string      `toml:"exclude_tags"`
	}

	// Gitleaks holds version and config information for the Betterleaks scanner
	Gitleaks struct {
		Version    string `toml:"version"`
		ConfigPath string `toml:"config_path"`
	}

	// PatternServer provides pattern server configuration settings for the scanner
	PatternServer struct {
		// Deprecated: Use "auth" field instead
		AuthToken string         `toml:"auth_token"` // #nosec G117
		Auth      auths.OIDCAuth `toml:"auth"`
		URL       string         `toml:"url"`
	}
)

// Handles merging items in lists that should be merged instead of appended to
func mergeValues(cfg *Config) {
	if err := cfg.Sources.Merge(nil); err != nil {
		logger.Fatal("merge config: sources: %v", err)
	}
}

// Make sure that any config returned to the code goes through this function
func setMissingValues(cfg *Config) {
	envLoggerLevel := os.Getenv("LEAKTK_LOGGER_LEVEL")
	if len(envLoggerLevel) > 0 {
		cfg.Logger.Level = envLoggerLevel
	}

	urlFromEnvVar := os.Getenv("LEAKTK_PATTERN_SERVER_URL")
	if len(urlFromEnvVar) != 0 {
		cfg.Scanner.Patterns.Server.URL = urlFromEnvVar
	} else if urlFromFile := loadPatternServerURL(); len(urlFromFile) != 0 {
		cfg.Scanner.Patterns.Server.URL = urlFromFile
	}

	autofetchStr, found := os.LookupEnv("LEAKTK_AUTOFETCH")
	if !found {
		// TODO: remove at v1.0.0 & update docs
		autofetchStr, found = os.LookupEnv("LEAKTK_SCANNER_AUTOFETCH")
		if found {
			logger.Warning("LEAKTK_SCANNER_AUTOFETCH is deprecated, use LEAKTK_AUTOFETCH instead")
		}
	}

	cfg.Scanner.Patterns.Autofetch = stringToBool(
		autofetchStr, cfg.Scanner.Patterns.Autofetch,
	)

	// It's better to have the auth token out of the config file to make it
	// easier to write via the login command and to minimize secrets in the
	// config file. But it's still supported in the config file in case it's
	// desirable to generate one big config file for server based deployments.
	if authToken := loadPatternServerAuthToken(); len(authToken) != 0 {
		cfg.Scanner.Patterns.Server.Auth.Token = authToken
	}

	// Handle backwards compat until "auth_token" is removed but favor the new one if set
	if len(cfg.Scanner.Patterns.Server.Auth.Token) == 0 {
		cfg.Scanner.Patterns.Server.Auth.Token = cfg.Scanner.Patterns.Server.AuthToken
	}

	cfg.Scanner.Patterns.Server.Auth.Autologin = stringToBool(
		os.Getenv("LEAKTK_AUTOLOGIN"),
		cfg.Scanner.Patterns.Server.Auth.Autologin,
	)

	if len(cfg.Scanner.Patterns.Gitleaks.ConfigPath) == 0 {
		cfg.Scanner.Patterns.Gitleaks.ConfigPath = filepath.Join(
			cfg.Scanner.Workdir, "patterns", "gitleaks",
			cfg.Scanner.Patterns.Gitleaks.Version,
		)
	}
}

func loadConfigValueFromFile(path string) string {
	path = filepath.Clean(path)
	logger.Debug("loading value: path=%q", path)

	value, err := os.ReadFile(path)
	if err != nil {
		logger.Fatal("load value: %q path=%q", err, path)
	}

	return strings.TrimSpace(string(value))
}

func patternServerAuthTokenPath(configDir string) string {
	return filepath.Join(configDir, "pattern-server-auth-token")
}

func patternServerURLPath(configDir string) string {
	return filepath.Join(configDir, "pattern-server-url")
}

func loadPatternServerURL() string {
	path := patternServerURLPath(userConfigDir)
	if fs.FileExists(path) {
		return loadConfigValueFromFile(path)
	}

	path = patternServerURLPath(nixGlobalConfigDir)
	if fs.FileExists(path) {
		return loadConfigValueFromFile(path)
	}

	return ""
}

func loadPatternServerAuthToken() string {
	authTokenFromEnvVar := os.Getenv("LEAKTK_PATTERN_SERVER_AUTH_TOKEN")

	if len(authTokenFromEnvVar) > 0 {
		logger.Debug("loading pattern-server-auth-token from env var")

		return authTokenFromEnvVar
	}

	path := patternServerAuthTokenPath(userConfigDir)
	if fs.FileExists(path) {
		return loadConfigValueFromFile(path)
	}

	path = patternServerAuthTokenPath(systemConfigDir)
	if fs.FileExists(path) {
		return loadConfigValueFromFile(path)
	}

	return ""
}

func stringToBool(value string, defaultValue bool) bool {
	if len(value) == 0 {
		return defaultValue
	}
	return value == "true" || value == "1"
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
		Redactor: Redactor{
			RedactionMark: "*",
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
				ExcludeTags: []string{
					// Don't include test rules by default
					"grp:test",
				},
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

// UpdateConfigFromFile accepts a config object and updates it with settings
// found in the file at the provided path.
func UpdateConfigFromFile(cfg *Config, path string) error {
	path = filepath.Clean(path)
	logger.Debug("loading config: path=%q", path)
	_, err := toml.DecodeFile(path, cfg)
	if err != nil {
		return err
	}

	mergeValues(cfg)
	setMissingValues(cfg)
	return nil
}

// LocateAndLoadConfig looks through the possible places for the config
// favoring the provided path if it is set
func LocateAndLoadConfig(userConfigPath string) (*Config, error) {
	var err error
	cfg := DefaultConfig()
	setMissingValues(cfg)

	// Update the default config with the system config
	if !stringToBool(os.Getenv("LEAKTK_CONFIG_NOSYSTEM"), false) {
		systemConfigPath := filepath.Join(systemConfigDir, "config.toml")
		if fs.FileExists(systemConfigPath) {
			err = UpdateConfigFromFile(cfg, systemConfigPath)
			if err != nil {
				return cfg, err
			}
		}
	}

	// Update the default+system config with user config. The userConfigPath
	// passed into the function is provided by the CLI and takes precedence. If
	// it isn't set, the env should be checked for the path; else it should
	// fall back to looking at the user's default config dir.
	if len(userConfigPath) != 0 {
		err = UpdateConfigFromFile(cfg, userConfigPath)
		return cfg, err
	}
	if userConfigPath = os.Getenv("LEAKTK_CONFIG_PATH"); len(userConfigPath) != 0 {
		err = UpdateConfigFromFile(cfg, userConfigPath)
		return cfg, err
	}
	if userConfigPath = filepath.Join(userConfigDir, "config.toml"); fs.FileExists(userConfigPath) {
		err = UpdateConfigFromFile(cfg, userConfigPath)
		return cfg, err
	}
	return cfg, nil
}

func SavePatternServerAuth(serverURL, authToken string) error {
	if !fs.PathExists(userConfigDir) {
		if err := os.MkdirAll(userConfigDir, 0700); err != nil {
			return fmt.Errorf("could not create dir: path=%q", userConfigDir)
		}
	}

	if len(serverURL) > 0 {
		urlPath := patternServerURLPath(userConfigDir)
		if err := os.WriteFile(urlPath, []byte(strings.TrimSpace(serverURL)), 0600); err != nil {
			return err
		}
	}

	tokenPath := patternServerAuthTokenPath(userConfigDir)
	if err := os.WriteFile(tokenPath, []byte(strings.TrimSpace(authToken)), 0600); err != nil {
		return err
	}

	return nil
}

func RemovePatternServerAuth() error {
	for _, path := range []string{
		patternServerAuthTokenPath(userConfigDir),
		patternServerURLPath(userConfigDir),
	} {
		if fs.FileExists(path) {
			if err := os.Remove(path); err != nil {
				return err
			}
		}
	}

	return nil
}
