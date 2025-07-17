package cmd

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leaktk/leaktk/pkg/fs"
	"github.com/leaktk/leaktk/pkg/proto"
)

func TestScanCommandToRequest(t *testing.T) {
	cmd := scanCommand()
	args := []string{}

	// Resource must be set
	request, err := scanCommandToRequest(cmd, args)
	assert.Nil(t, request)
	require.Error(t, err)
	assert.Equal(t, "missing required field: field=\"resource\"", err.Error())

	// Can provide resource as a positional argument
	args = []string{"https://github.com/leaktk/fake-leaks.git"}
	request, err = scanCommandToRequest(cmd, args)
	require.NoError(t, err)
	assert.NotNil(t, request)

	// ID should default to a random id
	assert.Len(t, request.ID, 11)
	// Kind should default to GitRepo
	assert.Equal(t, proto.GitRepoRequestKind, request.Kind)
	assert.Equal(t, "https://github.com/leaktk/fake-leaks.git", request.Resource)

	// If resource starts with @ and the thing is a valid path, resource will be loaded from there
	tmpDir := t.TempDir()
	dataPath, err := fs.CleanJoin(tmpDir, "data.json")
	require.NoError(t, err)
	err = os.WriteFile(dataPath, []byte("{\"some\": \"data\"}"), 0600)
	require.NoError(t, err)

	args[0] = "@" + dataPath
	_ = cmd.Flags().Set("kind", "JSONData")
	request, err = scanCommandToRequest(cmd, args)
	require.NoError(t, err)
	assert.Equal(t, proto.JSONDataRequestKind, request.Kind)
	assert.JSONEq(t, "{\"some\": \"data\"}", request.Resource)

	// If resource starts with @ and the thing is an invalid path, raise an error
	args[0] = "@" + dataPath + ".invalid"
	request, err = scanCommandToRequest(cmd, args)
	require.Error(t, err)
	assert.Nil(t, request)
	assert.Equal(t, fmt.Sprintf("resource path does not exist: path=%q", dataPath+".invalid"), err.Error())
}
