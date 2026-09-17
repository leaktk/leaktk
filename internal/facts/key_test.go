package facts

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeyFromName(t *testing.T) {
	for i, name := range KeyNames {
		t.Run(name, func(t *testing.T) {
			fk, ok := KeyFromName(name)
			require.True(t, ok)
			assert.Equal(t, Key(i), fk)
		})
	}

	t.Run("unknown", func(t *testing.T) {
		_, ok := KeyFromName("DoesNotExist")
		assert.False(t, ok)
	})
}
