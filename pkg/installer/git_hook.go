package installer

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/adrg/xdg"

	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/fs"
	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/leaktk/leaktk/pkg/version"
)

const systemGitTemplateDir = "/usr/share/git-core/templates"

// GitHookOpts contains flags and options to pass to the git hook installer
type GitHookOpts struct {
	// Name is the hookname to install (e.g. "git.pre-commit")
	Name string

	// Path is the directory to search for git repositories to install into
	Path string

	// Recursive tells the installer to look git repositories in sub-paths.
	Recursive bool

	// Force replaces existing hooks instead of skipping them
	Force bool

	// SystemTemplateDir installs the hook in /usr/share/git-core/templates
	SystemTemplateDir bool

	// UserTemplateDir installs the hook in the user's git init.templateDir
	// (one is created at ~/.config/git/template if not already configured)
	UserTemplateDir bool
}

// defaultErrorDocsURL is the base URL template for error documentation.
// %s is replaced with the error code (e.g. "command_not_found").
// This can be made configurable in the future via config.toml.
const defaultErrorDocsURL = "https://github.com/leaktk/leaktk/blob/HEAD/docs/errors/%s.md"

// gitPreCommitHookTemplate is the shell script written to .git/hooks/pre-commit.
// Placeholders in order: createdBy, createdOn, hookname, errorDocURL.
// TemplateID must stay the same across leaktk versions as long as this
// script's content does not change, so repos can be audited by template version.
const gitPreCommitHookTemplate = `#!/bin/sh
# TemplateID: f2998aee-4684-4c46-b724-7c8e37e6020c
# CreatedBy: %s
# CreatedOn: %s
if command -v leaktk > /dev/null 2>&1
then
    exec leaktk hook %s
else
    echo 'leaktk command not found' >&2
    echo 'See: %s' >&2
    exit 1
fi
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

// hookScriptName extracts the script filename from a full hookname.
// e.g. "git.pre-commit" -> "pre-commit"
func hookScriptName(hookname string) string {
	_, name, _ := strings.Cut(hookname, ".")
	return name
}

// findGitDirs returns the absolute git directory path for every git repository
// found at or under rootDir. It always recurses. It skips .git directory
// internals and bare repo internals to avoid double-counting.
func findGitDirs(ctx context.Context, rootDir string) ([]string, error) {
	var gitDirs []string

	err := filepath.WalkDir(rootDir, func(path string, d iofs.DirEntry, err error) error {
		if err != nil {
			logger.Warning("skipping path: %v path=%q", err, path)
			return nil
		}

		if !d.IsDir() {
			return nil
		}

		// Don't recurse inside .git directories
		if d.Name() == ".git" {
			return filepath.SkipDir
		}

		// Working tree: directory contains a .git entry (dir for normal repos,
		// file for submodules/worktrees)
		gitEntry := filepath.Join(path, ".git")
		if fs.PathExists(gitEntry) {
			absDir, err := gitFindAbsDir(ctx, path)
			if err != nil {
				logger.Warning("skipping repo: %v path=%q", err, path)
				return nil
			}
			gitDirs = append(gitDirs, absDir)
			return nil
		}

		// Bare repo: directory name ends in .git (e.g. repo.git) and git
		// recognises it as a git directory
		if strings.HasSuffix(d.Name(), ".git") {
			absDir, err := gitFindAbsDir(ctx, path)
			if err == nil {
				gitDirs = append(gitDirs, absDir)
				return filepath.SkipDir // don't descend into bare repo internals
			}
		}

		return nil
	})

	return gitDirs, err
}

// gitUserTemplateDir returns the path to the user's git init.templateDir.
// If init.templateDir is not configured, it creates a default directory at
// $XDG_CONFIG_HOME/git/template and sets init.templateDir to that path.
func gitUserTemplateDir(ctx context.Context) (string, error) {
	// Try reading the current value
	cmd := exec.CommandContext(ctx, "git", "config", "--global", "init.templateDir") // #nosec G204
	if output, err := cmd.Output(); err == nil {
		dir := strings.TrimSpace(string(output))
		if dir != "" {
			return filepath.Clean(expandTilde(dir)), nil
		}
	}

	// Not configured — create a default and set it.
	// Reload picks up any runtime changes to XDG_CONFIG_HOME (e.g. in tests).
	xdg.Reload()
	defaultDir := filepath.Join(xdg.ConfigHome, "git", "template")
	if err := os.MkdirAll(defaultDir, 0700); err != nil {
		return "", fmt.Errorf("could not create template dir: %w path=%q", err, defaultDir)
	}

	setCmd := exec.CommandContext(ctx, "git", "config", "--global", "init.templateDir", defaultDir) // #nosec G204
	if err := setCmd.Run(); err != nil {
		return "", fmt.Errorf("could not set init.templateDir: %w", err)
	}

	logger.Info("configured git init.templateDir: path=%q", defaultDir)
	return defaultDir, nil
}

// expandTilde replaces a leading ~ with the current user's home directory.
func expandTilde(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[2:])
}

// gitInstallHookDir installs a git hook script into installDirPath (the .git directory).
// gitHookName is the full hook name e.g. "git.pre-commit".
func gitInstallHookDir(gitHookName, installDirPath string) error {
	_, scriptName, found := strings.Cut(gitHookName, ".")
	if !found {
		return fmt.Errorf("invalid git hook name: hookname=%q", gitHookName)
	}

	hooksDir := filepath.Join(installDirPath, "hooks")
	hookPath := filepath.Join(hooksDir, scriptName)

	if err := os.MkdirAll(hooksDir, 0755); err != nil { // #nosec G301 -- git hooks directory standard permissions
		return fmt.Errorf("could not create hooks dir: path=%q error=%w", hooksDir, err)
	}

	createdBy := "leaktk-" + version.Version
	createdOn := time.Now().UTC().Format(time.RFC3339)
	errorDocURL := fmt.Sprintf(defaultErrorDocsURL, "command_not_found")
	script := fmt.Sprintf(gitPreCommitHookTemplate, createdBy, createdOn, gitHookName, errorDocURL)

	if err := os.WriteFile(hookPath, []byte(script), 0750); err != nil { // #nosec G306 -- hook script must be executable
		return fmt.Errorf("could not write hook: path=%q error=%w", hookPath, err)
	}

	logger.Info("installed hook: path=%q", hookPath)
	return nil
}

// GitHookInstall installs git hooks according to opts.
// It installs in all git repos found under opts.Path, and optionally in the
// user's git init.templateDir and/or the system git template directory.
func GitHookInstall(cfg *config.Config, opts GitHookOpts) error {
	ctx := context.Background()
	var installErrors []error
	var err error

	if opts.Path != "" {
		if !fs.PathExists(opts.Path) {
			return fmt.Errorf("path does not exist: path=%q", opts.Path)
		}

		var gitDirs []string

		if !opts.Recursive {
			gitDir, err := gitFindAbsDir(ctx, opts.Path)
			if err != nil {
				return fmt.Errorf("could not find git repo: %w path=%q", err, opts.Path)
			}
			if len(gitDir) > 0 {
				gitDirs = append(gitDirs, gitDir)
			}
		} else {
			gitDirs, err = findGitDirs(ctx, opts.Path)
			if err != nil {
				return fmt.Errorf("could not find git repos: %w path=%q", err, opts.Path)
			}
		}

		if len(gitDirs) == 0 {
			logger.Warning("no git repositories found: path=%q", opts.Path)
		}

		for _, gitDir := range gitDirs {
			hookPath := filepath.Join(gitDir, "hooks", hookScriptName(opts.Name))
			if fs.IsExecutable(hookPath) && !opts.Force {
				logger.Info("skipping existing hook: path=%q", hookPath)
				continue
			}
			if err := gitInstallHookDir(opts.Name, gitDir); err != nil {
				installErrors = append(installErrors, err)
			}
		}
	}

	if opts.UserTemplateDir {
		templateDir, err := gitUserTemplateDir(ctx)
		if err != nil {
			installErrors = append(installErrors, fmt.Errorf("could not resolve user template dir: %w", err))
		} else {
			hookPath := filepath.Join(templateDir, "hooks", hookScriptName(opts.Name))
			if fs.IsExecutable(hookPath) && !opts.Force {
				logger.Info("skipping existing hook in user template dir: path=%q", hookPath)
			} else if err := gitInstallHookDir(opts.Name, templateDir); err != nil {
				installErrors = append(installErrors, err)
			}
		}
	}

	if opts.SystemTemplateDir {
		hookPath := filepath.Join(systemGitTemplateDir, "hooks", hookScriptName(opts.Name))
		if fs.IsExecutable(hookPath) && !opts.Force {
			logger.Info("skipping existing hook in system template dir: path=%q", hookPath)
		} else if err := gitInstallHookDir(opts.Name, systemGitTemplateDir); err != nil {
			installErrors = append(installErrors, err)
		}
	}

	return errors.Join(installErrors...)
}
