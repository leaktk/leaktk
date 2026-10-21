package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/leaktk/leaktk/internal/auths"
	"github.com/leaktk/leaktk/internal/httpclient"
	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/logger"
)

func initOIDCAuth(a *auths.OIDCAuth, serverURL string) error {
	client := httpclient.NewClient()
	resp, err := client.Get(serverURL + "/userinfo")
	if err != nil {
		return fmt.Errorf("get server issuer: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		return fmt.Errorf("unexpected status code: status_code=%d", resp.StatusCode)
	}

	header := resp.Header.Get("WWW-Authenticate")
	if len(header) == 0 {
		return errors.New("no www-authenticate header")
	}

	return a.FromHeader(header)
}

func runLogin(cmd *cobra.Command, args []string) {
	serverURL := cfg.Scanner.Patterns.Server.URL
	if len(args) > 0 {
		serverURL = args[0]
	}

	logger.Info("logging in: server=%q", serverURL)

	flags := cmd.Flags()
	token := mustGetString(flags, "token")
	web := mustGetBool(flags, "web")

	if len(token) == 0 {
		if web {
			var a auths.OIDCAuth

			if err := initOIDCAuth(&a, serverURL); err != nil {
				logger.Debug("init oidc auth: %v", err)
			} else if len(a.Issuer) == 0 {
				logger.Debug("init oidc auth: no issuer")
			} else if err = a.Login(cmd.Context()); err != nil {
				logger.Debug("oidc auth login: %v", err)
			}

			token = a.Token
			if len(token) == 0 {
				logger.Warning("server does not support web login flow")
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

	if err := config.SaveServerAuth(serverURL, token); err != nil {
		logger.Fatal("could not save token: %v", err)
	}

	logger.Info("login successful")
}

func loginCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login [url]",
		Short: "Log into a LeakTK server",
		Args:  cobra.MaximumNArgs(1),
		Run:   runLogin,
	}

	cmd.Flags().String("token", "", "Bearer token for authentication to the server")
	cmd.Flags().BoolP("web", "w", false, "Login with web browser using OAuth2")

	return cmd
}
func runLogout(cmd *cobra.Command, args []string) {
	logger.Info("logging out: server=%q", cfg.Scanner.Patterns.Server.URL)

	if err := config.RemoveServerAuth(); err != nil {
		logger.Fatal("could not logout: %v", err)
	}

	logger.Info("logout successful")
}

func logoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Log out of the configured LeakTK server",
		Run:   runLogout,
	}
}
