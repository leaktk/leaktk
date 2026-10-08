package cmd

import (
	"fmt"
	"os"

	"github.com/leaktk/leaktk/internal/auths"
	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func runLogin(cmd *cobra.Command, args []string) {
	var err error

	serverURL := cfg.Scanner.Patterns.Server.URL
	if len(args) > 0 {
		serverURL = args[0]
	}

	logger.Info("logging in: pattern_server=%q", serverURL)

	flags := cmd.Flags()
	token := mustGetString(flags, "token")
	web := mustGetBool(flags, "web")

	if len(token) == 0 {
		if web {
			oidcAuth := &auths.OIDCAuth{
				Flags:     OIDCAllowWebLogin,
				IssuerURL: serverURL,
			}
			if err := oidcAuth.Login(); err != nil {
				logger.Error("web login failed: %v", err)
			} else {
				token = oidcAuth.Token
			}
		}

		if len(token) == 0 {
			fmt.Printf("Enter %s auth token: ", serverURL)
			//nolint:gosec // os.Stdin file descriptor (0) safely fits within int bounds
			tokenBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Println("")
			if err != nil {
				logger.Fatal("could not login: %v", err)
			}
			token = string(tokenBytes)
		}
	}

	if err := config.SavePatternServerAuth(serverURL, token); err != nil {
		logger.Fatal("could not save token: %v", err)
	}

	logger.Info("login successful")
}

func loginCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login [url]",
		Short: "Log into a pattern server",
		Args:  cobra.MaximumNArgs(1),
		Run:   runLogin,
	}

	cmd.Flags().String("token", "", "Bearer token for authentication to the server")
	cmd.Flags().BoolP("web", "w", false, "Login with web browser using OAuth2")

	return cmd
}
func runLogout(cmd *cobra.Command, args []string) {
	logger.Info("logging out: pattern_server=%q", cfg.Scanner.Patterns.Server.URL)

	if err := config.RemovePatternServerAuth(); err != nil {
		logger.Fatal("could not logout: %v", err)
	}

	logger.Info("logout successful")
}

func logoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Log out of the configured pattern server",
		Run:   runLogout,
	}
}
