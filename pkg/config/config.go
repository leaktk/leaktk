package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"

	"github.com/leaktk/leaktk/internal/auths"
	"github.com/leaktk/leaktk/internal/fs"
	"github.com/leaktk/leaktk/internal/sources"
	"github.com/leaktk/leaktk/internal/tomlutil"
	"github.com/leaktk/leaktk/internal/urlutil"
	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/leaktk/leaktk/pkg/version"
)

// GitConfigGlobalOverride helps normalize things in the scanner
// it is a variable here so other code can check if it's been set
const GitConfigGlobalOverride = "/dev/null"
const LeakTKServerEnvSourceID = "__leaktk_server_env__"
const systemConfigDir = "/etc/leaktk"

var systemConfigPath = filepath.Join(systemConfigDir, "config.toml")
var userConfigDir = filepath.Join(filepath.Clean(xdg.ConfigHome), "leaktk")
var userConfigPath = filepath.Join(userConfigDir, "config.toml")
var defaultServerURL = *urlutil.MustParseURL("https://raw.githubusercontent.com/leaktk/patterns/main/target")

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

	if path := os.Getenv("LEAKTK_CONFIG_PATH"); len(path) != 0 {
		if err := SetUserConfigPath(path); err != nil {
			logger.Fatal("set user config path from LEAKTK_CONFIG_PATH: %v", err)
		}
	}

}

type (
	Config struct {
		Formatter Formatter
		Logger    Logger
		Redactor  Redactor
		Scanner   Scanner
		Sources   sources.Sources
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
		Autofetch    bool     `toml:"autofetch"`
		Autologin    bool     `toml:"autologin"`
		ExpiredAfter int      `toml:"expired_after"`
		Gitleaks     Gitleaks `toml:"gitleaks"`
		RefreshAfter int      `toml:"refresh_after"`
		Server       Server   `toml:"server"`
		ExcludeTags  []string `toml:"exclude_tags"`
	}

	// Gitleaks holds version and config information for the Betterleaks scanner
	Gitleaks struct {
		Version    string `toml:"version"`
		ConfigPath string `toml:"config_path"`
	}

	// Server provides LeakTK server configuration settings for the scanner
	Server struct {
		URL urlutil.URL `toml:"url"`
	}
)

func (s *Server) UnmarshalTOML(d any) error {
	m, ok := d.(map[string]any)
	if !ok {
		return fmt.Errorf("could not load data: expected=\"map[string]any\" got=%T", d)
	}

	for k, v := range m {
		switch k {
		case "url":
			rawURL, ok := v.(string)
			if !ok {
				return fmt.Errorf("%s must be a string", k)
			}
			err := s.URL.UnmarshalText([]byte(rawURL))
			if err != nil {
				return fmt.Errorf("invalid %s: %w base_url=%q", k, err, rawURL)
			}
		}
	}

	return nil

}

// UserConfigPath returns the current user configured config path
func UserConfigPath() string {
	return userConfigPath
}

// SetUserConfigPath updates the default location for user config during the scanner's run
func SetUserConfigPath(path string) error {
	if !fs.FileExists(path) {
		return fmt.Errorf("file does not exist: path=%q", path)
	}
	userConfigPath = path
	return nil
}

// SystemConfigPath returns the path to the system wide config
func SystemConfigPath() string {
	return systemConfigPath
}

func SaveLeakTKServerSource(s *sources.LeakTKServer) error {
	return nil
}

