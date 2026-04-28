package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/leaktk/leaktk/pkg/hooks"
	"github.com/leaktk/leaktk/pkg/installer"
	"github.com/leaktk/leaktk/pkg/logger"
)

func installCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install and configure leaktk subsystems",
		Run:   runHelp,
	}
	cmd.AddCommand(hookInstallCommand())
	return cmd
}

func hookInstallCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook",
		Short: "Install and configure hooks",
		Run:   runHelp,
	}
	for _, hook := range hooks.Hooks {
		switch hookkind := hook.Kind(); hookkind {
		case hooks.GitHookKind:
			cmd.AddCommand(gitHookInstallCommand(hook))
		default:
			logger.Fatal("hookkind not supported by installer: hookkind=%q", hookkind)
		}
	}
	return cmd
}

func gitHookInstallCommand(hook hooks.Hook) *cobra.Command {
	hookname := hook.Name()
	cmd := &cobra.Command{
		Use:   hookname,
		Short: "Install and configure " + hookname,
		Run:   runGitHookInstall,
	}
	flags := cmd.Flags()
	flags.Bool("user-template-dir", false, fmt.Sprintf("Install the %s hook in your git init.templateDir (one is created if not already defined)", hookname))
	flags.Bool("system-template-dir", false, fmt.Sprintf("Install the %s hook in /usr/share/git-core/templates", hookname))
	flags.String("path", "", fmt.Sprintf("Install the %s hook in all git repositories under this path", hookname))
	flags.Bool("recursive", false, fmt.Sprintf("Install the %s hook in all git repositories under the selected path", hookname))
	flags.Bool("force", false, fmt.Sprintf("Replace any existing %s hooks instead of skipping them", hookname))
	return cmd
}

func runGitHookInstall(cmd *cobra.Command, args []string) {
	flags := cmd.Flags()
	opts := installer.GitHookOpts{
		Hook:              hooks.Hook(cmd.Use),
		UserTemplateDir:   mustGetBool(flags, "user-template-dir"),
		SystemTemplateDir: mustGetBool(flags, "system-template-dir"),
		Path:              mustGetString(flags, "path"),
		Recursive:         mustGetBool(flags, "recursive"),
		Force:             mustGetBool(flags, "force"),
	}

	if len(opts.Path) == 0 && !opts.UserTemplateDir && !opts.SystemTemplateDir {
		logger.Fatal("install requires at least one of: --path, --user-template-dir, --system-template-dir")
	}

	if err := installer.GitHookInstall(cmd.Context(), cfg, opts); err != nil {
		logger.Fatal("could not install git hook: %v hookname=%q", err, opts.Hook.Name())
	}
}
