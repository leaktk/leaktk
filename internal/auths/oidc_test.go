package auths

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOIDCAuth(t *testing.T) {
	t.Run("UnmarshalText", func(t *testing.T) {
		t.Run("FullHeader", func(t *testing.T) {
			a := OIDCAuth{}
			err := a.UnmarshalText([]byte(`Bearer realm="https://auth.example.com/",scope="openid email"`))
			require.NoError(t, err)
			assert.Equal(t, "https://auth.example.com/", a.IssuerURL)
			assert.Equal(t, "openid email", a.Scope)
		})

		t.Run("RealmOnly", func(t *testing.T) {
			a := OIDCAuth{}
			err := a.UnmarshalText([]byte(`Bearer realm="https://auth.example.com"`))
			require.NoError(t, err)
			assert.Equal(t, "https://auth.example.com", auth.Realm)
			assert.Equal(t, "openid", auth.Scope)
		})

		t.Run("CaseInsensitiveScheme", func(t *testing.T) {
			a := OIDCAuth{}
			err := a.UnmarshalText([]byte(`bearer realm="https://auth.example.com"`))
			require.NoError(t, err)
			assert.Equal(t, "https://auth.example.com", auth.Realm)
		})

		t.Run("EmptyHeader", func(t *testing.T) {
			a := OIDCAuth{}
			err := a.UnmarshalText([]byte{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "empty")
		})

		t.Run("UnsupportedScheme", func(t *testing.T) {
			a := OIDCAuth{}
			err := a.UnmarshalText([]byte(`basic realm="test"`))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unsupported auth scheme")
		})

		t.Run("MissingRealm", func(t *testing.T) {
			a := OIDCAuth{}
			err := a.UnmarshalText([]byte(`Bearer service="test"`))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "missing realm")
		})

		t.Run("NoSpaceAfterScheme", func(t *testing.T) {
			a := OIDCAuth{}
			err := a.UnmarshalText([]byte(`BearerNoParams`))
			_, err := ParseWWWAuthenticate("BearerNoParams")
			require.Error(t, err)
		})
	})

	// t.Run("(t *testing.T) {
	// 	ctx := context.Background()

	// 	t.Run("ValidDiscovery", func(t *testing.T) {
	// 		var serverURL string
	// 		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// 			w.Header().Set("Content-Type", "application/json")
	// 			_ = json.NewEncoder(w).Encode(map[string]string{ // #nosec G101
	// 				"issuer":                 serverURL,
	// 				"authorization_endpoint": "https://sso.example.com/auth",
	// 				"token_endpoint":         "https://sso.example.com/token",
	// 			})
	// 		}))
	// 		defer ts.Close()
	// 		serverURL = ts.URL

	// 		cfg, err := OAuthConfig(ctx, ts.Client(), ts.URL, "", "http://localhost/callback")
	// 		require.NoError(t, err)
	// 		assert.Equal(t, "https://sso.example.com/auth", cfg.Endpoint.AuthURL)
	// 		assert.Equal(t, "https://sso.example.com/token", cfg.Endpoint.TokenURL)
	// 		assert.Equal(t, defaultClientID, cfg.ClientID)
	// 		assert.Equal(t, "http://localhost/callback", cfg.RedirectURL)
	// 	})

	// 	t.Run("CustomClientID", func(t *testing.T) {
	// 		var serverURL string
	// 		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// 			w.Header().Set("Content-Type", "application/json")
	// 			_ = json.NewEncoder(w).Encode(map[string]string{ // #nosec G101
	// 				"issuer":                 serverURL,
	// 				"authorization_endpoint": "https://sso.example.com/auth",
	// 				"token_endpoint":         "https://sso.example.com/token",
	// 			})
	// 		}))
	// 		defer ts.Close()
	// 		serverURL = ts.URL

	// 		cfg, err := OAuthConfig(ctx, ts.Client(), ts.URL, "my-client", "http://localhost/callback")
	// 		require.NoError(t, err)
	// 		assert.Equal(t, "my-client", cfg.ClientID)
	// 	})

	// 	t.Run("DiscoveryFailure", func(t *testing.T) {
	// 		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// 			w.WriteHeader(http.StatusNotFound)
	// 		}))
	// 		defer ts.Close()

	// 		_, err := OAuthConfig(ctx, ts.Client(), ts.URL, "", "http://localhost/callback")
	// 		require.Error(t, err)
	// 		assert.Contains(t, err.Error(), "OIDC discovery failed")
	// 	})
	// }

	// func TestChallenge(t *testing.T) {
	// 	ctx := context.Background()

	// 	t.Run("ServerOK", func(t *testing.T) {
	// 		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// 			w.WriteHeader(http.StatusOK)
	// 		}))
	// 		defer ts.Close()

	// 		auth, err := Challenge(ctx, ts.Client(), ts.URL)
	// 		require.NoError(t, err)
	// 		assert.Nil(t, auth)
	// 	})

	// 	t.Run("Unauthorized", func(t *testing.T) {
	// 		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// 			w.Header().Set("WWW-Authenticate", `Bearer realm="https://auth.example.com"`)
	// 			w.WriteHeader(http.StatusUnauthorized)
	// 		}))
	// 		defer ts.Close()

	// 		auth, err := Challenge(ctx, ts.Client(), ts.URL)
	// 		require.NoError(t, err)
	// 		require.NotNil(t, auth)
	// 		assert.Equal(t, "https://auth.example.com", auth.Realm)
	// 	})

	// 	t.Run("UnauthorizedNoHeader", func(t *testing.T) {
	// 		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// 			w.WriteHeader(http.StatusUnauthorized)
	// 		}))
	// 		defer ts.Close()

	// 		_, err := Challenge(ctx, ts.Client(), ts.URL)
	// 		require.Error(t, err)
	// 		assert.Contains(t, err.Error(), "use --token instead")
	// 	})

	// 	t.Run("ServerError", func(t *testing.T) {
	// 		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// 			w.WriteHeader(http.StatusInternalServerError)
	// 		}))
	// 		defer ts.Close()

	// 		_, err := Challenge(ctx, ts.Client(), ts.URL)
	// 		require.Error(t, err)
	// 		assert.Contains(t, err.Error(), "500")
	// 	})

	// 	t.Run("RedirectToOAuth", func(t *testing.T) {
	// 		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// 			switch r.URL.Path {
	// 			case "/":
	// 				http.Redirect(w, r, "/token", http.StatusFound)
	// 			case "/token":
	// 				http.Redirect(w, r, "/oauth2/callback", http.StatusFound)
	// 			case "/oauth2/callback":
	// 				authURL := "https://sso.example.com/auth/realms/MyRealm/protocol/openid-connect/auth?response_type=code&client_id=my-server-client"
	// 				http.Redirect(w, r, authURL, http.StatusFound)
	// 			}
	// 		}))
	// 		defer ts.Close()

	// 		auth, err := Challenge(ctx, ts.Client(), ts.URL)
	// 		require.NoError(t, err)
	// 		require.NotNil(t, auth)
	// 		assert.Equal(t, "https://sso.example.com/auth/realms/MyRealm", auth.Realm)
	// 		assert.Equal(t, "my-server-client", auth.ClientID)
	// 	})

	// 	t.Run("NegotiateScheme", func(t *testing.T) {
	// 		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// 			w.Header().Set("WWW-Authenticate", "Negotiate")
	// 			w.WriteHeader(http.StatusUnauthorized)
	// 		}))
	// 		defer ts.Close()

	// 		_, err := Challenge(ctx, ts.Client(), ts.URL)
	// 		require.Error(t, err)
	// 		assert.Contains(t, err.Error(), "use --token instead")
	// 	})

	// 	t.Run("RedirectToNonOAuth", func(t *testing.T) {
	// 		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// 			switch r.URL.Path {
	// 			case "/":
	// 				http.Redirect(w, r, "/other", http.StatusFound)
	// 			case "/other":
	// 				w.WriteHeader(http.StatusNotFound)
	// 			}
	// 		}))
	// 		defer ts.Close()

	// 		_, err := Challenge(ctx, ts.Client(), ts.URL)
	// 		require.Error(t, err)
	// 		assert.Contains(t, err.Error(), "could not discover")
	// 	})

	// 	t.Run("RedirectToAuthorize", func(t *testing.T) {
	// 		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// 			authURL := "https://idp.example.com/authorize?response_type=code&client_id=test"
	// 			http.Redirect(w, r, authURL, http.StatusFound)
	// 		}))
	// 		defer ts.Close()

	// 		auth, err := Challenge(ctx, ts.Client(), ts.URL)
	// 		require.NoError(t, err)
	// 		require.NotNil(t, auth)
	// 		assert.Equal(t, "https://idp.example.com", auth.Realm)
	// 	})
	// }

	// func TestValidateToken(t *testing.T) {
	// 	ctx := context.Background()

	// 	t.Run("Valid", func(t *testing.T) {
	// 		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// 			assert.Equal(t, "Bearer good-token", r.Header.Get("Authorization"))
	// 			w.WriteHeader(http.StatusOK)
	// 		}))
	// 		defer ts.Close()

	// 		err := ValidateToken(ctx, ts.Client(), ts.URL, "good-token")
	// 		require.NoError(t, err)
	// 	})

	// 	t.Run("Invalid", func(t *testing.T) {
	// 		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// 			w.WriteHeader(http.StatusUnauthorized)
	// 		}))
	// 		defer ts.Close()

	// 		err := ValidateToken(ctx, ts.Client(), ts.URL, "bad-token")
	// 		require.Error(t, err)
	// 		assert.Contains(t, err.Error(), "401")
	// 	})
	// }

	// func TestStartCallbackServer(t *testing.T) {
	// 	t.Run("ReceivesCodeAndState", func(t *testing.T) {
	// 		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	// 		defer cancel()

	// 		addr, resultCh, shutdown, err := StartCallbackServer(ctx, "127.0.0.1:0")
	// 		require.NoError(t, err)
	// 		defer shutdown()

	// 		require.NotEmpty(t, addr)

	// 		resp, err := http.Get("http://" + addr + "/callback?code=test-code&state=test-state")
	// 		require.NoError(t, err)
	// 		defer func() { _ = resp.Body.Close() }()
	// 		assert.Equal(t, http.StatusOK, resp.StatusCode)

	// 		select {
	// 		case result := <-resultCh:
	// 			assert.Equal(t, "test-code", result.Code)
	// 			assert.Equal(t, "test-state", result.State)
	// 			assert.Empty(t, result.Error)
	// 		case <-time.After(2 * time.Second):
	// 			t.Fatal("timed out waiting for callback result")
	// 		}
	// 	})

	// 	t.Run("ReceivesError", func(t *testing.T) {
	// 		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	// 		defer cancel()

	// 		addr, resultCh, shutdown, err := StartCallbackServer(ctx, "127.0.0.1:0")
	// 		require.NoError(t, err)
	// 		defer shutdown()

	// 		resp, err := http.Get("http://" + addr + "/callback?error=access_denied")
	// 		require.NoError(t, err)
	// 		body, _ := io.ReadAll(resp.Body)
	// 		_ = resp.Body.Close()

	// 		assert.Contains(t, string(body), "Login failed")

	// 		select {
	// 		case result := <-resultCh:
	// 			assert.Equal(t, "access_denied", result.Error)
	// 		case <-time.After(2 * time.Second):
	// 			t.Fatal("timed out waiting for callback result")
	// 		}
	// 	})

	// 	t.Run("ShowsSuccessOnValidCallback", func(t *testing.T) {
	// 		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	// 		defer cancel()

	// 		addr, _, shutdown, err := StartCallbackServer(ctx, "127.0.0.1:0")
	// 		require.NoError(t, err)
	// 		defer shutdown()

	// 		resp, err := http.Get("http://" + addr + "/callback?code=test-code&state=test-state")
	// 		require.NoError(t, err)
	// 		body, _ := io.ReadAll(resp.Body)
	// 		_ = resp.Body.Close()

	// 		assert.Contains(t, string(body), "Login successful")
	// 	})

	// 	t.Run("ShutdownStopsServer", func(t *testing.T) {
	// 		ctx := context.Background()

	// 		addr, _, shutdown, err := StartCallbackServer(ctx, "127.0.0.1:0")
	// 		require.NoError(t, err)

	// 		shutdown()

	// 		// Give the server a moment to shut down
	// 		time.Sleep(50 * time.Millisecond)

	// 		_, err = http.Get("http://" + addr + "/callback?code=x")
	// 		require.Error(t, err)
	// 	})

	// 	t.Run("ShutdownByCallerAfterCancel", func(t *testing.T) {
	// 		ctx, cancel := context.WithCancel(context.Background())

	// 		addr, _, shutdown, err := StartCallbackServer(ctx, "127.0.0.1:0")
	// 		require.NoError(t, err)

	// 		cancel()
	// 		shutdown()

	// 		_, err = http.Get("http://" + addr + "/callback?code=x")
	// 		require.Error(t, err)
	// 	})
	// }
}
