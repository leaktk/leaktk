package cmd

import (
	"fmt"
	"os"
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
	flags.StringP("path", "p", "", fmt.Sprintf("Install the %s hook in all git repositories under this path (defaults to current directory)", hookname))
	flags.BoolP("force", "f", false, fmt.Sprintf("Replace any existing %s hooks instead of skipping them", hookname))

	return cmd
}

func runGitHookInstall(cmd *cobra.Command, args []string) {
	flags := cmd.Flags()

	userTemplateDir, _ := flags.GetBool("user-template-dir")
	systemTemplateDir, _ := flags.GetBool("system-template-dir")
	path, _ := flags.GetString("path")
	force, _ := flags.GetBool("force")

	// Use current directory if no path specified
	if path == "" {
		var err error
		path, err = os.Getwd()
		if err != nil {
			logger.Fatal("could not get current directory: %v", err)
		}
	}

	opts := installer.GitHookOpts{
		Name:              cmd.Use,
		UserTemplateDir:   userTemplateDir,
		SystemTemplateDir: systemTemplateDir,
		Path:              path,
		Force:             force,
	}

	if err := installer.GitHookInstall(cfg, opts); err != nil {
		logger.Fatal("could not install git hook: %v hookname=%q", err, opts.Name)
	}
}
