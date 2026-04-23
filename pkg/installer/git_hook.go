package installer

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/adrg/xdg"

	"github.com/leaktk/leaktk/internal/git"
	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/docs"
	"github.com/leaktk/leaktk/pkg/fs"
	"github.com/leaktk/leaktk/pkg/hooks"
	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/leaktk/leaktk/pkg/version"
)

const systemGitTemplateDir = "/usr/share/git-core/templates"

// GitHookOpts contains flags and options to pass to the git hook installer
type GitHookOpts struct {
	// Hook is the hook to install (e.g. hooks.Hook("git.pre-commit"))
	Hook hooks.Hook
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

// gitPreCommitHookTemplate is the shell script written to .git/hooks/pre-commit.
// TemplateID SHOULD stay the same across leaktk versions as long as this
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

// findGitDirs returns the absolute git directory path for every git repository
// found at or under the root path
func findGitDirs(ctx context.Context, root string) ([]string, error) {
	// Use a set in case multiple worktrees point to the same git dir
	gitDirSet := make(map[string]bool)

	err := filepath.WalkDir(root, func(path string, d iofs.DirEntry, err error) error {
		if err != nil {
			logger.Error("could not access path: %v path=%q", err, path)
			return nil
		}

		// Ignore anything that isn't a .git file or directory
		if !strings.HasSuffix(d.Name(), ".git") {
			logger.Trace("skipping path without .git suffix: path=%q", path)
			return nil
		}

		repoInfo, err := git.GetRepoInfo(ctx, path)
		if err != nil {
			logger.Error("could not get repo info: %v path=%q", err, path)
			return nil
		}

		// Add the new dir to the gitDirSet
		if !gitDirSet[repoInfo.GitDir] {
			gitDirSet[repoInfo.GitDir] = true
		}
		return nil
	})

	return slices.Sorted(maps.Keys(gitDirSet)), err
}

// gitUserTemplateDir returns the path to the user's git init.templateDir.
// If init.templateDir is not configured, it creates a default directory at
// $XDG_CONFIG_HOME/git/template and sets init.templateDir to that path.
func gitUserTemplateDir(ctx context.Context) (string, error) {
	// NOTE: GetGlobalConfigPath handles ~ expansion for paths in git configs
	templateDir, err := git.GetGlobalConfigPath(ctx, "init.templateDir")
	if err != nil {
		return "", fmt.Errorf("could not look up user template dir: %w", err)
	}

	if len(templateDir) > 1 {
		return templateDir, nil
	}

	templateDir = filepath.Join(xdg.ConfigHome, "git", "template")
	if err := os.MkdirAll(templateDir, 0700); err != nil {
		return "", fmt.Errorf("could not create user git template dir: %w path=%q", err, templateDir)
	}

	if err := git.SetGlobalConfigPath(ctx, "init.templateDir", templateDir); err != nil {
		return "", fmt.Errorf("could not set user git template dir: %w path=%q", err, templateDir)
	}

	logger.Info("configured user git template dir: path=%q", templateDir)
	return templateDir, nil
}

// gitHookInstall installs a git hook script into installDir (the .git directory).
func gitHookInstall(hook hooks.Hook, installDir string, force bool) error {
	hooksDir := filepath.Join(installDir, "hooks")
	hookPath := filepath.Join(hooksDir, hook.Event())

	if fs.IsExecutable(hookPath) && !force {
		logger.Info("skipping existing hook: force install not enabled: path=%q force_install=%v", hookPath, force)
		return nil
	}

	if err := os.MkdirAll(hooksDir, 0700); err != nil {
		return fmt.Errorf("could not create hooks dir: %w path=%q", err, hooksDir)
	}

	createdBy := "leaktk-" + version.Version
	createdOn := time.Now().UTC().Format(time.RFC3339)
	script := fmt.Sprintf(
		gitPreCommitHookTemplate,
		createdBy,
		createdOn,
		hook.Name(),
		docs.DocURL(docs.CommandNotFoundTopic),
	)

	if err := os.WriteFile(hookPath, []byte(script), 0750); err != nil { // #nosec G306 -- hook script must be executable
		return fmt.Errorf("could not write hook: %w path=%q", err, hookPath)
	}
	logger.Info("installed hook: hook=%q path=%q", hook.Name(), hookPath)
	return nil
}

// GitHookInstall installs git hooks according to opts.
// It installs in all git repos found under opts.Path, and optionally in the
// user's git init.templateDir and/or the system git template directory.
func GitHookInstall(ctx context.Context, cfg *config.Config, opts GitHookOpts) error {
	var err error
	var gitDirs []string

	hookname := opts.Hook.Name()
	hadErrors := false

	if opts.Path != "" {
		if !fs.PathExists(opts.Path) {
			return fmt.Errorf("path does not exist: path=%q", opts.Path)
		}

		if !opts.Recursive {
			repoInfo, err := git.GetRepoInfo(ctx, opts.Path)
			if err != nil {
				return fmt.Errorf("could not find git repo: %w hookname=%q path=%q", err, hookname, opts.Path)
			}
			if len(repoInfo.GitDir) > 0 {
				gitDirs = append(gitDirs, repoInfo.GitDir)
			}
		} else {
			gitDirs, err = findGitDirs(ctx, opts.Path)
			if err != nil {
				return fmt.Errorf("could not find git repos: %w hookname=%q path=%q", err, hookname, opts.Path)
			}
		}

		if len(gitDirs) == 0 {
			logger.Warning("no git repositories found: hookname=%q path=%q", hookname, opts.Path)
		}

		for _, gitDir := range gitDirs {
			logger.Info("installing hook: hookname=%q path=%q", hookname, gitDir)
			if err := gitHookInstall(opts.Hook, gitDir, opts.Force); err != nil {
				logger.Info("could not install hook: %v hookname=%q path=%q", err, hookname, gitDir)
				hadErrors = true
			}
		}
	}

	if opts.UserTemplateDir {
		userGitTemplateDir, err := gitUserTemplateDir(ctx)
		if err != nil {
			logger.Error("could not resolve user template dir: %v hookname=%s", err, hookname)
			hadErrors = true
		} else {
			logger.Info("installing hook in user git template dir: hookname=%q path=%q", hookname, userGitTemplateDir)
			if err := gitHookInstall(opts.Hook, userGitTemplateDir, opts.Force); err != nil {
				logger.Info(
					"could not install hook in user git template dir: %v hookname=%q path=%q",
					err, hookname, userGitTemplateDir,
				)
				hadErrors = true
			}
		}
	}

	if opts.SystemTemplateDir {
		logger.Info("installing hook in system git template dir: hookname=%q path=%q", hookname, systemGitTemplateDir)
		if err := gitHookInstall(opts.Hook, systemGitTemplateDir, opts.Force); err != nil {
			logger.Info(
				"could not install hook in ystem git template dir: %v hookname=%q path=%q",
				err, hookname, systemGitTemplateDir,
			)
			hadErrors = true
		}
	}

	if hadErrors {
		return errors.New("errors detected during install")
	}
	return nil
}
