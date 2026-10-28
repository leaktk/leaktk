package auths

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/leaktk/leaktk/internal/auths/oidcclient"
	"github.com/leaktk/leaktk/pkg/logger"
)

var wwwAuthRealmRe = regexp.MustCompile(`(?i)\brealm\s*=\s*(?:"((?:[^"\\]|\\.)*)"|([^,\s]+))`)

const oidcDefaultClientID = "leaktk-cli"
const oidcDefaultResource = "leaktk-server"
const oidcDefaultScope = "openid profile offline_access"

type OIDCAuth struct {
	ClientID     string `toml:"client_id"`
	Issuer       string `toml:"issuer"`
	Resource     string `toml:"resource"`
	Scope        string `toml:"scope"`
	AccessToken  string `toml:"access_token"`
	RefreshToken string `toml:"refresh_token"`
}

func (a *OIDCAuth) SetHeader(ctx context.Context, h http.Header) error {
	if len(a.AccessToken) != 0 {
		h.Set("Authorization", "Bearer "+a.AccessToken)
	} else {
		logger.Debug("set header: access token missing")
	}
	return nil
}

func (a *OIDCAuth) Load(d any) error {
	m, ok := d.(map[string]any)
	if !ok {
		return fmt.Errorf("could not load data: expected=\"map[string]any\" got=%T", d)
	}

	for k, v := range m {
		switch k {
		case "client_id":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("%s must be a string", k)
			}
			a.ClientID = s
		case "issuer":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("%s must be a string", k)
			}
			a.Issuer = s
		case "resource":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("%s must be a string", k)
			}
			a.Resource = s
		case "scope":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("%s must be a string", k)
			}
			a.Scope = s
		case "access_token":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("%s must be a string", k)
			}
			a.AccessToken = s
		case "refresh_token":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("%s must be a string", k)
			}
			a.RefreshToken = s
		}
	}

	return nil
}

func (a *OIDCAuth) Dump() map[string]string {
	m := map[string]string{
		"client_id":     a.ClientID,
		"issuer":        a.Issuer,
		"resource":      a.Resource,
		"scope":         a.Scope,
		"access_token":  a.AccessToken,
		"refresh_token": a.RefreshToken,
	}

	return m
}

func (a *OIDCAuth) Merge(o *OIDCAuth) error {
	if len(o.ClientID) != 0 {
		a.ClientID = o.ClientID
	}
	if len(o.Issuer) != 0 {
		a.Issuer = o.Issuer
	}
	if len(o.Resource) != 0 {
		a.Resource = o.Resource
	}
	if len(o.Scope) != 0 {
		a.Scope = o.Scope
	}
	if len(o.AccessToken) != 0 {
		a.AccessToken = o.AccessToken
	}
	if len(o.RefreshToken) != 0 {
		a.RefreshToken = o.RefreshToken
	}
	return nil
}

// FromHeader only supports the Bearer as the first header and first challenge
func (a *OIDCAuth) FromHeader(header string) error {
	if len(header) == 0 {
		return errors.New("header empty")
	}

	scheme, paramStr, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return fmt.Errorf("unsupported auth scheme: %q", scheme)
	}

	m := wwwAuthRealmRe.FindStringSubmatch(paramStr)
	if len(m) == 0 {
		if !strings.Contains(paramStr, "realm") {
			return errors.New("missing realm")
		} else {
			return errors.New("could not parse realm")
		}
	}

	// m[1] is if the string is quoted and m[2] is unquoted
	if len(m[1]) > 0 {
		var b strings.Builder
		escaped := false
		for _, r := range m[1] {
			if escaped {
				b.WriteRune(r)
				escaped = false
			} else if r == '\\' {
				escaped = true
			} else {
				b.WriteRune(r)
			}
		}
		a.Issuer = b.String()
	} else {
		a.Issuer = m[2]
	}

	if len(a.Issuer) == 0 {
		return errors.New("missing realm")
	}

	return nil
}

func (a *OIDCAuth) Login(ctx context.Context) error {
	var authTokens oidcclient.AuthTokens
	var cfg oidcclient.Config

	if len(a.Issuer) == 0 {
		return errors.New("no issuer URL set")
	}

	if len(a.ClientID) == 0 {
		a.ClientID = oidcDefaultClientID
		logger.Debug("using default client id: client_id=%q issuer=%q", a.ClientID, a.Issuer)
	}
	if len(a.Resource) == 0 {
		a.Resource = oidcDefaultResource
		logger.Debug("using default resource: resource=%q issuer=%q", a.Resource, a.Issuer)
	}
	if len(a.Scope) == 0 {
		a.Scope = oidcDefaultScope
		logger.Debug("oidc: using default scope: scope=%q issuer=%q", a.Scope, a.Issuer)
	}

	cfg.ClientID = a.ClientID
	cfg.Issuer = a.Issuer
	cfg.Resource = a.Resource
	for _, s := range strings.Split(a.Scope, " ") {
		s = strings.TrimSpace(s)
		if len(s) > 0 {
			cfg.Scopes = append(cfg.Scopes, s)
		}
	}

	err := oidcclient.LoginAuthCodePKCE(ctx, &authTokens, &cfg)
	if err != nil {
		return err
	}

	a.AccessToken = authTokens.AccessToken
	a.RefreshToken = authTokens.RefreshToken

	if len(a.AccessToken) == 0 {
		return errors.New("no access token set after login")
	}

	return nil
}
