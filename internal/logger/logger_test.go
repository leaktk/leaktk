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
	require.NoError(t, SetLoggerLevelString(DEBUG.String()))
	assert.Equal(t, DEBUG.String(), GetLoggerLevel().String())
	require.NoError(t, SetLoggerLevelString(INFO.String()))
	assert.Equal(t, INFO.String(), GetLoggerLevel().String())
}
