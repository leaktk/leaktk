package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

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

type OIDCEndpoints struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
}

func ParseWWWAuthenticate(header string) (WWWAuthenticate, error) {
	var auth WWWAuthenticate

	if len(header) == 0 {
		return auth, errors.New("empty WWW-Authenticate header")
	}

	scheme, params, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return auth, fmt.Errorf("unsupported auth scheme: %q", scheme)
	}

	for _, pair := range splitParams(params) {
		key, value, found := strings.Cut(pair, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, "\"")

		switch strings.ToLower(key) {
		case "realm":
			auth.Realm = value
		case "service":
			auth.Service = value
		case "scope":
			auth.Scope = value
		}
	}

	if len(auth.Realm) == 0 {
		return auth, errors.New("WWW-Authenticate header missing realm")
	}

	return auth, nil
}

func splitParams(s string) []string {
	var parts []string
	var current strings.Builder
	inQuotes := false

	for _, r := range s {
		switch {
		case r == '"':
			inQuotes = !inQuotes
			current.WriteRune(r)
		case r == ',' && !inQuotes:
			parts = append(parts, current.String())
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

func DiscoverEndpoints(ctx context.Context, client *http.Client, realm string) (*OIDCEndpoints, error) {
	discoveryURL := strings.TrimRight(realm, "/") + "/.well-known/openid-configuration"

	request, err := http.NewRequestWithContext(ctx, "GET", discoveryURL, nil)
	if err != nil {
		return fallbackEndpoints(realm), nil
	}

	response, err := client.Do(request)
	if err != nil {
		return fallbackEndpoints(realm), nil
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return fallbackEndpoints(realm), nil
	}

	var endpoints OIDCEndpoints
	if err := json.NewDecoder(response.Body).Decode(&endpoints); err != nil {
		return fallbackEndpoints(realm), nil
	}

	if len(endpoints.AuthorizationEndpoint) == 0 || len(endpoints.TokenEndpoint) == 0 {
		return fallbackEndpoints(realm), nil
	}

	return &endpoints, nil
}

func fallbackEndpoints(realm string) *OIDCEndpoints {
	return &OIDCEndpoints{
		AuthorizationEndpoint: realm,
		TokenEndpoint:         realm,
	}
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
	return &http.Client{
		Transport: client.Transport,
		Timeout:   client.Timeout,
		Jar:       client.Jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
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
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode == http.StatusOK {
			return nil, nil
		}

		if resp.StatusCode == http.StatusUnauthorized {
			header := resp.Header.Get("WWW-Authenticate")
			if len(header) > 0 {
				auth, err := ParseWWWAuthenticate(header)
				if err == nil {
					return &auth, nil
				}
			}
		}

		if resp.StatusCode != http.StatusMovedPermanently && resp.StatusCode != http.StatusFound &&
			resp.StatusCode != http.StatusSeeOther && resp.StatusCode != http.StatusTemporaryRedirect {
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

// PKCEChallenge holds the PKCE code verifier and challenge pair.
type PKCEChallenge struct {
	Verifier  string
	Challenge string
}

// GeneratePKCE creates a PKCE code verifier and its S256 challenge.
func GeneratePKCE() (*PKCEChallenge, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("could not generate PKCE verifier: %w", err)
	}

	verifier := base64.RawURLEncoding.EncodeToString(buf)
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])

	return &PKCEChallenge{Verifier: verifier, Challenge: challenge}, nil
}

func BuildAuthURL(authEndpoint, redirectURI, state, clientID string, pkce *PKCEChallenge) string {
	if len(clientID) == 0 {
		clientID = defaultClientID
	}

	params := url.Values{
		"response_type": {"code"},
		"client_id":     {clientID},
		"redirect_uri":  {redirectURI},
		"state":         {state},
		"scope":         {"openid"},
	}

	if pkce != nil {
		params.Set("code_challenge", pkce.Challenge)
		params.Set("code_challenge_method", "S256")
	}

	sep := "?"
	if strings.Contains(authEndpoint, "?") {
		sep = "&"
	}

	return authEndpoint + sep + params.Encode()
}

func ExchangeCode(ctx context.Context, client *http.Client, tokenEndpoint, code, redirectURI, clientID, codeVerifier string) (string, error) {
	if len(clientID) == 0 {
		clientID = defaultClientID
	}

	data := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {redirectURI},
		"client_id":    {clientID},
	}

	if len(codeVerifier) > 0 {
		data.Set("code_verifier", codeVerifier)
	}

	request, err := http.NewRequestWithContext(ctx, "POST", tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("could not create token request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("could not read token response: %w", err)
	}

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned status %d: %s", response.StatusCode, string(body))
	}

	var tokenResponse struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}

	if err := json.Unmarshal(body, &tokenResponse); err != nil {
		return "", fmt.Errorf("could not parse token response: %w", err)
	}

	if len(tokenResponse.Error) > 0 {
		return "", fmt.Errorf("token error: %s: %s", tokenResponse.Error, tokenResponse.Description)
	}

	if len(tokenResponse.AccessToken) == 0 {
		return "", errors.New("token response did not contain an access token")
	}

	return tokenResponse.AccessToken, nil
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

	endpoints, err := DiscoverEndpoints(ctx, client, wwwAuth.Realm)
	if err != nil {
		return "", fmt.Errorf("could not discover auth endpoints: %w", err)
	}

	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", fmt.Errorf("could not generate state parameter: %w", err)
	}
	state := hex.EncodeToString(stateBytes)

	pkce, err := GeneratePKCE()
	if err != nil {
		return "", err
	}

	callbackCtx, callbackCancel := context.WithTimeout(ctx, loginTimeout)
	defer callbackCancel()

	addr, resultCh, shutdown, err := StartCallbackServer(callbackCtx, "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("could not start callback server: %w", err)
	}
	defer shutdown()

	redirectURI := "http://" + addr + "/callback"
	authURL := BuildAuthURL(endpoints.AuthorizationEndpoint, redirectURI, state, "", pkce)

	if err := OpenBrowser(authURL); err != nil {
		logger.Info("could not open browser automatically")
		fmt.Printf("Open your browser and visit:\n%s\n\n", authURL)
	}

	logger.Info("waiting for authentication...")

	select {
	case result := <-resultCh:
		if len(result.Error) > 0 {
			return "", fmt.Errorf("authentication error: %s", result.Error)
		}

		if result.State != state {
			return "", errors.New("authentication failed: state parameter mismatch")
		}

		logger.Info("exchanging authorization code for token")
		token, err := ExchangeCode(ctx, client, endpoints.TokenEndpoint, result.Code, redirectURI, "", pkce.Verifier)
		if err != nil {
			return "", err
		}

		logger.Info("validating token")
		if err := ValidateToken(ctx, client, serverURL, token); err != nil {
			return "", err
		}

		return token, nil

	case <-callbackCtx.Done():
		return "", errors.New("authentication timed out; please try again")
	}
}
