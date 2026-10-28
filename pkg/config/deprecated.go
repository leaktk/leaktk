package config

import (
	"path/filepath"
)

// These are config items that will be removed in version 1.0.0

func serverAuthTokenPath(configDir string) string {
	return filepath.Join(filepath.Clean(configDir), "pattern-server-auth-token")
}

func serverURLPath(configDir string) string {
	return filepath.Join(filepath.Clean(configDir), "pattern-server-url")
}
