package cmd

import (
	"fmt"
	"strings"

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
	for _, hookname := range hooks.Names {
		hookkind, _, found := strings.Cut(hookname, ".")
		if !found {
			// All hooknames must have <kind>.<event>
			logger.Fatal("invalid hookname detected: hookname=%q", hookname)
		}

		switch hookkind {
		case "git":
			cmd.AddCommand(gitHookInstallCommand(hookname))
		default:
			logger.Fatal("hookkind not supported by installer: hookkind=%q", hookkind)
		}
	}
	return cmd
}

func gitHookInstallCommand(hookname string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   hookname,
		Short: "Install and configure " + hookname,
		Run:   runGitHookInstall,
	}

	flags := cmd.Flags()
	flags.Bool("user-template-dir", false, fmt.Sprintf("Install the %s hook in your git init.templateDir (one is created if not already defined)", hookname))
	flags.Bool("system-template-dir", false, fmt.Sprintf("Install the %s hook in /usr/share/git-core/templates", hookname))
	flags.StringP("dir", "d", "", fmt.Sprintf("Install the %s hook in the git repository at this path", hookname))
	flags.BoolP("recursive", "r", false, fmt.Sprintf("Install the %s hook in all git repositories under --dir=<path>", hookname))
	flags.Bool("force", false, fmt.Sprintf("Replace any existing %s hooks instead of skipping them", hookname))

	return cmd
}

func runGitHookInstall(cmd *cobra.Command, args []string) {
	opts := installer.GitHookOpts{
		Name:              cmd.Flags().Name(),
		UserTemplateDir:   false, // TODO
		GlobalTemplateDir: false, // TODO
		Dir:               "",    // TODO
		Recursive:         false, // TODO
		Force:             false, // TODO
	}

	if err := installer.GitHookInstall(cfg, opts); err != nil {
		logger.Fatal("could not install git hook: %v hookname=%q", err, opts.Name)
	}
}
