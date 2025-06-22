package scanner

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leaktk/leaktk/pkg/proto"
)

const validGitRepoRequest = `
{
  "id": "foobar",
  "kind": "GitRepo",
  "resource": "https://github.com/leaktk/fake-leaks.git",
  "options": {
    "depth": 256,
    "since": "2000-01-01"
  }
}
`

const invalidGitRepoRequest = `
{
  "id": "foobar",
  "kind": "GitRepo",
  "options": {
    "depth": true,
    "since": "2000-01-01"
  }
}
`

func TestGitRepoRequest(t *testing.T) {
	t.Run("ValidGitRepoRequest", func(t *testing.T) {
		var validRequest proto.Request
		err := json.Unmarshal([]byte(validGitRepoRequest), &validRequest)
		require.NoError(t, err)

		assert.Equal(t, "foobar", validRequest.ID)
		assert.Equal(t, proto.GitRepoRequestKind, validRequest.Kind)
	})

	t.Run("InvalidRequest", func(t *testing.T) {
		var invalidRequest proto.Request
		err := json.Unmarshal([]byte(invalidGitRepoRequest), &invalidRequest)
		assert.Error(t, err)
	})
}
