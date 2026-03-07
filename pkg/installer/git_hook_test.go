package installer

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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
	hookName := "git.pre-commit"
	err = gitInstallHookDir(hookName, gitDir)
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
	assert.True(t, strings.HasPrefix(contentStr, "#!/bin/sh\n#"))
	assert.Contains(t, contentStr, "exec leaktk hook git.pre-commit")
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

func TestGitFindRepos(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a regular git repository
	repo1 := filepath.Join(tmpDir, "repo1")
	setupGitRepo(t, repo1, false)

	// Create a bare git repository
	repo2 := filepath.Join(tmpDir, "repo2.git")
	setupGitRepo(t, repo2, true)

	// Create a nested git repository
	nestedDir := filepath.Join(tmpDir, "nested")
	repo3 := filepath.Join(nestedDir, "repo3")
	setupGitRepo(t, repo3, false)

	t.Run("find single repo non-recursive", func(t *testing.T) {
		opts := GitHookOpts{
			Name:      "git.pre-commit",
			Dir:       repo1,
			Recursive: false,
			Force:     false,
		}

		repos, err := gitFindRepos(repo1, opts)
		require.NoError(t, err)
		assert.Len(t, repos, 1)
	})

	t.Run("find multiple repos recursive", func(t *testing.T) {
		opts := GitHookOpts{
			Name:      "git.pre-commit",
			Dir:       tmpDir,
			Recursive: true,
			Force:     false,
		}

		repos, err := gitFindRepos(tmpDir, opts)
		require.NoError(t, err)

		// Should find repo1, repo2.git, and nested/repo3
		assert.GreaterOrEqual(t, len(repos), 3)
	})

	t.Run("skip repos with existing hooks when force=false", func(t *testing.T) {
		// Install a hook in repo1
		ctx := context.Background()
		absDir, err := gitFindAbsDir(ctx, repo1)
		require.NoError(t, err)

		err = gitInstallHookDir("git.pre-commit", absDir)
		require.NoError(t, err)

		opts := GitHookOpts{
			Name:      "git.pre-commit",
			Dir:       tmpDir,
			Recursive: true,
			Force:     false,
		}

		repos, err := gitFindRepos(tmpDir, opts)
		require.NoError(t, err)

		// Should skip repo1 since it has an executable hook
		assert.NotContains(t, repos, absDir)
	})

	t.Run("include repos with existing hooks when force=true", func(t *testing.T) {
		opts := GitHookOpts{
			Name:      "git.pre-commit",
			Dir:       tmpDir,
			Recursive: true,
			Force:     true,
		}

		repos, err := gitFindRepos(tmpDir, opts)
		require.NoError(t, err)

		// Should find all repos including repo1
		assert.GreaterOrEqual(t, len(repos), 3)
	})

	t.Run("non-recursive stops at top level", func(t *testing.T) {
		opts := GitHookOpts{
			Name:      "git.pre-commit",
			Dir:       tmpDir,
			Recursive: false,
			Force:     true,
		}

		repos, err := gitFindRepos(tmpDir, opts)
		require.NoError(t, err)

		// Should not find nested repo
		ctx := context.Background()
		nestedAbsDir, _ := gitFindAbsDir(ctx, repo3)
		assert.NotContains(t, repos, nestedAbsDir)
	})
}

func TestGitHookInstall(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test repositories
	repo1 := filepath.Join(tmpDir, "repo1")
	setupGitRepo(t, repo1, false)

	repo2 := filepath.Join(tmpDir, "subdir", "repo2")
	setupGitRepo(t, repo2, false)

	cfg := &config.Config{}

	t.Run("install in single repo", func(t *testing.T) {
		opts := GitHookOpts{
			Name:      "git.pre-commit",
			Dir:       repo1,
			Recursive: false,
			Force:     false,
		}

		err := GitHookInstall(cfg, opts)
		require.NoError(t, err)

		// Verify hook was installed
		hookPath := filepath.Join(repo1, ".git", "hooks", "pre-commit")
		assert.True(t, fs.IsExecutable(hookPath))
	})

	t.Run("install recursively", func(t *testing.T) {
		opts := GitHookOpts{
			Name:      "git.pre-commit",
			Dir:       tmpDir,
			Recursive: true,
			Force:     true,
		}

		err := GitHookInstall(cfg, opts)
		require.NoError(t, err)

		// Verify hooks were installed in both repos
		hook1 := filepath.Join(repo1, ".git", "hooks", "pre-commit")
		hook2 := filepath.Join(repo2, ".git", "hooks", "pre-commit")

		assert.True(t, fs.IsExecutable(hook1))
		assert.True(t, fs.IsExecutable(hook2))
	})

	t.Run("fail with empty dir", func(t *testing.T) {
		opts := GitHookOpts{
			Name: "git.pre-commit",
			Dir:  "",
		}

		err := GitHookInstall(cfg, opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "directory path is required")
	})

	t.Run("handle nonexistent directory", func(t *testing.T) {
		opts := GitHookOpts{
			Name: "git.pre-commit",
			Dir:  filepath.Join(tmpDir, "nonexistent"),
		}

		err := GitHookInstall(cfg, opts)
		require.Error(t, err)
	})
}
