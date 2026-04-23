package installer

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/fs"
)

// setupGitRepo creates a git repository in the given directory
// It disables git template directory to avoid using the host's templates
func setupGitRepo(t *testing.T, repoPath string, bare bool) {
	t.Helper()

	err := os.MkdirAll(repoPath, 0755) // #nosec G301 -- test directory permissions
	require.NoError(t, err)

	ctx := context.Background()
	var cmd *exec.Cmd
	if bare {
		cmd = exec.CommandContext(ctx, "git", "-c", "init.templateDir=", "init", "--bare", repoPath) // #nosec G204 -- test code with safe inputs
	} else {
		cmd = exec.CommandContext(ctx, "git", "-c", "init.templateDir=", "init", repoPath) // #nosec G204 -- test code with safe inputs
	}

	err = cmd.Run()
	require.NoError(t, err)
}

func TestGitFindAbsDir(t *testing.T) {
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "test-repo")

	// Create a test git repository
	setupGitRepo(t, repoPath, false)

	// Test finding the absolute git directory
	ctx := context.Background()
	absDir, err := gitFindAbsDir(ctx, repoPath)
	require.NoError(t, err)

	expectedGitDir := filepath.Join(repoPath, ".git")
	assert.Equal(t, expectedGitDir, absDir)

	// Test with bare repository
	bareRepoPath := filepath.Join(tmpDir, "bare-repo.git")
	setupGitRepo(t, bareRepoPath, true)

	absDir, err = gitFindAbsDir(ctx, bareRepoPath)
	require.NoError(t, err)
	assert.Equal(t, bareRepoPath, absDir)
}

func TestGitInstallHookDir(t *testing.T) {
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")

	// Create .git directory structure
	err := os.MkdirAll(gitDir, 0755) // #nosec G301 -- test directory permissions
	require.NoError(t, err)

	// Test installing a hook
	hookname := "git.pre-commit"
	err = gitInstallHookDir(hookname, gitDir)
	require.NoError(t, err)

	// Verify the hook was created
	hookPath := filepath.Join(gitDir, "hooks", "pre-commit")
	assert.True(t, fs.FileExists(hookPath))

	// Verify the hook is executable
	info, err := os.Stat(hookPath)
	require.NoError(t, err)

	assert.NotEqual(t, 0, info.Mode().Perm()&0111, "hook file should be executable")
	assert.Equal(t, os.FileMode(0750), info.Mode().Perm())

	// Read and verify hook content
	content, err := os.ReadFile(hookPath) // #nosec G304 -- test code reading test file
	require.NoError(t, err)

	contentStr := string(content)
	// Frontmatter must have a space after the #
	assert.True(t, strings.HasPrefix(contentStr, "#!/bin/sh\n# TemplateID:"), "script must start with shebang and spaced frontmatter")
	// Must contain the command-v guard
	assert.Contains(t, contentStr, "if command -v leaktk > /dev/null 2>&1")
	assert.Contains(t, contentStr, "exec leaktk hook git.pre-commit")
	// Must contain the error doc link
	assert.Contains(t, contentStr, "docs/errors/command_not_found")
	// Must NOT exec unconditionally (the old broken form)
	assert.NotContains(t, contentStr, "\nexec leaktk")
}

func TestGitInstallHookDirInvalidName(t *testing.T) {
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")

	err := os.MkdirAll(gitDir, 0755) // #nosec G301 -- test directory permissions
	require.NoError(t, err)

	// Test with invalid hook name (no dot separator)
	hookName := "invalid-hook-name"
	err = gitInstallHookDir(hookName, gitDir)
	require.Error(t, err)
}

func TestFindGitDirs(t *testing.T) {
	tmpDir := t.TempDir()

	// Regular repo
	repo1 := filepath.Join(tmpDir, "repo1")
	setupGitRepo(t, repo1, false)

	// Bare repo
	repo2 := filepath.Join(tmpDir, "repo2.git")
	setupGitRepo(t, repo2, true)

	// Nested repo (under a subdirectory)
	repo3 := filepath.Join(tmpDir, "nested", "repo3")
	setupGitRepo(t, repo3, false)

	ctx := context.Background()

	t.Run("finds all repos recursively", func(t *testing.T) {
		gitDirs, err := findGitDirs(ctx, tmpDir)
		require.NoError(t, err)
		assert.Len(t, gitDirs, 3)
	})

	t.Run("finds single repo when pointed at it directly", func(t *testing.T) {
		gitDirs, err := findGitDirs(ctx, repo1)
		require.NoError(t, err)
		assert.Len(t, gitDirs, 1)
		assert.Equal(t, filepath.Join(repo1, ".git"), gitDirs[0])
	})

	t.Run("finds bare repo", func(t *testing.T) {
		gitDirs, err := findGitDirs(ctx, repo2)
		require.NoError(t, err)
		assert.Len(t, gitDirs, 1)
		assert.Equal(t, repo2, gitDirs[0])
	})

	t.Run("non-git directory returns empty", func(t *testing.T) {
		emptyDir := filepath.Join(tmpDir, "notarepo")
		require.NoError(t, os.MkdirAll(emptyDir, 0755)) // #nosec G301
		gitDirs, err := findGitDirs(ctx, emptyDir)
		require.NoError(t, err)
		assert.Empty(t, gitDirs)
	})
}

