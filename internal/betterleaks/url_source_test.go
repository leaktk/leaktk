package betterleaks

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	blsources "github.com/betterleaks/betterleaks/sources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leaktk/leaktk/internal/auths"
	"github.com/leaktk/leaktk/internal/httpclient"
	"github.com/leaktk/leaktk/internal/sources"
)

func TestURL(t *testing.T) {
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var content string

		switch r.URL.Path {
		case "/general":
			w.Header().Add("Content-Type", "text/plain")
			content = "general-content"
		case "/data.json":
			w.Header().Add("Content-Type", "application/json")
			content = "{\"data\": \"json-data\"}"
		case "/secure/content/1":
			if r.Header.Get("Authorization") != "Basic dXNlcjpwYXNz" {
				w.WriteHeader(http.StatusUnauthorized)
				_, err := io.WriteString(w, content)
				assert.NoError(t, err)
				return
			}
			w.Header().Add("Content-Type", "text/plain")
			content = "secure-content"
		default:
			t.Errorf("invalid URL path: path=%q", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
		_, err := io.WriteString(w, content)
		assert.NoError(t, err)
	}))

	ts.Start()
	defer ts.Close()

	t.Run("General", func(t *testing.T) {
		generalURL, err := url.JoinPath(ts.URL, "general")
		require.NoError(t, err)

		source := URL{
			RateLimit: httpclient.NewRateLimit(),
			RawURL:    generalURL,
		}

		fragments := []blsources.Fragment{}
		err = source.Fragments(context.Background(), func(fragment blsources.Fragment, err error) error {
			fragments = append(fragments, fragment)
			return nil
		})

		require.NoError(t, err)
		assert.Len(t, fragments, 1)
		assert.Equal(t, "/general", fragments[0].FilePath)
		assert.Equal(t, "general-content", fragments[0].Raw)
	})

	t.Run("JSONData", func(t *testing.T) {
		jsonDataURL, err := url.JoinPath(ts.URL, "data.json")
		require.NoError(t, err)
		source := URL{
			RateLimit: httpclient.NewRateLimit(),
			RawURL:    jsonDataURL,
		}

		fragments := []blsources.Fragment{}
		err = source.Fragments(context.Background(), func(fragment blsources.Fragment, err error) error {
			fragments = append(fragments, fragment)
			return nil
		})

		require.NoError(t, err)
		assert.Len(t, fragments, 1)
		assert.Equal(t, "/data.json!data", fragments[0].FilePath)
		assert.Equal(t, "json-data", fragments[0].Raw)
	})

	t.Run("RequiresAuth", func(t *testing.T) {
		secureURL, err := url.JoinPath(ts.URL, "/secure/content/1")
		require.NoError(t, err)

		// This one doesn't have any sources configured for it so it'll fail auth
		source := URL{
			RateLimit: httpclient.NewRateLimit(),
			RawURL:    secureURL,
		}
		fragments := []blsources.Fragment{}
		err = source.Fragments(context.Background(), func(fragment blsources.Fragment, err error) error {
			return nil
		})
		require.Error(t, err)

		// Now attach a source to it and try again
		source.Sources = append(source.Sources, &sources.AtlassianCloudJira{
			BaseURL: ts.URL,
			BasicAuth: auths.BasicAuth{
				Username: "user",
				Password: "pass",
			},
		})

		err = source.Fragments(context.Background(), func(fragment blsources.Fragment, err error) error {
			fragments = append(fragments, fragment)
			return nil
		})
		require.NoError(t, err)

		assert.Len(t, fragments, 1)
		assert.Equal(t, "/secure/content/1", fragments[0].FilePath)
		assert.Equal(t, "secure-content", fragments[0].Raw)
	})
}
