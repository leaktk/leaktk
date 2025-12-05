package sources

import (
	"testing"

	"github.com/leaktk/leaktk/pkg/config"

	"github.com/stretchr/testify/require"
)

func TestNewSource(t *testing.T) {
	t.Run("GitHub", func(t *testing.T) {
		srcCfg := config.Source{
			ID:   "github.com",
			Kind: config.GitHubSourceKind,
			URL:  "https://api.github.com",
		}

		source, err := NewSource(srcCfg)
		require.NoError(t, err)

		_, ok := source.(GitHub)
		require.True(t, ok, "NewSource did not return a GitHub source")

		// TODO: test some basic sources things here
	})
}
