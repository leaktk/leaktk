package betterleaks

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	blsources "github.com/betterleaks/betterleaks/sources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leaktk/leaktk/internal/auths"
	"github.com/leaktk/leaktk/internal/httpclient"
	"github.com/leaktk/leaktk/internal/sources"
	"github.com/leaktk/leaktk/internal/urlutil"
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

	tsURL, err := urlutil.ParseURL(ts.URL)
	require.NoError(t, err)

	t.Run("General", func(t *testing.T) {
		source := URL{
			RateLimit: httpclient.NewRateLimit(),
			RawURL:    tsURL.JoinPath("general").String(),
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
		source := URL{
			RateLimit: httpclient.NewRateLimit(),
			RawURL:    tsURL.JoinPath("data.json").String(),
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
		// This one doesn't have any sources configured for it so it'll fail auth
		source := URL{
			RateLimit: httpclient.NewRateLimit(),
			RawURL:    tsURL.JoinPath("secure", "content", "1").String(),
		}
		fragments := []blsources.Fragment{}
		err = source.Fragments(context.Background(), func(fragment blsources.Fragment, err error) error {
			return nil
		})
		require.Error(t, err)

		// Now attach a source to it and try again
		jiraSrc := sources.AtlassianCloudJira{
			BaseURL: *tsURL,
			BasicAuth: auths.BasicAuth{
				Username: "user",
				Password: "pass",
			},
		}
		require.NoError(t, jiraSrc.Load(map[string]any{"id": "jira"}))
		require.NoError(t, source.Sources.Add(&jiraSrc))

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
