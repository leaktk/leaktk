package sources

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKindFromName(t *testing.T) {
	skip := 1 // skip UnknownKind
	for i, name := range KindNames[skip:] {
		i += skip
		t.Run(name, func(t *testing.T) {
			k, ok := KindFromName(name)
			require.True(t, ok)
			assert.Equal(t, Kind(i), k)
		})
	}

	t.Run("unknown", func(t *testing.T) {
		_, ok := KindFromName("DoesNotExist")
		assert.False(t, ok)
	})
}
