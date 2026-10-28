package oidcclient

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/pkg/browser"

	"github.com/leaktk/leaktk/internal/httpclient"
	"github.com/leaktk/leaktk/pkg/logger"
)

const tokenRawLen = 32    // crypto.Sha256.Size
const tokenLen = 43       // base64.RawURLEncoding.EncodedLen(32)
const tokenMaxIatAge = 60 // 1 minute
const paramsTempl = "client_id=%s" +
	"&code_challenge=%s" +
	"&code_challenge_method=S256" +
	"&nonce=%s" +
	"&redirect_uri=%s" +
	"&resource=%s" +
	"&response_type=code" +
	"&scope=%s" +
	"&state=%s"

var b64u = base64.RawURLEncoding
var cleanErrRe = regexp.MustCompile(`[<>]+`)

type AuthReqToken [tokenLen]byte

func (t *AuthReqToken) String() string {
	return string(t[:])
}

type Config struct {
	// Provided values
	ClientID string   `json:"-"`
	Resource string   `json:"-"`
	Scopes   []string `json:"-"`

	// Provided then updated from fetch values
	Issuer string `json:"issuer"`

	// Fetched values
	TokenEndpoint         string   `json:"token_endpoint"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	JWKSURL               string   `json:"jwks_uri"`
	ScopesSupported       []string `json:"scopes_supported"`
}

type AuthTokens struct {
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token"`
	IDToken      IDToken `json:"id_token"`
}

func s256dgst(dgst, data []byte) {
	h := sha256.Sum256(data)
	b64u.Encode(dgst, h[:])
}

func loadAuthReqToken(token *AuthReqToken) {
	var rawToken [tokenRawLen]byte
	rand.Read(rawToken[:])
	b64u.Encode(token[:], rawToken[:])
}

func FetchTokens(ctx context.Context, authTokens *AuthTokens, cfg *Config, code, redirectURI string, codeVerifier *AuthReqToken) error {
	logger.Debug("fetching tokens")
	values := make(url.Values)
	values.Set("client_id", cfg.ClientID)
	values.Set("code", code)
	values.Set("code_verifier", codeVerifier.String())
	values.Set("grant_type", "authorization_code")
	values.Set("redirect_uri", redirectURI)

	client := httpclient.NewClient()
	body := strings.NewReader(values.Encode())
	req, err := http.NewRequestWithContext(ctx, "POST", cfg.TokenEndpoint, body)
	if err != nil {
		return fmt.Errorf("request token: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request token: unexpected status code: status_code=%d", resp.StatusCode)
	}

	if err = json.NewDecoder(resp.Body).Decode(authTokens); err != nil {
		return fmt.Errorf("decode token resp: %w", err)
	}

	return nil
}

// ValidateIDToken does some basic checks to make sure the ID token returned is
// valid.  It attempts to be somewhat compliant with the spec, but leans on the
// service that the access token is being sent to to actually validate the
// token is valid.  If using this to gate access to a resource, a more thorough
// implmentation is needed.
func ValidateIDToken(ctx context.Context, t *IDToken, cfg *Config, nonce *AuthReqToken) error {
	logger.Debug("validating token")
	now := time.Now().Unix()

	if len(t.Value) == 0 {
		return errors.New("missing value")
	}

	// The Issuer Identifier for the OpenID Provider (which is typically obtained
	// during Discovery) MUST exactly match the value of the iss (issuer) Claim.
	if t.Claims.Iss != cfg.Issuer {
		return fmt.Errorf("invalid issuer: expected=%q actual=%q", cfg.Issuer, t.Claims.Iss)
	}

	// The Client MUST validate that the aud (audience) Claim contains its
	// client_id value registered at the Issuer identified by the iss (issuer)
	// Claim as an audience. The aud (audience) Claim MAY contain an array with
	// more than one element. The ID Token MUST be rejected if the ID Token
	// does not list the Client as a valid audience, or if it contains
	// additional audiences not trusted by the Client.
	aud := t.Claims.Aud
	if aud != cfg.ClientID && aud != fmt.Sprintf("[%q]", cfg.ClientID) {
		return errors.New("aud does not match client id")
	}

	// The alg value SHOULD be the default of RS256 or the algorithm sent by
	// the Client in the id_token_signed_response_alg parameter during
	// Registration.
	if t.Header.Alg != "RS256" {
		return errors.New("this client only supports RS256 signed tokens")
	}

	// The current time MUST be before the time represented by the exp Claim.
	if now > t.Claims.Exp {
		return errors.New("token is expired")
	}
	// The iat Claim can be used to reject tokens that were issued too far away
	// from the current time, limiting the amount of time that nonces need to
	// be stored to prevent attacks. The acceptable range is Client specific.
	if now-t.Claims.Iat > tokenMaxIatAge {
		logger.Debug("timing details: now=%d iat=%d max_iat_age=%d", now, t.Claims.Iat, tokenMaxIatAge)
		return fmt.Errorf("id token issued more than max %d seconds ago", tokenMaxIatAge)
	}

	// If a nonce value was sent in the Authentication Request, a nonce Claim
	// MUST be present and its value checked to verify that it is the same
	// value as the one that was sent in the Authentication Request. The Client
	// SHOULD check the nonce value for replay attacks. The precise method for
	// detecting replay attacks is Client specific.
	if t.Claims.Nonce != nonce.String() {
		return errors.New("invalid nonce")
	}

	// Verify signature
	var jwks JWKS
	if err := FetchJWKs(ctx, &jwks, cfg.JWKSURL); err != nil {
		return fmt.Errorf("fetch jwks: %w", err)
	}

	for _, jwk := range jwks.Keys {
		if jwk.Kid == t.Header.Kid && jwk.Alg == "RS256" && jwk.Use == "sig" {
			logger.Debug("matching JWK found: %s", jwk.String())
			if err := jwk.VerifyRS256(t.Value); err != nil {
				return fmt.Errorf("jwt verify: %w", err)
			}
			return nil
		}
	}

	return fmt.Errorf("token kid not found in jwks: kid=%q", t.Header.Kid)
}