func TestGitHookInstallUserTemplateDir(t *testing.T) {
	// Isolate git config so we don't touch the real ~/.gitconfig
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(tmpHome, ".gitconfig"))
	// Redirect XDG so xdg.ConfigHome picks up our temp dir
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpHome, ".config"))

	cfg := &config.Config{}
	ctx := context.Background()

	t.Run("creates and configures templateDir when not set", func(t *testing.T) {
		templateDir, err := gitUserTemplateDir(ctx)
		require.NoError(t, err)
		assert.True(t, fs.DirExists(templateDir))

		// git config --global init.templateDir should now be set
		cmd := exec.CommandContext(ctx, "git", "config", "--global", "init.templateDir") // #nosec G204
		output, err := cmd.Output()
		require.NoError(t, err)
		assert.Equal(t, templateDir, strings.TrimSpace(string(output)))
	})

	t.Run("installs hook into templateDir", func(t *testing.T) {
		opts := GitHookOpts{
			Name:            "git.pre-commit",
			UserTemplateDir: true,
			Force:           false,
		}
		err := GitHookInstall(cfg, opts)
		require.NoError(t, err)

		templateDir, err := gitUserTemplateDir(ctx)
		require.NoError(t, err)
		hookPath := filepath.Join(templateDir, "hooks", "pre-commit")
		assert.True(t, fs.IsExecutable(hookPath))
	})
}

func TestGitHookInstall(t *testing.T) {
	tmpDir := t.TempDir()

	repo1 := filepath.Join(tmpDir, "repo1")
	setupGitRepo(t, repo1, false)

	repo2 := filepath.Join(tmpDir, "subdir", "repo2")
	setupGitRepo(t, repo2, false)

	cfg := &config.Config{}

	t.Run("install in single repo", func(t *testing.T) {
		opts := GitHookOpts{
			Name:  "git.pre-commit",
			Path:  repo1,
			Force: false,
		}
		err := GitHookInstall(cfg, opts)
		require.NoError(t, err)
		hookPath := filepath.Join(repo1, ".git", "hooks", "pre-commit")
		assert.True(t, fs.IsExecutable(hookPath))
	})

	t.Run("installs in all repos under path when recursive is set", func(t *testing.T) {
		opts := GitHookOpts{
			Name:      "git.pre-commit",
			Force:     true,
			Path:      tmpDir,
			Recursive: true,
		}
		err := GitHookInstall(cfg, opts)
		require.NoError(t, err)
		hook1 := filepath.Join(repo1, ".git", "hooks", "pre-commit")
		hook2 := filepath.Join(repo2, ".git", "hooks", "pre-commit")
		assert.True(t, fs.IsExecutable(hook1))
		assert.True(t, fs.IsExecutable(hook2))
	})

	t.Run("skips existing hook when force=false", func(t *testing.T) {
		opts := GitHookOpts{Name: "git.pre-commit", Path: repo1, Force: false}
		require.NoError(t, GitHookInstall(cfg, opts))
		info, err := os.Stat(filepath.Join(repo1, ".git", "hooks", "pre-commit"))
		require.NoError(t, err)
		originalMtime := info.ModTime()
		// install again — should skip
		require.NoError(t, GitHookInstall(cfg, opts))
		info2, err := os.Stat(filepath.Join(repo1, ".git", "hooks", "pre-commit"))
		require.NoError(t, err)
		assert.Equal(t, originalMtime, info2.ModTime(), "file should not have been overwritten")
	})

	t.Run("overwrites existing hook when force=true", func(t *testing.T) {
		opts := GitHookOpts{Name: "git.pre-commit", Path: repo1, Force: true}
		require.NoError(t, GitHookInstall(cfg, opts))
		info, err := os.Stat(filepath.Join(repo1, ".git", "hooks", "pre-commit"))
		require.NoError(t, err)
		originalMtime := info.ModTime()
		time.Sleep(10 * time.Millisecond)
		require.NoError(t, GitHookInstall(cfg, opts))
		info2, err := os.Stat(filepath.Join(repo1, ".git", "hooks", "pre-commit"))
		require.NoError(t, err)
		assert.NotEqual(t, originalMtime, info2.ModTime(), "file should have been overwritten")
	})

	t.Run("returns error for nonexistent path", func(t *testing.T) {
		opts := GitHookOpts{
			Name: "git.pre-commit",
			Path: filepath.Join(tmpDir, "nonexistent"),
		}
		err := GitHookInstall(cfg, opts)
		require.Error(t, err)
	})
}
