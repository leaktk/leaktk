package collectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKindFromName(t *testing.T) {
	for i, name := range KindNames {
		t.Run(name, func(t *testing.T) {
			fk, ok := KindFromName(name)
			require.True(t, ok)
			assert.Equal(t, Kind(i), fk)
		})
	}

	t.Run("unknown", func(t *testing.T) {
		_, ok := KindFromName("DoesNotExist")
		assert.False(t, ok)
	})
}
