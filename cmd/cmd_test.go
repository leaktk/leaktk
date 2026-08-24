package cmd

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/fs"
	"github.com/leaktk/leaktk/pkg/healthcheck"
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
	tempDir := t.TempDir()
	dataPath, err := fs.CleanJoin(tempDir, "data.json")
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

func TestHealthcheckCommand(t *testing.T) {
	cmd := healthcheckCommand()

	assert.Equal(t, "healthcheck [flags] [project]", cmd.Use)
	assert.Equal(t, "Check project settings that help prevent credential leaks", cmd.Short)
	assert.NotNil(t, cmd.Flags().Lookup("fix"))
	assert.NotNil(t, cmd.Flags().Lookup("exit-code"))
}

func TestFormatHealthcheck(t *testing.T) {
	result := &healthcheck.Result{
		Project: "/tmp/project",
		Findings: []healthcheck.Finding{{
			Policy:      "gitignore.env",
			Path:        "/tmp/project/.gitignore",
			Summary:     ".env is not ignored by .gitignore",
			Remediation: "add .env to .gitignore",
		}},
	}

	t.Run("JSON", func(t *testing.T) {
		formatter, err := NewFormatter(config.Formatter{Format: "JSON"})
		require.NoError(t, err)

		assert.JSONEq(t, `{"project":"/tmp/project","findings":[{"policy":"gitignore.env","path":"/tmp/project/.gitignore","summary":".env is not ignored by .gitignore","remediation":"add .env to .gitignore","fixed":false}]}`,
			formatter.FormatHealthcheck(result))
	})

	t.Run("Human", func(t *testing.T) {
		formatter, err := NewFormatter(config.Formatter{Format: "HUMAN"})
		require.NoError(t, err)

		assert.Contains(t, formatter.FormatHealthcheck(result), "Status: needs action")
	})
}
