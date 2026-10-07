package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/leaktk/leaktk/pkg/logger"
)

const defaultClientID = "leaktk-cli"
const loginTimeout = 5 * time.Minute

type WWWAuthenticate struct {
	Realm    string
	Service  string
	Scope    string
	ClientID string
}

var wwwAuthParamRe = regexp.MustCompile(`([^\s"=,]+)\s*=\s*(?:"((?:[^"\\]|\\.)*)"|([^\s",]+))`)

func (a *WWWAuthenticate) UnmarshalText(text []byte) error {
	header := string(text)

	if len(header) == 0 {
		return errors.New("empty WWW-Authenticate header")
	}

	scheme, paramStr, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return fmt.Errorf("unsupported auth scheme: %q", scheme)
	}

	params := parseParams(paramStr)
	a.Realm = params["realm"]
	a.Service = params["service"]
	a.Scope = params["scope"]

	if len(a.Realm) == 0 {
		return errors.New("WWW-Authenticate header missing realm")
	}

	return nil
}

func ParseWWWAuthenticate(header string) (WWWAuthenticate, error) {
	var auth WWWAuthenticate
	err := auth.UnmarshalText([]byte(header))
	return auth, err
}

func parseParams(s string) map[string]string {
	params := make(map[string]string)
	for _, match := range wwwAuthParamRe.FindAllStringSubmatch(s, -1) {
		key := match[1]
		if match[2] != "" {
			// It matched the quoted group (Group 2)
			// Clean up escaped quotes (convert \" back to ")
			params[key] = strings.ReplaceAll(match[2], `\"`, `"`)
		} else {
			// It matched the unquoted group (Group 3)
			params[key] = match[3]
		}
	}
	return params
}

func OAuthConfig(ctx context.Context, client *http.Client, realm, clientID, redirectURI string) (*oauth2.Config, error) {
    if len(clientID) == 0 {
        clientID = defaultClientID
    }
    ctx = oidc.ClientContext(ctx, client)
    provider, err := oidc.NewProvider(ctx, realm)
    if err != nil {
        return nil, fmt.Errorf("OIDC discovery failed: %w", err)
    }
    return &oauth2.Config{
        ClientID:    clientID,
        Endpoint:    provider.Endpoint(),
        RedirectURL: redirectURI,
        Scopes:      []string{oidc.ScopeOpenID},
    }, nil
}

const maxRedirects = 10

func Challenge(ctx context.Context, client *http.Client, serverURL string) (*WWWAuthenticate, error) {
	noFollow := noRedirectClient(client)

	request, err := http.NewRequestWithContext(ctx, "GET", serverURL, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create challenge request: %w", err)
	}

	response, err := noFollow.Do(request)
	if err != nil {
		return nil, fmt.Errorf("challenge request failed: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode == http.StatusOK {
		return nil, nil
	}

	if response.StatusCode == http.StatusMovedPermanently || response.StatusCode == http.StatusFound ||
		response.StatusCode == http.StatusSeeOther || response.StatusCode == http.StatusTemporaryRedirect {
		return discoverFromRedirects(ctx, noFollow, response)
	}

	if response.StatusCode == http.StatusUnauthorized {
		header := response.Header.Get("WWW-Authenticate")
		if len(header) > 0 {
			auth, err := ParseWWWAuthenticate(header)
			if err == nil {
				return &auth, nil
			}
		}
		return nil, errors.New("server requires authentication but did not provide OAuth details; use --token instead")
	}

	return nil, fmt.Errorf("unexpected status from server: status_code=%d", response.StatusCode)
}

func noRedirectClient(client *http.Client) *http.Client {
	c := *client
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	return &c
}

func discoverFromRedirects(ctx context.Context, client *http.Client, resp *http.Response) (*WWWAuthenticate, error) {
	for i := 0; i < maxRedirects; i++ {
		location := resp.Header.Get("Location")
		if len(location) == 0 {
			break
		}

		locURL, err := url.Parse(location)
		if err != nil {
			break
		}

		if !locURL.IsAbs() {
			locURL = resp.Request.URL.ResolveReference(locURL)
		}

		if locURL.Query().Get("response_type") == "code" {
			realm := extractIssuer(locURL)
			if len(realm) > 0 {
				clientID := locURL.Query().Get("client_id")
				logger.Debug("discovered OAuth realm from redirect chain: realm=%q client_id=%q", realm, clientID)
				return &WWWAuthenticate{Realm: realm, ClientID: clientID}, nil
			}
		}

		req, err := http.NewRequestWithContext(ctx, "GET", locURL.String(), nil)
		if err != nil {
			break
		}

		_ = resp.Body.Close()
		resp, err = client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("error following redirect chain: %w", err)
		}

		if resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			return nil, nil
		}

		if resp.StatusCode == http.StatusUnauthorized {
			header := resp.Header.Get("WWW-Authenticate")
			_ = resp.Body.Close()
			if len(header) > 0 {
				auth, err := ParseWWWAuthenticate(header)
				if err == nil {
					return &auth, nil
				}
			}
		}

		if resp.StatusCode != http.StatusMovedPermanently && resp.StatusCode != http.StatusFound &&
			resp.StatusCode != http.StatusSeeOther && resp.StatusCode != http.StatusTemporaryRedirect {
			_ = resp.Body.Close()
			break
		}
	}

	return nil, errors.New("could not discover OAuth endpoint from server; use --token instead")
}

