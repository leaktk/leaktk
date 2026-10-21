package auths

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/leaktk/leaktk/internal/auths/oidc"
)

var wwwAuthRealmRe = regexp.MustCompile(`(?i)\brealm\s*=\s*(?:"((?:[^"\\]|\\.)*)"|([^,\s]+))`)

const oidcDefaultClientID = "leaktk-cli"
const oidcDefaultResource = "leaktk-server"

type OIDCAuth struct {
	// TODO:
	// - change this to access token
	// - add refresh token field
	// - store and leverage refresh token if its present
	Token     string `toml:"token"`
	ClientID  string `toml:"client_id"`
	Issuer    string `toml:"issuer"`
	Resource  string `toml:"resource"`
	Autologin bool   `toml:"autologin"`
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

func (a *OIDCAuth) SetHeader(ctx context.Context, h http.Header) error {
	if len(a.Token) == 0 {
		if !a.Autologin {
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
	var creds oidc.Creds
	var cfg oidc.Config

	if len(a.ClientID) == 0 {
		a.ClientID = oidcDefaultClientID
	}
	if len(a.Resource) == 0 {
		a.Resource = oidcDefaultResource
	}
	if len(a.Issuer) == 0 {
		return errors.New("no issuer URL set")
	}

	cfg.ClientID = a.ClientID
	cfg.Issuer = a.Issuer
	cfg.Resource = a.Resource
	cfg.MaxTokenAge = 60 * 5 // 5 minutes

	err := oidc.LoginAuthCodePKCE(ctx, &creds, &cfg)
	if err != nil {
		return err
	}

	a.Token = creds.AccessToken
	if len(a.Token) == 0 {
		return errors.New("no token set after login")
	}

	return nil
}
