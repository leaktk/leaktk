package id

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestID(t *testing.T) {
	t.Run("NoParamsCreatesRandomID", func(t *testing.T) {
		a := ID()
		b := ID()
		assert.NotEqual(t, a, b)
	})

	t.Run("IDsAreSameLength", func(t *testing.T) {
		assert.Len(t, ID(), 11)
		assert.Len(t, ID("foo"), 11)
		assert.Len(t, ID("foo", "bar"), 11)
	})

	t.Run("IDsAreHexadecimal", func(t *testing.T) {
		// Run the test a bunch of times on random and parameterized ID calls
		for i := 0; i < 100; i++ {
			assert.Regexp(t, `^[\w-]+$`, ID())
			assert.Regexp(t, `^[\w-]+$`, ID(strconv.Itoa(i), ID()))
		}
	})
}
