package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leaktk/leaktk/internal/git"
	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/fs"
	"github.com/leaktk/leaktk/pkg/hooks"
)

// setupGitRepo creates a git repository in the given directory
// It disables git template directory to avoid using the host's templates
func setupGitRepo(t *testing.T, repoPath string, bare bool) {
	ctx := t.Context()

	if bare {
		require.NoError(t, git.RunContext(ctx, "-c", "init.templateDir=", "init", "--bare", repoPath))
	} else {
		require.NoError(t, git.RunContext(ctx, "-c", "init.templateDir=", "init", repoPath))
	}
}

func cleanPath(t *testing.T, path string) string {
	var err error
	path, err = filepath.Abs(path)
	require.NoError(t, err)
	path, err = filepath.EvalSymlinks(path)
	require.NoError(t, err)
	return path
}

func TestGitInstallHook(t *testing.T) {
	t.Run("gitCreateHookScript", func(t *testing.T) {
		tempDir := t.TempDir()
		setupGitRepo(t, tempDir, false)

		// Test installing a hook
		hook := hooks.GitPreCommitHook
		require.NoError(t, gitHookInstall(hook, tempDir, false, 0750))

		// Verify the hook was created
		hookPath := filepath.Join(tempDir, "hooks", "pre-commit")
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
	})

	t.Run("findGitDirs", func(t *testing.T) {
		tempDir := t.TempDir()
		ctx := t.Context()

		// Regular repo
		repo1Dir := filepath.Join(tempDir, "repo1")
		setupGitRepo(t, repo1Dir, false)

		// Bare repo
		repo2Dir := filepath.Join(tempDir, "repo2.git")
		setupGitRepo(t, repo2Dir, true)

		// Nested repo (under a subdirectory)
		repo3Dir := filepath.Join(tempDir, "nested", "repo3")
		setupGitRepo(t, repo3Dir, false)

		t.Run("finds all recursively", func(t *testing.T) {
			gitDirs, err := findGitDirs(ctx, tempDir)
			require.NoError(t, err)
			assert.Len(t, gitDirs, 3)
		})

		t.Run("finds single repo when pointed at it directly", func(t *testing.T) {
			gitDirs, err := findGitDirs(ctx, repo1Dir)
			require.NoError(t, err)
			assert.Len(t, gitDirs, 1)
			assert.Equal(t, cleanPath(t, filepath.Join(repo1Dir, ".git")), cleanPath(t, gitDirs[0]))
		})

		t.Run("finds bare repo", func(t *testing.T) {
			gitDirs, err := findGitDirs(ctx, repo2Dir)
			require.NoError(t, err)
			assert.Len(t, gitDirs, 1)
			assert.Equal(t, cleanPath(t, repo2Dir), cleanPath(t, gitDirs[0]))
		})

		t.Run("non-git directory returns empty", func(t *testing.T) {
			emptyDir := filepath.Join(tempDir, "notarepo")
			require.NoError(t, os.MkdirAll(emptyDir, 0700))
			gitDirs, err := findGitDirs(ctx, emptyDir)
			require.NoError(t, err)
			assert.Empty(t, gitDirs)
		})
	})

	t.Run("install in single repo", func(t *testing.T) {
		tempDir := t.TempDir()
		setupGitRepo(t, tempDir, false)

		cfg := &config.Config{}
		err := GitHookInstall(t.Context(), cfg, GitHookOpts{
			Hook:  hooks.GitPreCommitHook,
			Path:  tempDir,
			Force: false,
		})
		require.NoError(t, err)
		hookPath := filepath.Join(tempDir, ".git", "hooks", "pre-commit")
		assert.True(t, fs.IsExecutable(hookPath))
	})

	t.Run("installs in all repos under path when recursive is set", func(t *testing.T) {
		tempDir := t.TempDir()
		repo1Dir := filepath.Join(tempDir, "repo1")
		repo2Dir := filepath.Join(tempDir, "repo2")

		setupGitRepo(t, repo1Dir, false)
		setupGitRepo(t, repo2Dir, false)

		cfg := &config.Config{}
		err := GitHookInstall(t.Context(), cfg, GitHookOpts{
			Hook:      hooks.GitPreCommitHook,
			Force:     true,
			Path:      tempDir,
			Recursive: true,
		})
		require.NoError(t, err)
		hook1 := filepath.Join(repo1Dir, ".git", "hooks", "pre-commit")
		hook2 := filepath.Join(repo2Dir, ".git", "hooks", "pre-commit")
		assert.True(t, fs.IsExecutable(hook1))
		assert.True(t, fs.IsExecutable(hook2))
	})

	t.Run("skips existing hook when force=false", func(t *testing.T) {
		tempDir := t.TempDir()
		setupGitRepo(t, tempDir, false)

		cfg := &config.Config{}
		opts := GitHookOpts{Hook: hooks.GitPreCommitHook, Path: tempDir, Force: false}
		require.NoError(t, GitHookInstall(t.Context(), cfg, opts))
		info, err := os.Stat(filepath.Join(tempDir, ".git", "hooks", "pre-commit"))
		require.NoError(t, err)
		originalMtime := info.ModTime()
		// install again — should skip
		require.NoError(t, GitHookInstall(t.Context(), cfg, opts))
		info2, err := os.Stat(filepath.Join(tempDir, ".git", "hooks", "pre-commit"))
		require.NoError(t, err)
		assert.Equal(t, originalMtime, info2.ModTime(), "file should not have been overwritten")
	})

	t.Run("overwrites existing hook when force=true", func(t *testing.T) {
		tempDir := t.TempDir()
		setupGitRepo(t, tempDir, false)

		cfg := &config.Config{}
		opts := GitHookOpts{Hook: hooks.GitPreCommitHook, Path: tempDir, Force: true}
		require.NoError(t, GitHookInstall(t.Context(), cfg, opts))
		info, err := os.Stat(filepath.Join(tempDir, ".git", "hooks", "pre-commit"))
		require.NoError(t, err)
		originalMtime := info.ModTime()
		time.Sleep(10 * time.Millisecond)
		require.NoError(t, GitHookInstall(t.Context(), cfg, opts))
		info2, err := os.Stat(filepath.Join(tempDir, ".git", "hooks", "pre-commit"))
		require.NoError(t, err)
		assert.NotEqual(t, originalMtime, info2.ModTime(), "file should have been overwritten")
	})

	t.Run("returns error for nonexistent path", func(t *testing.T) {
		cfg := &config.Config{}
		err := GitHookInstall(t.Context(), cfg, GitHookOpts{
			Hook: hooks.GitPreCommitHook,
			Path: filepath.Join(t.TempDir(), "nonexistent"),
		})
		require.Error(t, err)
	})
}