// updateSourcesFromEnv configures cfg.Sources from applicable env variables.
// For example if LEAKTK_SERVER_URL is set but it's not in the config
func updateSourcesFromEnv(cfg *Config) error {
	rawBaseURL := ""
	accessToken := ""

	// Load the base URL from the env
	err := loadValueSearch(
		&rawBaseURL,
		// preferred variable
		valueEnvVar("LEAKTK_SERVER_BASE_URL"),
		// legacy value sources to remove in 1.0.0
		valueEnvVar("LEAKTK_PATTERN_SERVER_URL"),
		valuePath(serverURLPath(userConfigDir)),
		valuePath(serverURLPath(systemConfigDir)),
		// default fallback value
		valueLiteral(cfg.Scanner.Patterns.Server.URL.String()),
	)
	if err != nil {
		return fmt.Errorf("load base url: %w", err)
	}
	baseURl, err := urlutil.ParseURL(rawBaseURL)
	if err != nil {
		return fmt.Errorf("parse base url: %w", err)
	}

	// Ensure the current configured server URL is set
	cfg.Scanner.Patterns.Server.URL = *baseURl

	// Load the access token from the env
	err = loadValueSearch(
		&accessToken,
		// preferred variable
		valueEnvVar("LEAKTK_SERVER_ACCESS_TOKEN"),
		// legacy value sources to remove in 1.0.0
		valueEnvVar("LEAKTK_PATTERN_SERVER_AUTH_TOKEN"),
		valuePath(serverAuthTokenPath(userConfigDir)),
		valuePath(serverAuthTokenPath(systemConfigDir)),
	)
	if err != nil {
		return fmt.Errorf("load access token: %w", err)
	}

	// Create a prototype source for patching existing sources or defining a
	// new one if they dont' exist
	protoSrc := &sources.LeakTKServer{
		OIDCAuth: auths.OIDCAuth{
			AccessToken:  accessToken,
			RefreshToken: os.Getenv("LEAKTK_SERVER_REFRESH_TOKEN"),
		},
	}
	if err := protoSrc.BaseURL.UnmarshalText([]byte(rawBaseURL)); err != nil {
		return fmt.Errorf("unmarshal base url: %w", err)
	}

	// Update any applicable already configured sources with the prototype
	// source
	updated := false
	for _, s := range cfg.Sources.Iter() {
		switch s := s.(type) {
		case *sources.LeakTKServer:
			if s.AppliesTo(baseURl) {
				if err := s.Merge(protoSrc); err != nil {
					return fmt.Errorf("apply LeakTK server config from env: %w", err)
				}

				// set the save callback to save login creds
				s.Save = SaveLeakTKServerSource
				updated = true
			}
		}
	}

	// If there wasn't any existing config, use the prototype source as the
	// server auth
	if !updated {
		// Create a source with a unique ID to use for config from the env
		id := LeakTKServerEnvSourceID
		envSrc, err := sources.NewSourceFromKindID(sources.LeakTKServerKind, id)
		if err != nil {
			return fmt.Errorf("new source from kind: %w", err)
		}

		// set the save callback to save login creds
		envSrc.(*sources.LeakTKServer).Save = SaveLeakTKServerSource

		// Update it with the values from the prototype source
		if err := envSrc.Merge(protoSrc); err != nil {
			return fmt.Errorf("apply LeakTK server config from env: %w", err)
		}

		// Add the env source to the soruces config
		if err := cfg.Sources.Add(envSrc); err != nil {
			return fmt.Errorf("apply LeakTK server config from env: %w", err)
		}
	}

	return nil
}

// Make sure that any config returned to the code goes through this function
func setMissingValues(cfg *Config) {
	envLoggerLevel := os.Getenv("LEAKTK_LOGGER_LEVEL")
	if len(envLoggerLevel) > 0 {
		cfg.Logger.Level = envLoggerLevel
	}

	// Determines if LeakTK can fetch auto fetch patterns when current are stale
	autofetchStr := ""
	err := loadValueSearch(
		&autofetchStr,
		// preferred variable
		valueEnvVar("LEAKTK_AUTOFETCH"),
		// legacy value sources to remove in 1.0.0
		valueEnvVar("LEAKTK_SCANNER_AUTOFETCH"),
	)
	if err != nil {
		logger.Fatal("load autofetch value from env: %w", err)
	}
	cfg.Scanner.Patterns.Autofetch = stringToBool(autofetchStr, cfg.Scanner.Patterns.Autofetch)

	// Determines if LeakTK can kick off a --web login flow for servers that support it
	cfg.Scanner.Patterns.Autologin = stringToBool(os.Getenv("LEAKTK_AUTOLOGIN"), cfg.Scanner.Patterns.Autologin)

	if len(cfg.Scanner.Patterns.Gitleaks.ConfigPath) == 0 {
		cfg.Scanner.Patterns.Gitleaks.ConfigPath = filepath.Join(
			cfg.Scanner.Workdir, "patterns", "gitleaks",
			cfg.Scanner.Patterns.Gitleaks.Version,
		)
	}

	// Override any source values that need to be overridden by the env
	if err := updateSourcesFromEnv(cfg); err != nil {
		logger.Fatal("update LeakTK server source from env: %w", err)
	}
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
				Server: Server{
					URL: defaultServerURL,
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
	rawCfg, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file: %w path=%q", err, path)
	}

	err = tomlutil.Unmarshal(rawCfg, &cfg)
	if err != nil {
		return fmt.Errorf("update config: %w path=%q", err, path)
	}

	setMissingValues(cfg)
	return nil
}

// Load loads config across the different scopes, merges them together
// and sets defaults
func Load() (cfg *Config, err error) {
	usrCfgPath := UserConfigPath()
	sysCfgPath := SystemConfigPath()

	// Start with default config
	cfg = DefaultConfig()

	// Apply system config
	if !stringToBool(os.Getenv("LEAKTK_CONFIG_NOSYSTEM"), false) {
		if fs.FileExists(sysCfgPath) {
			if err = UpdateConfigFromFile(cfg, sysCfgPath); err != nil {
				return cfg, err
			}
		}
	}

	// Apply user config
	if fs.FileExists(usrCfgPath) {
		if err = UpdateConfigFromFile(cfg, usrCfgPath); err != nil {
			return cfg, err
		}
	}

	return cfg, err
}
