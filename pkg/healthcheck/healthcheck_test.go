package healthcheck

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	t.Run("ReportsMissingEnvIgnore", func(t *testing.T) {
		project := t.TempDir()

		result, err := Run(project, false)

		require.NoError(t, err)
		assert.Equal(t, project, result.Project)
		require.Len(t, result.Findings, 1)
		assert.Equal(t, gitIgnoreEnvPolicy, result.Findings[0].Policy)
		assert.Equal(t, filepath.Join(project, ".gitignore"), result.Findings[0].Path)
		assert.False(t, result.Findings[0].Fixed)
		assert.True(t, result.NeedsAction())
		assert.NoFileExists(t, filepath.Join(project, ".gitignore"))
	})

	t.Run("RecognizesCommonEnvIgnorePatterns", func(t *testing.T) {
		for _, pattern := range []string{".env", "/.env", "**/.env", "*.env"} {
			t.Run(pattern, func(t *testing.T) {
				project := t.TempDir()
				err := os.WriteFile(filepath.Join(project, ".gitignore"), []byte(pattern+"\n"), 0600)
				require.NoError(t, err)

				result, err := Run(project, false)

				require.NoError(t, err)
				assert.Empty(t, result.Findings)
			})
		}
	})

	t.Run("LastMatchingPatternWins", func(t *testing.T) {
		project := t.TempDir()
		err := os.WriteFile(filepath.Join(project, ".gitignore"), []byte(".env\n!.env\n"), 0600)
		require.NoError(t, err)

		result, err := Run(project, false)

		require.NoError(t, err)
		assert.Len(t, result.Findings, 1)
	})

	t.Run("FixCreatesGitIgnore", func(t *testing.T) {
		project := t.TempDir()

		result, err := Run(project, true)

		require.NoError(t, err)
		require.Len(t, result.Findings, 1)
		assert.True(t, result.Findings[0].Fixed)
		assert.False(t, result.NeedsAction())
		assert.FileExists(t, filepath.Join(project, ".gitignore"))
		assertFileContent(t, filepath.Join(project, ".gitignore"), ".env\n")

		result, err = Run(project, true)
		require.NoError(t, err)
		assert.Empty(t, result.Findings)
	})

	t.Run("FixAppendsSeparateLineAndPreservesLineEnding", func(t *testing.T) {
		project := t.TempDir()
		path := filepath.Join(project, ".gitignore")
		err := os.WriteFile(path, []byte("node_modules\r\ncoverage"), 0644)
		require.NoError(t, err)

		_, err = Run(project, true)

		require.NoError(t, err)
		assertFileContent(t, path, "node_modules\r\ncoverage\r\n.env\r\n")
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0644), info.Mode().Perm())
	})

	t.Run("RejectsFilePath", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "project")
		err := os.WriteFile(path, []byte{}, 0600)
		require.NoError(t, err)

		result, err := Run(path, false)

		assert.Nil(t, result)
		require.EqualError(t, err, "project path is not a directory: path="+`"`+path+`"`)
	})
}

func assertFileContent(t *testing.T, path string, expected string) {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, expected, string(data))
}
