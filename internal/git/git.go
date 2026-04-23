package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/leaktk/leaktk/pkg/fs"
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

func GetRepoInfo(ctx context.Context, path string) (RepoInfo, error) {
	info := RepoInfo{}
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

	fields := bytes.Fields(rawInfo)
	if len(fields) != 2 {
		return info, errors.New("could not load git repo info")
	}

	// Load the field data from above
	info.GitDir = string(fields[0])
	info.IsBare = bytes.Equal(fields[1], []byte("true"))
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

func RemoveWorkingTree(ctx context.Context, repoInfo RepoInfo) error {
	// First try cleaning up the worktrees the proper way
	logger.Debug("removing temp git working tree: path=%q", repoInfo.WorkingTreePath)
	if err := RunContext(ctx, "-C", repoInfo.GitDir, "worktree", "remove", "--force", repoInfo.WorkingTreePath); err != nil {
		logger.Error("issue encountered removing temp git working tree: %v path=%q", err, repoInfo.WorkingTreePath)
	}

	// This is a fallback to make real sure we clean up as much as we can
	if fs.PathExists(repoInfo.WorkingTreePath) {
		logger.Warning("removing some worktree files manually: path=%q", repoInfo.WorkingTreePath)
		if err := os.RemoveAll(repoInfo.WorkingTreePath); err != nil {
			return fmt.Errorf("'git worktree remove' failed and could not manually remove temp git working tree: %w", err)
		}
		if err := RunContext(ctx, "-C", repoInfo.GitDir, "worktree", "prune"); err != nil {
			return fmt.Errorf("'git worktree remove' failed and could not prune manually removed temp git working tree: %w", err)
		}
	}

	return nil
}

// GetGlobalConfigPath gets a value from the global config and applies a --type=path flag
// to handle normalizing it
func GetGlobalConfigPath(ctx context.Context, name string) (string, error) {
	output, err := CommandContext(ctx, "config", "get", "--type=path", name).Output()
	if err != nil {
		err = fmt.Errorf("could not get git config value: %w name=%q", err, name)
	}
	return strings.TrimSpace(string(output)), err
}

// SetGlobalConfigPath sets a value in the global config and applies a --type=path flag
// to handle normalizing it
func SetGlobalConfigPath(ctx context.Context, name, value string) error {
	if err := RunContext(ctx, "config", "set", "--type=path", name, value); err != nil {
		return fmt.Errorf("could not set git config value: %w name=%q value=%q", err, name, value)
	}
	return nil
}
