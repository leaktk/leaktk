package installer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/fs"
	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/leaktk/leaktk/pkg/version"
)

// GitHookOpts contains flags and options to pass to the git hook installer
type GitHookOpts struct {
	// Name is the name of the hook to install
	Name string

	// Dir is the path to install the hook under. It can be used in addition to the other Dir flags
	Dir string

	// Force replaces existing hooks instead of skipping them
	Force bool

	// Recursive installs hooks in repositories under the top level Dir path
	Recursive bool

	// GlobalTemplateDir installs the hook in the global git template
	GlobalTemplateDir bool

	// UserTemplateDir installs the hook in the user's git init.templateDir (and creates it if it doesn't exist)
	UserTemplateDir bool
}

const gitHookTemplate = `#!/bin/sh
#TemplateID: 6b563e53-cf8c-49d1-8b4c-a03835c7daa9
#CreatedBy: %s
#CreatedOn: %s
exec leaktk hook %s "$@"
`

// gitFindAbsDir returns the absolute git directory path for a given path
// It wraps the git rev-parse --absolute-git-dir command
func gitFindAbsDir(ctx context.Context, path string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", path, "rev-parse", "--absolute-git-dir") // #nosec G204
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// gitInstallHookDir installs a git hook script in the specified directory
// gitHookName is the full hook name (e.g., "git.pre-commit")
// installDirPath is the path to the .git directory where hooks should be installed
func gitInstallHookDir(gitHookName, installDirPath string) error {
	// Extract the git hook script name from the full hook name
	// e.g., "git.pre-commit" -> "pre-commit"
	_, scriptName, found := strings.Cut(gitHookName, ".")
	if !found {
		return fmt.Errorf("invalid git hook name format: %s", gitHookName)
	}

	// Build the hook script path
	hooksDir := filepath.Join(installDirPath, "hooks")
	hookPath := filepath.Join(hooksDir, scriptName)

	// Ensure hooks directory exists
	if !fs.DirExists(hooksDir) {
		if err := os.MkdirAll(hooksDir, 0755); err != nil { // #nosec G301 -- git hooks directory standard permissions
			return fmt.Errorf("failed to create hooks directory: %w", err)
		}
	}

	// Generate the hook script content
	createdOn := time.Now().Format(time.RFC3339)
	createdBy := "leaktk-" + version.Version
	script := fmt.Sprintf(gitHookTemplate, createdBy, createdOn, gitHookName)

	// Write the hook script (0750 required for executable)
	if err := os.WriteFile(hookPath, []byte(script), 0750); err != nil { // #nosec G306 -- hook script must be executable
		return fmt.Errorf("failed to write hook script: %w", err)
	}

	logger.Info("installed hook: path=%q", hookPath)
	return nil
}

// gitFindRepos finds all git repositories under rootDir and returns their git directories
// It respects the options in opts:
// - If Force is false, repos with existing executable hooks are skipped
// - Only walks subdirectories if Recursive is true
// - GlobalTemplateDir and UserTemplateDir are ignored (handled elsewhere)
func gitFindRepos(rootDir string, opts GitHookOpts) ([]string, error) {
	ctx := context.Background()
	var gitDirs []string

	// Extract the git hook script name from the full hook name
	_, scriptName, found := strings.Cut(opts.Name, ".")
	if !found {
		return nil, fmt.Errorf("invalid git hook name format: %s", opts.Name)
	}

	// Helper function to check if a directory is a git repo and add it
	checkAndAddRepo := func(path string) {
		// Check if this is a git directory
		// Check if it's a .git directory or ends with .git (bare repo)
		isGitDir := (filepath.Base(path) == ".git" && fs.DirExists(path)) ||
			(strings.HasSuffix(path, ".git") && fs.DirExists(path))

		// If it looks like a git directory, verify with git command
		if isGitDir {
			absDir, err := gitFindAbsDir(ctx, path)
			if err != nil {
				// Not a valid git repo, skip
				return
			}

			// Check if hook already exists and is executable
			hookPath := filepath.Join(absDir, "hooks", scriptName)
			if !opts.Force && fs.IsExecutable(hookPath) {
				logger.Debug("skipping repo with existing hook: path=%q", absDir)
				return
			}

			gitDirs = append(gitDirs, absDir)
			return
		}

		// If it's a regular directory (not .git), check if it contains a .git subdirectory
		gitSubDir := filepath.Join(path, ".git")
		if fs.DirExists(gitSubDir) {
			absDir, err := gitFindAbsDir(ctx, path)
			if err != nil {
				// Not a valid git repo, skip
				return
			}

			// Check if hook already exists and is executable
			hookPath := filepath.Join(absDir, "hooks", scriptName)
			if !opts.Force && fs.IsExecutable(hookPath) {
				logger.Debug("skipping repo with existing hook: path=%q", absDir)
				return
			}

			gitDirs = append(gitDirs, absDir)
		}
	}

	// Check the root directory itself
	checkAndAddRepo(rootDir)

	// If recursive, walk subdirectories
	if opts.Recursive {
		entries, err := os.ReadDir(rootDir)
		if err != nil {
			return nil, fmt.Errorf("failed to read directory: %w", err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			subPath := filepath.Join(rootDir, entry.Name())

			// Recursively search subdirectories
			subGitDirs, err := gitFindRepos(subPath, opts)
			if err != nil {
				logger.Warning("error searching subdirectory: path=%q error=%v", subPath, err)
				continue
			}

			gitDirs = append(gitDirs, subGitDirs...)
		}
	}

	return gitDirs, nil
}

// GitHookInstall is the install function for git hooks
func GitHookInstall(cfg *config.Config, opts GitHookOpts) error {
	if opts.Dir == "" {
		return errors.New("directory path is required")
	}

	// Check if directory exists
	if !fs.PathExists(opts.Dir) {
		return fmt.Errorf("directory does not exist: %s", opts.Dir)
	}

	// Find all git repositories
	gitDirs, err := gitFindRepos(opts.Dir, opts)
	if err != nil {
		return fmt.Errorf("failed to find git repositories: %w", err)
	}

	if len(gitDirs) == 0 {
		logger.Warning("no git repositories found")
		return nil
	}

	// Install hooks in each repository
	var installErrors []error
	for _, gitDir := range gitDirs {
		if err := gitInstallHookDir(opts.Name, gitDir); err != nil {
			installErrors = append(installErrors, fmt.Errorf("failed to install in %s: %w", gitDir, err))
		}
	}

	if len(installErrors) > 0 {
		return fmt.Errorf("encountered %d error(s) during installation: %v", len(installErrors), installErrors)
	}

	logger.Info("successfully installed hook in %d repositories", len(gitDirs))
	return nil
}