func extractIssuer(u *url.URL) string {
	issuer := *u
	issuer.RawQuery = ""
	issuer.Fragment = ""

	path := issuer.Path
	for _, suffix := range []string{
		"/protocol/openid-connect/auth",
		"/authorize",
	} {
		if strings.HasSuffix(path, suffix) {
			issuer.Path = strings.TrimSuffix(path, suffix)
			return issuer.String()
		}
	}

	issuer.Path = strings.TrimRight(path, "/")
	return issuer.String()
}

func ValidateToken(ctx context.Context, client *http.Client, serverURL, token string) error {
	request, err := http.NewRequestWithContext(ctx, "GET", serverURL, nil)
	if err != nil {
		return fmt.Errorf("could not create validation request: %w", err)
	}

	request.Header.Set("Authorization", "Bearer "+token)

	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("validation request failed: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("token validation failed: status_code=%d", response.StatusCode)
	}

	return nil
}

func WebLogin(ctx context.Context, client *http.Client, serverURL string) (string, error) {
	logger.Info("checking server authentication requirements: server=%q", serverURL)

	wwwAuth, err := Challenge(ctx, client, serverURL)
	if err != nil {
		return "", err
	}

	if wwwAuth == nil {
		logger.Info("server does not require authentication")
		return "", errors.New("server does not require authentication")
	}

	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", fmt.Errorf("could not generate state parameter: %w", err)
	}
	state := hex.EncodeToString(stateBytes)

	callbackCtx, callbackCancel := context.WithTimeout(ctx, loginTimeout)
	defer callbackCancel()

	addr, resultCh, shutdown, err := StartCallbackServer(callbackCtx, "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("could not start callback server: %w", err)
	}
	defer shutdown()

	redirectURI := "http://" + addr + "/callback"

	oauthCfg, err := OAuthConfig(ctx, client, wwwAuth.Realm, wwwAuth.ClientID, redirectURI)
	if err != nil {
		return "", fmt.Errorf("could not discover auth endpoints: %w", err)
	}

	verifier := oauth2.GenerateVerifier()
	authURL := oauthCfg.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))

	if err := OpenBrowser(authURL); err != nil {
		logger.Info("could not open browser automatically")
		fmt.Printf("Open your browser and visit:\n%s\n\n", authURL)
	}

	logger.Info("waiting for authentication...")

	oauthCtx := oidc.ClientContext(ctx, client)

	select {
	case result := <-resultCh:
		if len(result.Error) > 0 {
			return "", fmt.Errorf("authentication error: %s", result.Error)
		}

		if result.State != state {
			return "", errors.New("authentication failed: state parameter mismatch")
		}

		logger.Info("exchanging authorization code for token")
		oauthToken, err := oauthCfg.Exchange(oauthCtx, result.Code, oauth2.VerifierOption(verifier))
		if err != nil {
			return "", fmt.Errorf("token exchange failed: %w", err)
		}

		logger.Info("validating token")
		if err := ValidateToken(ctx, client, serverURL, oauthToken.AccessToken); err != nil {
			return "", err
		}

		return oauthToken.AccessToken, nil

	case <-callbackCtx.Done():
		return "", errors.New("authentication timed out; please try again")
	}
}
