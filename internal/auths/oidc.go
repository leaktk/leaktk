package auths

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/pkg/browser"
	"golang.org/x/oauth2"
)

var wwwAuthParamRe = regexp.MustCompile(`([^\s"=,]+)\s*=\s*(?:"((?:[^"\\]|\\.)*)"|([^\s",]+))`)

const oidcDefaultClientID = "leaktk-cli"

type OIDCAuthFlag int

const (
	// Bit flags for setting behavior on OIDCAuth
	OIDCNoFlags        OIDCAuthFlag = 0
	OIDCAllowAutoLogin              = 1 << iota
	OIDCAllowWebLogin
)

func oidcGenRandState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

type OIDCAuth struct {
	Token     string `toml:"token"`
	IssuerURL string `toml:"iss"`
	ClientID  string `toml:"client_id"`
	IssuerURL string `toml:"issuer_url"`
	Flags     OIDCAuthFlag
}

func (a *OIDCAuth) UnmarshalText(text []byte) error {
	header = string(text)
	if len(header) == 0 {
		return errors.New("header empty")
	}

	scheme, paramStr, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return fmt.Errorf("unsupported auth scheme: %q", scheme)
	}

	// TODO: make simpler if we only need realm
	// parse params
	params := make(map[string]string, 3)
	for _, match := range wwwAuthParamRe.FindAllStringSubmatch(paramStr, -1) {
		key := match[1]
		if match[2] != "" {
			// Clean up escaped quotes (convert \" back to ")
			params[key] = strings.ReplaceAll(match[2], `\"`, `"`)
		} else {
			// It matched the unquoted group (Group 3)
			params[key] = match[3]
		}
	}

	a.IssuerURL = params["realm"]
	if len(a.IssuerURL) == 0 {
		return errors.New("header missing realm")
	}

	return nil
}

func (a *OIDCAuth) SetHeader(ctx context.Context, h http.Header) error {
	if len(a.Token) == 0 {
		if a.Flags&OIDCAllowAutoLogin == 0 {
			return errors.New("no token set and autologin disabled")
		}
		if err := a.Login(ctx); err != nil {
			return fmt.Errorf("login: %w", err)
		}
	}
	h.Set("Authorization", "Bearer "+a.Token)
	return nil
}

func (a *OIDCAuth) Login(ctx context.Context) error {
	if len(a.ClientID) == 0 {
		a.ClientID = oidcDefaultClientID
	}
	if len(a.IssuerURL) == 0 {
		return errors.New("no issuer URL set")
	}

	provider, err := oidc.NewProvider(ctx, a.IssuerURL)
	if err != nil {
		return fmt.Errorf("could not query OIDC provider: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return errors.New("could not create login listener")
	}
	defer listener.Close()

	redirectURL := fmt.Sprintf("http://%s/callback", listener.Addr())
	loginURL := fmt.Sprintf("http://%s/login", listener.Addr())

	// Create config
	oauth2Config := oauth2.Config{
		ClientID:    a.ClientID,
		RedirectURL: redirectURL,
		Endpoint:    provider.Endpoint(),
		Scopes:      []string{oidc.ScopeOpenID},
	}

	// Create verifier
	verifier := provider.VerifierContext(ctx, &oidc.Config{ClientID: a.ClientID})

	// Channel to signal flow completion or failure back to Login()
	resultChan := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		state, err := oidcGenRandState()
		if err != nil {
			http.Error(w, "Failed to generate state", http.StatusInternalServerError)
			return
		}
		nonce, err := oidcGenRandState()
		if err != nil {
			http.Error(w, "Failed to generate nonce", http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, &http.Cookie{Name: "state", Value: state, HttpOnly: true, Path: "/"})
		http.SetCookie(w, &http.Cookie{Name: "nonce", Value: nonce, HttpOnly: true, Path: "/"})

		authURL := oauth2Config.AuthCodeURL(state, oidc.Nonce(nonce))
		http.Redirect(w, r, authURL, http.StatusFound)
	})

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		stateCookie, err := r.Cookie("state")
		if err != nil || r.URL.Query().Get("state") != stateCookie.Value {
			http.Error(w, "Invalid state state parameter", http.StatusBadRequest)
			return
		}

		oauth2Token, err := oauth2Config.Exchange(ctx, r.URL.Query().Get("code"))
		if err != nil {
			http.Error(w, fmt.Sprintf("Token exchange failed: %v", err), http.StatusInternalServerError)
			return
		}

		rawIDToken, ok := oauth2Token.Extra("id_token").(string)
		if !ok {
			http.Error(w, "No id_token field in token response", http.StatusInternalServerError)
			return
		}

		idToken, err := verifier.Verify(ctx, rawIDToken)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to verify ID token: %v", err), http.StatusInternalServerError)
			return
		}
		nonceCookie, err := r.Cookie("nonce")
		if err != nil || idToken.Nonce != nonceCookie.Value {
			http.Error(w, "Invalid nonce parameter", http.StatusBadRequest)
			return
		}

		a.Token = oauth2Token.AccessToken
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Success. You may now close this tab.")
		select {
		case resultChan <- nil:
		default:
		}
	})

	// Create server
	server := &http.Server{
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		Handler:      mux,
	}

	// Always ensure server and listener clean up when Login exits
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case resultChan <- fmt.Errorf("server error: %w", err):
			default:
			}
		}
	}()

	logger.Info("starting login server: url=\"http://%s\"", listener.Addr())
	if a.Flags&OIDCAllowWebLogin != 0 {
		go func() {
			logger.Info("opening browser: url=%q", loginURL)
			// Give the server time to come up
			time.Sleep(1 * time.Second)
			if err := browser.OpenURL(loginURL); err != nil {
				logger.Error("browser failed to open: %w", err)
				fmt.Printf("\nVisit URL to login: %s\n", loginURL)
			}
		}()
	} else {
		logger.Info("\nVisit URL to login: %s\n", loginURL)
	}

	// Block until token exchange finishes OR parent context is canceled (e.g. Ctrl+C)
	select {
	case err := <-resultChan:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	if len(a.Token) == 0 {
		return errors.New("no token set after login")
	}

	return nil
}
