package hooks

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leaktk/leaktk/pkg/config"
)

const gitleaksConfig = `
[[rules]]
id = "test-pre-commit"
regex = '''secret\s*=\s*"secretvalue"'''
`

func TestPreCommit(t *testing.T) {
	tempDir := filepath.Clean(t.TempDir())

	cfg := config.DefaultConfig()
	cfg.Scanner.Patterns.Autofetch = false
	cfg.Scanner.Patterns.ExpiredAfter = 0
	cfg.Scanner.Patterns.RefreshAfter = 0
	cfg.Scanner.Patterns.Gitleaks.ConfigPath = filepath.Join(tempDir, ".git/gitleaks.toml")

	ctx := t.Context()

	// Chdir into the tempDir
	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tempDir))
	defer func() { assert.NoError(t, os.Chdir(origWd)) }()

	// Create a git repo
	require.NoError(t, exec.CommandContext(ctx, "git", "-C", tempDir, "init").Run()) // #nosec G204

	// Setup the gitleaks config
	file, err := os.Create(cfg.Scanner.Patterns.Gitleaks.ConfigPath)
	require.NoError(t, err)
	_, err = file.Write([]byte(gitleaksConfig))
	_ = file.Close()
	require.NoError(t, err)

	// Write a non-secret to the repo
	nonSecretFilePath := filepath.Join(tempDir, "some-file")
	file, err = os.Create(nonSecretFilePath)
	require.NoError(t, err)
	_, err = file.Write([]byte("Hello, world!"))
	_ = file.Close()
	require.NoError(t, err)

	// Stage the changes
	require.NoError(t, exec.CommandContext(ctx, "git", "add", nonSecretFilePath).Run()) // #nosec G204

	// Run a scan (should not have findings)
	statusCode, err := preCommitRun(cfg, "git.pre-commit", []string{})
	assert.NoError(t, err)
	assert.Equal(t, statusCode, 0)

	// Write a secret to the repo
	secretFilePath := filepath.Join(tempDir, "secret-file")
	file, err = os.Create(secretFilePath)
	require.NoError(t, err)
	_, err = file.Write([]byte("secret=\"secretvalue\""))
	_ = file.Close()
	require.NoError(t, err)

	// Stage the changes
	require.NoError(t, exec.CommandContext(ctx, "git", "add", secretFilePath).Run()) // #nosec G204

	// Run a scan (should have findings)
	statusCode, err = preCommitRun(cfg, "git.pre-commit", []string{})
	assert.NoError(t, err)
	assert.Equal(t, statusCode, 1)
}
