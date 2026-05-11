package hooks

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leaktk/leaktk/pkg/config"
)

const betterleaksPreReceiveTestConfig = `
[[rules]]
id = "test-pre-receive"
regex = '''secret\s*=\s*"secretvalue"'''
`

// TestGitPreReceive only tests the function itself but does not do a full integration test
func TestGitPreReceive(t *testing.T) {
	tempDir := filepath.Clean(t.TempDir())

	cfg := config.DefaultConfig()
	cfg.Scanner.Patterns.Autofetch = false
	cfg.Scanner.Patterns.ExpiredAfter = 0
	cfg.Scanner.Patterns.RefreshAfter = 0
	cfg.Scanner.Patterns.Gitleaks.ConfigPath = filepath.Join(tempDir, ".git/gitleaks.toml")

	ctx := t.Context()

	// Chdir into the tempDir since the hook is executed from the git dir
	origWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tempDir))
	defer func() { assert.NoError(t, os.Chdir(origWd)) }()

	// Mock Stdin
	mockStdin, err := os.OpenFile(filepath.Join(tempDir, "stdin"), os.O_RDWR|os.O_CREATE, 0600)
	defer func() { require.NoError(t, mockStdin.Close()) }()
	require.NoError(t, err)
	origStdin := os.Stdin
	defer func() { os.Stdin = origStdin }()
	os.Stdin = mockStdin

	// Create a git repo
	require.NoError(t, exec.CommandContext(ctx, "git", "-C", tempDir, "init").Run())                                       // #nosec G204
	require.NoError(t, exec.CommandContext(ctx, "git", "-C", tempDir, "config", "user.email", "leaktk@example.com").Run()) // #nosec G204
	require.NoError(t, exec.CommandContext(ctx, "git", "-C", tempDir, "config", "user.name", "LeakTK").Run())              // #nosec G204

	// Setup the betterleaks config
	file, err := os.Create(cfg.Scanner.Patterns.Gitleaks.ConfigPath)
	require.NoError(t, err)
	_, err = file.Write([]byte(betterleaksPreReceiveTestConfig))
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
	require.NoError(t, exec.CommandContext(ctx, "git", "-C", tempDir, "add", nonSecretFilePath).Run())                             // #nosec G204
	require.NoError(t, exec.CommandContext(ctx, "git", "-C", tempDir, "commit", "-m", "add non-secret file", "--no-verify").Run()) // #nosec G204
	nonSecretCommitID, err := exec.CommandContext(ctx, "git", "-C", tempDir, "rev-parse", "HEAD").Output()                         // #nosec G204
	require.NoError(t, err)
	nonSecretCommitID = bytes.TrimSpace(nonSecretCommitID)

	// Setup the data for pre-receive to handle
	_, err = mockStdin.Seek(0, 0)
	require.NoError(t, err)
	_, _ = mockStdin.Write(emptyOID)
	_, _ = mockStdin.WriteString(" ")
	_, _ = mockStdin.Write(nonSecretCommitID)
	_, _ = mockStdin.WriteString(" ")
	_, _ = mockStdin.WriteString("refs/heads/main\n")
	_, err = mockStdin.Seek(0, 0)
	require.NoError(t, err)

	// Run a scan (should not have findings)
	statusCode, err := gitPreReceiveRun(cfg, "git.pre-receive", []string{})
	require.NoError(t, err)
	assert.Equal(t, 0, statusCode)

	// Write a secret to the repo
	secretFilePath := filepath.Join(tempDir, "secret-file")
	file, err = os.Create(secretFilePath)
	require.NoError(t, err)
	_, err = file.Write([]byte("secret=\"secretvalue\""))
	_ = file.Close()
	require.NoError(t, err)

	// Stage the changes
	require.NoError(t, exec.CommandContext(ctx, "git", "-C", tempDir, "add", secretFilePath).Run())                            // #nosec G204
	require.NoError(t, exec.CommandContext(ctx, "git", "-C", tempDir, "commit", "-m", "add secret file", "--no-verify").Run()) // #nosec G204
	secretCommitID, err := exec.CommandContext(ctx, "git", "-C", tempDir, "rev-parse", "HEAD").Output()                        // #nosec G204
	require.NoError(t, err)
	secretCommitID = bytes.TrimSpace(secretCommitID)

	// Setup the data for pre-receive to handle
	_, err = mockStdin.Seek(0, 0)
	require.NoError(t, err)
	_, _ = mockStdin.Write(nonSecretCommitID)
	_, _ = mockStdin.WriteString(" ")
	_, _ = mockStdin.Write(secretCommitID)
	_, _ = mockStdin.WriteString(" ")
	_, _ = mockStdin.WriteString("refs/heads/main\n")
	_, err = mockStdin.Seek(0, 0)
	require.NoError(t, err)

	// Run a scan (should have findings)
	statusCode, err = gitPreReceiveRun(cfg, "git.pre-receive", []string{})
	require.NoError(t, err)
	assert.Equal(t, 1, statusCode)
}
