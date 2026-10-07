package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseWWWAuthenticate(t *testing.T) {
	t.Run("FullHeader", func(t *testing.T) {
		auth, err := ParseWWWAuthenticate(`Bearer realm="https://auth.example.com/token",service="registry.example.com",scope="repository:foo:pull"`)
		require.NoError(t, err)
		assert.Equal(t, "https://auth.example.com/token", auth.Realm)
		assert.Equal(t, "registry.example.com", auth.Service)
		assert.Equal(t, "repository:foo:pull", auth.Scope)
	})

	t.Run("RealmOnly", func(t *testing.T) {
		auth, err := ParseWWWAuthenticate(`Bearer realm="https://auth.example.com"`)
		require.NoError(t, err)
		assert.Equal(t, "https://auth.example.com", auth.Realm)
		assert.Empty(t, auth.Service)
		assert.Empty(t, auth.Scope)
	})

	t.Run("CaseInsensitiveScheme", func(t *testing.T) {
		auth, err := ParseWWWAuthenticate(`bearer realm="https://auth.example.com"`)
		require.NoError(t, err)
		assert.Equal(t, "https://auth.example.com", auth.Realm)
	})

	t.Run("EmptyHeader", func(t *testing.T) {
		_, err := ParseWWWAuthenticate("")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty")
	})

	t.Run("UnsupportedScheme", func(t *testing.T) {
		_, err := ParseWWWAuthenticate("Basic realm=\"test\"")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported auth scheme")
	})

	t.Run("MissingRealm", func(t *testing.T) {
		_, err := ParseWWWAuthenticate(`Bearer service="test"`)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing realm")
	})

	t.Run("NoSpaceAfterScheme", func(t *testing.T) {
		_, err := ParseWWWAuthenticate("BearerNoParams")
		require.Error(t, err)
	})
}

func TestDiscoverEndpoints(t *testing.T) {
	ctx := context.Background()

	t.Run("ValidDiscovery", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/.well-known/openid-configuration", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(OIDCEndpoints{ // #nosec G101
				AuthorizationEndpoint: "https://sso.example.com/auth",
				TokenEndpoint:         "https://sso.example.com/token",
			})
		}))
		defer ts.Close()

		endpoints, err := DiscoverEndpoints(ctx, ts.Client(), ts.URL)
		require.NoError(t, err)
		assert.Equal(t, "https://sso.example.com/auth", endpoints.AuthorizationEndpoint)
		assert.Equal(t, "https://sso.example.com/token", endpoints.TokenEndpoint)
	})

	t.Run("DiscoveryNotFound", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer ts.Close()

		endpoints, err := DiscoverEndpoints(ctx, ts.Client(), ts.URL)
		require.NoError(t, err)
		assert.Equal(t, ts.URL, endpoints.AuthorizationEndpoint)
		assert.Equal(t, ts.URL, endpoints.TokenEndpoint)
	})

	t.Run("MalformedJSON", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprint(w, "not json")
		}))
		defer ts.Close()

		endpoints, err := DiscoverEndpoints(ctx, ts.Client(), ts.URL)
		require.NoError(t, err)
		assert.Equal(t, ts.URL, endpoints.AuthorizationEndpoint)
		assert.Equal(t, ts.URL, endpoints.TokenEndpoint)
	})

	t.Run("EmptyEndpoints", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(OIDCEndpoints{})
		}))
		defer ts.Close()

		endpoints, err := DiscoverEndpoints(ctx, ts.Client(), ts.URL)
		require.NoError(t, err)
		assert.Equal(t, ts.URL, endpoints.AuthorizationEndpoint)
		assert.Equal(t, ts.URL, endpoints.TokenEndpoint)
	})
}

