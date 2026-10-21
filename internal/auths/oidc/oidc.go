package oidc

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
	"slices"
	"strings"
	"time"

	"github.com/pkg/browser"

	"github.com/leaktk/leaktk/internal/httpclient"
	"github.com/leaktk/leaktk/pkg/logger"
)

const tokenRawLen = 32 // crypto.Sha256.Size
const tokenLen = 43    // base64.RawURLEncoding.EncodedLen(32)
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
var defaultScopes = []string{"openid", "profile", "offline_access"}

type LoginFlag int

type Token [tokenLen]byte

func (t *Token) String() string {
	return string(t[:])
}

type Config struct {
	Issuer                string   `json:"issuer"`
	TokenEndpoint         string   `json:"token_endpoint"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	JWKSURL               string   `json:"jwks_uri"`
	ScopesSupported       []string `json:"scopes_supported"`
	ClientID              string   `json:"-"`
	Resource              string   `json:"-"`
	RedirectURI           string   `json:"-"`
	MaxTokenAge           int64    `json:"-"`
}

type RefreshToken []byte
type IDToken struct {
	Value  []byte
	Header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	Claims struct {
		Iss      string `json:"iss"`
		Sub      string `json:"sub"`
		Aud      string `json:"aud"`
		Exp      int64  `json:"exp"`
		Iat      int64  `json:"iat"`
		AuthTime int64  `json:"auth_time"`
		Nonce    string `json:"nonce"`
		Acr      string `json:"acr"`
		Amr      string `json:"amr"`
		Azp      string `json:"azp"`
		Name     string `json:"name"`
		Jti      string `json:"jti"`
		AtHash   string `json:"at_hash"`
	}
}

func (t *IDToken) UnmarshalText(text []byte) error {
	t.Value = text

	headerB64, bodyAndSigB64, found := bytes.Cut(text, []byte("."))
	if !found || len(headerB64) == 0 {
		return errors.New("jwt unmarshal: missing header")
	}

	header := make([]byte, b64u.DecodedLen(len(headerB64)))
	if _, err := b64u.Decode(header, headerB64); err != nil {
		return fmt.Errorf("jwt unmarshal: header: %w", err)
	}

	if err := json.Unmarshal(header, &t.Header); err != nil {
		return fmt.Errorf("jwt unmarshal: header: %w", err)
	}

	bodyB64, sigB64, found := bytes.Cut(bodyAndSigB64, []byte("."))
	if !found || len(bodyB64) == 0 {
		return errors.New("jwt unmarshal: missing body")
	} else if len(sigB64) == 0 {
		return errors.New("jwt unmarshal: missing signature")
	}
	body := make([]byte, b64u.DecodedLen(len(bodyB64)))
	if _, err := b64u.Decode(body, bodyB64); err != nil {
		return fmt.Errorf("jwt unmarshal: body: %w", err)
	}
	if err := json.Unmarshal(body, &t.Claims); err != nil {
		return fmt.Errorf("jwt unmarshal: body: %w", err)
	}
	return nil
}

func (t *IDToken) VerifyAccessToken(at string) error {
	if t.Header.Alg != "RS256" {
		return errors.New("this client can only verify RS256 access tokens")
	}

	atHash := []byte(t.Claims.AtHash)
	if len(atHash) == 0 {
		return errors.New("the id_token missing at_hash claim")
	}

	b := []byte(at)
	h := sha256.Sum256(b)
	d := make([]byte, b64u.EncodedLen(sha256.Size/2))
	b64u.Encode(d, h[:sha256.Size/2])
	if !bytes.Equal(atHash, d) {
		return fmt.Errorf("invalid at_hash: expected=%q actual=%q", atHash, d)
	}

	return nil
}

type Creds struct {
	AccessToken  string       `json:"access_token"`
	IDToken      IDToken      `json:"id_token"`
	RefreshToken RefreshToken `json:"refresh_token"`
}

func s256dgst(dgst, data []byte) {
	h := sha256.Sum256(data)
	b64u.Encode(dgst, h[:])
}

func loadToken(token *Token) {
	var rawToken [tokenRawLen]byte
	rand.Read(rawToken[:])
	b64u.Encode(token[:], rawToken[:])
}

func FetchCreds(ctx context.Context, creds *Creds, cfg *Config, code string, codeVerifier *Token) error {
	logger.Debug("fetching tokens")
	values := make(url.Values)
	values.Set("client_id", cfg.ClientID)
	values.Set("code", code)
	values.Set("code_verifier", codeVerifier.String())
	values.Set("grant_type", "authorization_code")
	values.Set("redirect_uri", cfg.RedirectURI)

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

	d := json.NewDecoder(resp.Body)
	err = d.Decode(creds)
	if err != nil {
		return fmt.Errorf("decode token resp: %w", err)
	}

	return nil
}

// ValidateIDToken does some basic checks to make sure the ID token returned is
// valid.  It attempts to be somewhat compliant with the spec, but leans on the
// service that the access token is being sent to to actually validate the
// token is valid.  If using this to gate access to a resource, a more thorough
// implmentation is needed.
func ValidateIDToken(ctx context.Context, t *IDToken, cfg *Config, nonce *Token) error {
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
	if now-t.Claims.Iat > cfg.MaxTokenAge {
		logger.Debug("timing details: now=%d iat=%d max_age=%d", now, t.Claims.Iat, cfg.MaxTokenAge)
		return fmt.Errorf("token exceeded max allowed age: max_age_seconds=%d", cfg.MaxTokenAge)
	}

	// If a nonce value was sent in the Authentication Request, a nonce Claim
	// MUST be present and its value checked to verify that it is the same
	// value as the one that was sent in the Authentication Request. The Client
	// SHOULD check the nonce value for replay attacks. The precise method for
	// detecting replay attacks is Client specific.
	if t.Claims.Nonce != nonce.String() {
		return errors.New("invalid nonce")
	}

	// If the auth_time Claim was requested, either through a specific request
	// for this Claim or by using the max_age parameter, the Client SHOULD
	// check the auth_time Claim value and request re-authentication if it
	// determines too much time has elapsed since the last End-User
	// authentication.
	if t.Claims.AuthTime > 0 && now-t.Claims.AuthTime > cfg.MaxTokenAge {
		logger.Debug("timing details: now=%d auth_time=%d max_age=%d", now, t.Claims.AuthTime, cfg.MaxTokenAge)
		return fmt.Errorf("token exceeded max allowed age: max_age_seconds=%d", cfg.MaxTokenAge)
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

	d := json.NewDecoder(resp.Body)
	err = d.Decode(cfg)
	if err != nil {
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

func LoginAuthCodePKCE(ctx context.Context, creds *Creds, cfg *Config) error {
	var (
		loginURL      string
		codeChallenge Token
		codeVerifier  Token
		nonce         Token
		state         Token
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

	cfg.RedirectURI = fmt.Sprintf("http://%s/callback", l.Addr())
	loginURL = fmt.Sprintf("http://%s/login", l.Addr())

	// set up tokens
	loadToken(&nonce)
	loadToken(&state)
	loadToken(&codeVerifier)

	// derive code challenge
	s256dgst(codeChallenge[:], codeVerifier[:])

	// Channel to signal flow completion or failure back to Login()
	resultChan := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		var scopes []string
		for _, scope := range defaultScopes {
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
			url.QueryEscape(cfg.RedirectURI),
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

		defer func() {
			select {
			case resultChan <- err:
			default:
			}
		}()

		if !bytes.Equal(rState, state[:]) {
			err = errors.New("invalid state parameter")
			http.Error(w, err.Error(), http.StatusBadRequest)
		} else if err = FetchCreds(ctx, creds, cfg, rCode, &codeVerifier); err != nil {
			err = fmt.Errorf("token exchange failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		} else if len(creds.AccessToken) == 0 {
			err = errors.New("could not fetch access token")
			http.Error(w, err.Error(), http.StatusInternalServerError)
		} else if err = ValidateIDToken(ctx, &creds.IDToken, cfg, &nonce); err != nil {
			err = fmt.Errorf("validate id token failed: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		} else if err = creds.IDToken.VerifyAccessToken(creds.AccessToken); err != nil {
			err = fmt.Errorf("verify access token failed: %v", err)
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

	if len(creds.AccessToken) == 0 {
		return errors.New("no access token set after login")
	}

	return nil
}
