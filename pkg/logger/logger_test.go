package logger

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAndSetLoggerLevel(t *testing.T) {
	// Default should be INFO
	assert.Equal(t, INFO.String(), GetLoggerLevel().String())

	// It should be changeable
	require.NoError(t, SetLoggerLevel(DEBUG.String()))
	assert.Equal(t, DEBUG.String(), GetLoggerLevel().String())
	require.NoError(t, SetLoggerLevel(INFO.String()))
	assert.Equal(t, INFO.String(), GetLoggerLevel().String())
}