func TestChallenge(t *testing.T) {
	ctx := context.Background()

	t.Run("ServerOK", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		auth, err := Challenge(ctx, ts.Client(), ts.URL)
		require.NoError(t, err)
		assert.Nil(t, auth)
	})

	t.Run("Unauthorized", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="https://auth.example.com"`)
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer ts.Close()

		auth, err := Challenge(ctx, ts.Client(), ts.URL)
		require.NoError(t, err)
		require.NotNil(t, auth)
		assert.Equal(t, "https://auth.example.com", auth.Realm)
	})

	t.Run("UnauthorizedNoHeader", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer ts.Close()

		_, err := Challenge(ctx, ts.Client(), ts.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "use --token instead")
	})

	t.Run("ServerError", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer ts.Close()

		_, err := Challenge(ctx, ts.Client(), ts.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "500")
	})

	t.Run("RedirectToOAuth", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/":
				http.Redirect(w, r, "/token", http.StatusFound)
			case "/token":
				http.Redirect(w, r, "/oauth2/callback", http.StatusFound)
			case "/oauth2/callback":
				authURL := "https://sso.example.com/auth/realms/MyRealm/protocol/openid-connect/auth?response_type=code&client_id=my-server-client"
				http.Redirect(w, r, authURL, http.StatusFound)
			}
		}))
		defer ts.Close()

		auth, err := Challenge(ctx, ts.Client(), ts.URL)
		require.NoError(t, err)
		require.NotNil(t, auth)
		assert.Equal(t, "https://sso.example.com/auth/realms/MyRealm", auth.Realm)
		assert.Equal(t, "my-server-client", auth.ClientID)
	})

	t.Run("NegotiateScheme", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("WWW-Authenticate", "Negotiate")
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer ts.Close()

		_, err := Challenge(ctx, ts.Client(), ts.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "use --token instead")
	})

	t.Run("RedirectToNonOAuth", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/":
				http.Redirect(w, r, "/other", http.StatusFound)
			case "/other":
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer ts.Close()

		_, err := Challenge(ctx, ts.Client(), ts.URL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "could not discover")
	})

	t.Run("RedirectToAuthorize", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authURL := "https://idp.example.com/authorize?response_type=code&client_id=test"
			http.Redirect(w, r, authURL, http.StatusFound)
		}))
		defer ts.Close()

		auth, err := Challenge(ctx, ts.Client(), ts.URL)
		require.NoError(t, err)
		require.NotNil(t, auth)
		assert.Equal(t, "https://idp.example.com", auth.Realm)
	})
}

func TestGeneratePKCE(t *testing.T) {
	pkce, err := GeneratePKCE()
	require.NoError(t, err)
	assert.NotEmpty(t, pkce.Verifier)
	assert.NotEmpty(t, pkce.Challenge)
	assert.NotEqual(t, pkce.Verifier, pkce.Challenge)

	pkce2, err := GeneratePKCE()
	require.NoError(t, err)
	assert.NotEqual(t, pkce.Verifier, pkce2.Verifier)
}

func TestBuildAuthURL(t *testing.T) {
	t.Run("BasicURL", func(t *testing.T) {
		result := BuildAuthURL("https://auth.example.com/authorize", "http://localhost:8080/callback", "abc123", "", nil)
		assert.Contains(t, result, "https://auth.example.com/authorize?")
		assert.Contains(t, result, "response_type=code")
		assert.Contains(t, result, "client_id=leaktk-cli")
		assert.Contains(t, result, "redirect_uri=")
		assert.Contains(t, result, "state=abc123")
		assert.NotContains(t, result, "code_challenge")
	})

	t.Run("CustomClientID", func(t *testing.T) {
		result := BuildAuthURL("https://auth.example.com", "http://localhost/cb", "state", "my-client", nil)
		assert.Contains(t, result, "client_id=my-client")
	})

	t.Run("EndpointWithExistingParams", func(t *testing.T) {
		result := BuildAuthURL("https://auth.example.com?foo=bar", "http://localhost/cb", "state", "", nil)
		assert.Contains(t, result, "https://auth.example.com?foo=bar&")
		assert.Contains(t, result, "response_type=code")
	})

	t.Run("WithPKCE", func(t *testing.T) {
		pkce := &PKCEChallenge{Verifier: "test-verifier", Challenge: "test-challenge"}
		result := BuildAuthURL("https://auth.example.com/authorize", "http://localhost/cb", "state", "", pkce)
		assert.Contains(t, result, "code_challenge=test-challenge")
		assert.Contains(t, result, "code_challenge_method=S256")
	})
}

func TestExchangeCode(t *testing.T) {
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "POST", r.Method)
			assert.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

			body, _ := io.ReadAll(r.Body)
			assert.Contains(t, string(body), "grant_type=authorization_code")
			assert.Contains(t, string(body), "code=test-code")

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"access_token": "test-token-123",
			})
		}))
		defer ts.Close()

		token, err := ExchangeCode(ctx, ts.Client(), ts.URL, "test-code", "http://localhost/cb", "", "")
		require.NoError(t, err)
		assert.Equal(t, "test-token-123", token)
	})

	t.Run("WithCodeVerifier", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			assert.Contains(t, string(body), "code_verifier=my-verifier")

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"access_token": "pkce-token",
			})
		}))
		defer ts.Close()

		token, err := ExchangeCode(ctx, ts.Client(), ts.URL, "code", "http://localhost/cb", "", "my-verifier")
		require.NoError(t, err)
		assert.Equal(t, "pkce-token", token)
	})

	t.Run("ServerError", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(w, `{"error":"invalid_grant"}`)
		}))
		defer ts.Close()

		_, err := ExchangeCode(ctx, ts.Client(), ts.URL, "bad-code", "http://localhost/cb", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "400")
	})

	t.Run("TokenError", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":             "invalid_client",
				"error_description": "bad client",
			})
		}))
		defer ts.Close()

		_, err := ExchangeCode(ctx, ts.Client(), ts.URL, "code", "http://localhost/cb", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid_client")
	})

	t.Run("EmptyAccessToken", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{})
		}))
		defer ts.Close()

		_, err := ExchangeCode(ctx, ts.Client(), ts.URL, "code", "http://localhost/cb", "", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "did not contain")
	})
}

func TestValidateToken(t *testing.T) {
	ctx := context.Background()

	t.Run("Valid", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "Bearer good-token", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		err := ValidateToken(ctx, ts.Client(), ts.URL, "good-token")
		require.NoError(t, err)
	})

	t.Run("Invalid", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer ts.Close()

		err := ValidateToken(ctx, ts.Client(), ts.URL, "bad-token")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "401")
	})
}
