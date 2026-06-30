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
		case hooks.PosixHookKind:
			cmd.AddCommand(posixHookInstallCommand(hook))
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
	flags.String("path", "", fmt.Sprintf("Install the %s hook in all git repositories under this path (unless --no-recursive is set)", hookname))
	flags.Bool("no-recursive", false, fmt.Sprintf("Install the %s hook only at the repository at the selected path", hookname))
	flags.Bool("force", false, fmt.Sprintf("Replace any existing %s hooks instead of skipping them", hookname))
	flags.Bool("stdout", false, fmt.Sprintf("Print the %s hook script to stdout (useful for certain custom installs)", hookname))
	return cmd
}

func posixHookInstallCommand(hook hooks.Hook) *cobra.Command {
	hookname := hook.Name()
	cmd := &cobra.Command{
		Use:   hookname,
		Short: "Install and configure " + hookname,
		Run:   runPosixHookInstall,
	}
	flags := cmd.Flags()
	var bashrc, zshrc, stdout bool
	flags.BoolVar(&bashrc, "bashrc", false, "Target ~/.bashrc for installation")
	flags.BoolVar(&zshrc, "zshrc", false, "Target ~/.zshrc for isntallation")
	flags.BoolVar(&stdout, "stdout", false, "Print the command to stdout for ad-hoc custom installs")
	cmd.MarkFlagsMutuallyExclusive("bashrc", "zshrc", "stdout")
	return cmd
}

func runGitHookInstall(cmd *cobra.Command, args []string) {
	flags := cmd.Flags()
	opts := installer.GitHookOpts{
		Hook:              hooks.Hook(cmd.Use),
		UserTemplateDir:   mustGetBool(flags, "user-template-dir"),
		SystemTemplateDir: mustGetBool(flags, "system-template-dir"),
		Path:              mustGetString(flags, "path"),
		Recursive:         !mustGetBool(flags, "no-recursive"),
		Force:             mustGetBool(flags, "force"),
		Stdout:            mustGetBool(flags, "stdout"),
	}

	if len(opts.Path) == 0 && !opts.UserTemplateDir && !opts.SystemTemplateDir && !opts.Stdout {
		logger.Fatal("install requires at least one of: --path, --user-template-dir, --system-template-dir, --stdout")
	}

	if err := installer.GitHookInstall(cmd.Context(), cfg, opts); err != nil {
		logger.Fatal("could not install git hook: %v hookname=%q", err, opts.Hook.Name())
	}
}

func runPosixHookInstall(cmd *cobra.Command, args []string) {
	flags := cmd.Flags()

	opts := installer.PosixStdioHookOpts{
		Hook:   hooks.Hook(cmd.Use),
		Bashrc: mustGetBool(flags, "bashrc"),
		Zshrc:  mustGetBool(flags, "zshrc"),
		Stdout: mustGetBool(flags, "stdout"),
	}

	if !opts.Bashrc && !opts.Zshrc && !opts.Stdout {
		logger.Fatal("install requires at least one of: --bashrc, --zshrc, --stdout")
	}

	if err := installer.PosixStdioHookInstall(cmd.Context(), cfg, opts); err != nil {
		logger.Fatal("could not install posix stdio hook: %v hookname=%q", err, opts.Hook.Name())
	}
}
