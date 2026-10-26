package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/leaktk/leaktk/internal/httpclient"
	"github.com/leaktk/leaktk/internal/sources"
	"github.com/leaktk/leaktk/internal/urlutil"
	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/logger"
)

func runLogin(cmd *cobra.Command, args []string) {
	serverURL := &cfg.Scanner.Patterns.Server.URL
	if len(args) > 0 {
		u, err := urlutil.ParseURL(args[0])
		if err != nil {
			logger.Fatal("invalid URL: url=%q", args[0])
		}
		*serverURL = *u
	}

	logger.Info("logging in: server=%q", serverURL)

	flags := cmd.Flags()
	token := mustGetString(flags, "token")
	web := mustGetBool(flags, "web")

	// Set default source if one doesn't exist in the config
	// or web login fails
	ltkSvrSrc := &sources.LeakTKServer{
		BaseURL:   *serverURL,
		RateLimit: httpclient.NewRateLimit(),
		Save:      config.SaveLeakTKServerSource,
	}

	if len(token) == 0 {
		if web {
			// Search existing source from the config
			for _, s := range cfg.Sources.Iter() {
				switch s := s.(type) {
				case *sources.LeakTKServer:
					if s.AppliesTo(serverURL) {
						ltkSvrSrc = s
						break
					}
				}
			}

			if err := ltkSvrSrc.Login(cmd.Context()); err != nil {
				logger.Debug("oidc auth login: %v", err)
			}

			token = ltkSvrSrc.AccessToken
			if len(token) == 0 {
				logger.Error("unable to complete web login flow")
			}
		}

		if len(token) == 0 {
			fmt.Printf("Enter %s auth token: ", serverURL)
			tokenBytes, err := term.ReadPassword(int(os.Stdin.Fd())) // #nosec G115 -- os.Stdin file descriptor (0) safely fits within int bounds
			fmt.Println("")
			if err != nil {
				logger.Fatal("could not login: %v", err)
			}
			token = strings.TrimSpace(string(tokenBytes))

			if len(token) != 0 {
				ltkSvrSrc.AccessToken = token

				if err := config.SaveLeakTKServerSource(ltkSvrSrc); err != nil {
					logger.Fatal("could not save token: %v", err)
				}
			}
		}
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
	serverURL := &cfg.Scanner.Patterns.Server.URL
	logger.Info("logging out: server=%q", serverURL.String())

	// Search existing source from the config
	updated := false
	for id, s := range cfg.Sources.Iter() {
		switch s := s.(type) {
		case *sources.LeakTKServer:
			if s.ID() == config.LeakTKServerEnvSourceID {
				logger.Debug("skipping source: leaktk server env source: id=%q", s.ID())
				continue
			}

			if (len(s.AccessToken) != 0 || len(s.RefreshToken) != 0) && s.AppliesTo(serverURL) {
				s.AccessToken = ""
				s.RefreshToken = ""

				if err := config.SaveLeakTKServerSource(s); err != nil {
					logger.Fatal("could not update source: id=%q url=%q", id, s.BaseURL.String())
				}

				updated = true
			}
		}
	}

	if updated {
		logger.Info("logout successful")
	} else {
		logger.Info("no saved credentials")
	}
}

func logoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Log out of the configured LeakTK server",
		Run:   runLogout,
	}
}
