package auths

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/leaktk/leaktk/pkg/logger"
)

const oauthMaxRedirects = 10
const oauthLoginTimeout = 5 * time.Minute

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
		return nil, errors.New("server requires authentication but did not provide OIDC details; use --token instead")
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
				logger.Debug("discovered OIDC realm from redirect chain: realm=%q client_id=%q", realm, clientID)
				return &WWWAuthenticate{Realm: realm, ClientID: clientID}, nil
			}
		}

		req, err := http.NewRequestWithContext(ctx, "GET", locURL.String(), nil)
		if err != nil {
			break
		}

		_ = resp.Body.Close()
		resp, err = client.Do(req) // codeql[go/request-forgery] server URL is configured by user
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

	return nil, errors.New("could not discover OIDC endpoint from server; use --token instead")
}