func FetchConfig(ctx context.Context, cfg *Config) error {
	logger.Debug("fetching well-known openid-configuration")
	client := httpclient.NewClient()
	req, err := http.NewRequestWithContext(ctx, "GET", cfg.Issuer+"/.well-known/openid-configuration", nil)
	if err != nil {
		return fmt.Errorf("fetch config: create request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch config: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch config: unexpected status code: status_code=%d", resp.StatusCode)
	}

	if err = json.NewDecoder(resp.Body).Decode(cfg); err != nil {
		return fmt.Errorf("decode config resp: %w", err)
	}

	if len(cfg.TokenEndpoint) == 0 {
		return errors.New("verify config: token_endpoint missing")
	}

	if len(cfg.AuthorizationEndpoint) == 0 {
		return errors.New("verify config: authorization_endpoint missing")
	}

	if len(cfg.JWKSURL) == 0 {
		return errors.New("verify config: jwks_uri missing")
	}

	return nil
}

func LoginAuthCodePKCE(ctx context.Context, authTokens *AuthTokens, cfg *Config) error {
	var (
		loginURL      string
		redirectURI   string
		codeChallenge AuthReqToken
		codeVerifier  AuthReqToken
		nonce         AuthReqToken
		state         AuthReqToken
		err           error
		l             net.Listener
	)

	err = FetchConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("load openid config: %w", err)
	}

	l, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer func() { _ = l.Close() }()

	redirectURI = fmt.Sprintf("http://%s/callback", l.Addr())
	loginURL = fmt.Sprintf("http://%s/login", l.Addr())

	loadAuthReqToken(&nonce)
	loadAuthReqToken(&state)
	loadAuthReqToken(&codeVerifier)

	// derive code challenge
	s256dgst(codeChallenge[:], codeVerifier[:])

	// Channel to signal flow completion or failure back to Login()
	resultChan := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		var scopes []string
		for _, scope := range cfg.Scopes {
			if slices.Contains(cfg.ScopesSupported, scope) {
				scopes = append(scopes, scope)
			} else {
				logger.Debug("ignoring unsupported scope: scope=%q", scope)
			}
		}
		params := fmt.Sprintf(
			paramsTempl,
			url.QueryEscape(cfg.ClientID),
			url.QueryEscape(codeChallenge.String()),
			url.QueryEscape(nonce.String()),
			url.QueryEscape(redirectURI),
			url.QueryEscape(cfg.Resource),
			url.QueryEscape(strings.Join(scopes, " ")),
			url.QueryEscape(state.String()),
		)
		authURL := fmt.Sprintf("%s?%s", cfg.AuthorizationEndpoint, params)
		logger.Debug("auth url: %v", authURL)
		http.Redirect(w, r, authURL, http.StatusFound)
	})

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		var err error
		rQuery := r.URL.Query()
		rState := []byte(rQuery.Get("state"))
		rCode := rQuery.Get("code")
		rErrCode := cleanErrRe.ReplaceAllString(rQuery.Get("error"), "")
		rErrDesc := cleanErrRe.ReplaceAllString(rQuery.Get("error_description"), "")

		defer func() {
			select {
			case resultChan <- err:
			default:
			}
		}()

		if !bytes.Equal(rState, state[:]) {
			err = errors.New("invalid state parameter")
			http.Error(w, err.Error(), http.StatusBadRequest)
		} else if len(rErrCode) != 0 {
			err = fmt.Errorf("server error response: code=%q message=%q", rErrCode, rErrDesc)
			http.Error(w, err.Error(), http.StatusBadRequest)
		} else if err = FetchTokens(ctx, authTokens, cfg, rCode, redirectURI, &codeVerifier); err != nil {
			err = fmt.Errorf("token exchange failed: %w", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		} else if len(authTokens.AccessToken) == 0 {
			err = errors.New("could not fetch access token")
			http.Error(w, err.Error(), http.StatusInternalServerError)
		} else if err = ValidateIDToken(ctx, &authTokens.IDToken, cfg, &nonce); err != nil {
			err = fmt.Errorf("validate id token failed: %w", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		} else if err = authTokens.IDToken.VerifyAccessToken(authTokens.AccessToken); err != nil {
			err = fmt.Errorf("verify access token failed: %w", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		} else {
			_, _ = fmt.Fprintf(w, "Success. You may now close this tab.")
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
		if err := server.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case resultChan <- fmt.Errorf("server error: %w", err):
			default:
			}
		}
	}()

	logger.Debug("starting login server: url=\"http://%s\"", l.Addr())
	go func() {
		logger.Info("opening browser: url=%q", loginURL)
		time.Sleep(1 * time.Second)
		if err := browser.OpenURL(loginURL); err != nil {
			logger.Debug("browser failed to open: %v", err)
		}
		fmt.Printf("\nIf the browser doesn't open, visit:\n\n%s\n", loginURL)
	}()

	select {
	case err := <-resultChan:
		if err != nil {
			return err
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	if len(authTokens.AccessToken) == 0 {
		return errors.New("no access token set after login")
	}

	return nil
}
