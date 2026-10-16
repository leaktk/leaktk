package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/leaktk/leaktk/pkg/logger"
)

// GitRepoInfo is a collection of facts about a repo being scanned.
// See `man 7 gitglossary` for more information about the terms.
type RepoInfo struct {
	// Whether or not the repo is a bare repo
	IsBare bool
	// The path to the actual GIT_DIR folder
	GitDir string
	// The working tree for the repo (a temp one is created for bare repos)
	WorkingTreePath string
}

func withHostConfig(f func()) {
	unsetEnv := []string{
		"GIT_CONFIG_NOSYSTEM",
		"GIT_CONFIG_GLOBAL",
	}

	for _, name := range unsetEnv {
		// Ensure it's set back at the end
		defer func(value string) {
			if err := os.Setenv(name, value); err != nil {
				logger.Fatal("could not set env var: %v name=%q value=%q", err, name, value)
			}
		}(os.Getenv(name))

		// Unset the variable
		if err := os.Unsetenv(name); err != nil {
			logger.Fatal("could not unset env var: %v name=%q", err, name)
		}
	}

	// Call the function that needs the host config
	f()
}

func GetRepoInfo(ctx context.Context, path string) (RepoInfo, error) {
	info := RepoInfo{WorkingTreePath: path}
	cmd := CommandContext(
		ctx,
		"-C",
		path,
		"rev-parse",
		// The order of these flags affects the field order below
		"--absolute-git-dir",
		"--is-bare-repository",
	) // #nosec G204

	logger.Debug("executing: %s", cmd)
	rawInfo, err := cmd.Output()
	if err != nil {
		return info, err
	}

	fields := bytes.Split(bytes.TrimSpace(rawInfo), []byte("\n"))
	if len(fields) != 2 {
		return info, errors.New("could not load git repo info")
	}

	// Load the field data from above
	info.GitDir = string(fields[0])
	info.IsBare = bytes.Equal(fields[1], []byte("true"))

	// Resolve the working tree to the toplevel path
	if !info.IsBare {
		// Running this separate since it's more prone to error out
		cmd := CommandContext(
			ctx,
			"-C",
			info.WorkingTreePath,
			"rev-parse",
			"--show-toplevel",
		) // #nosec G204
		logger.Debug("executing: %s", cmd)
		rawTopLevel, err := cmd.Output()
		if err == nil {
			info.WorkingTreePath = string(bytes.TrimSpace(rawTopLevel))
			logger.Debug("setting working tree to toplevel dir: path=%q", info.WorkingTreePath)
		} else {
			logger.Debug("unable to set working tree: %v", err)
		}
	}

	return info, nil
}

func RunContext(ctx context.Context, args ...string) error {
	cmd := CommandContext(ctx, args...)
	logger.Debug("executing: %s", cmd)
	return cmd.Run()
}

// RemoteRefExists checks if the provided ref exists on the remote repo
func RemoteRefExists(ctx context.Context, repository, ref string) bool {
	return RunContext(ctx, "ls-remote", "--exit-code", "--quiet", repository, ref) == nil
}

// GetGlobalConfigPath gets a value from the global config and applies a --type=path flag
// to handle normalizing it
func GetGlobalConfigPath(ctx context.Context, name string) (configPath string) {
	withHostConfig(func() {
		logger.Debug("getting global config value: name=%q", name)
		cmd := CommandContext(ctx, "config", "--global", "--type=path", name)
		logger.Debug("executing: %s", cmd)
		output, err := cmd.Output()
		if err != nil {
			logger.Debug("existing value not found: %v name=%q", err, name)
			return
		}
		configPath = strings.TrimSpace(string(output))
	})
	return
}

// SetGlobalConfigPath sets a value in the global config and applies a --type=path flag
// to handle normalizing it
func SetGlobalConfigPath(ctx context.Context, name, value string) (err error) {
	withHostConfig(func() {
		logger.Debug("setting global config value: name=%q value=%q", name, value)
		if err = RunContext(ctx, "config", "--global", "--type=path", name, value); err != nil {
			err = fmt.Errorf("could not set git config value: %w name=%q value=%q", err, name, value)
		}
	})
	return
}

func SafeDirectories(ctx context.Context) []string {
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		logger.Debug("using / as user home dir: %v", err)
		userHomeDir = "/"
	}

	cmd := CommandContext(ctx, "config", "get", "--all", "safe.directory")
	cmd.Dir = userHomeDir // Make sure not to pick up repo config

	logger.Debug("executing: %s", cmd)
	output, err := cmd.Output()
	if err != nil {
		logger.Debug("could not look up safe.directory: %v", err)
		return nil
	}

	var safeDirs []string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if len(line) > 0 {
			safeDirs = append(safeDirs, line)
		}
	}

	return safeDirs
}
