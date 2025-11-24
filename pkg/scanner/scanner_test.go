package scanner

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/proto"
)

func TestScanner(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Scanner.ScanTimeout = 10
	cfg.Scanner.MaxArchiveDepth = 5
	cfg.Scanner.ScanWorkers = 2
	cfg.Scanner.Workdir = tempDir
	cfg.Scanner.Patterns.Gitleaks.ConfigPath = filepath.Join(tempDir, "gitleaks.toml")

	t.Run("RemoteScanSuccess", func(t *testing.T) {
		scanner := NewScanner(cfg)
		request := &proto.Request{
			ID:       "test-remote-request",
			Kind:     proto.GitRepoRequestKind,
			Resource: "https://github.com/leaktk/fake-leaks.git",
			Opts: proto.Opts{
				Branch: "main",
				Depth:  32,
			},
		}

		var wg sync.WaitGroup

		scanner.Send(request)
		wg.Add(1)

		go scanner.Recv(func(response *proto.Response) {
			assert.Nil(t, response.Error)
			assert.NotEmpty(t, response.Results)
			result := response.Results[0]
			// This is just making sure we got a result for this repo. Use the
			// local scan test to test specific behavior
			assert.Equal(t, request.Resource, result.Notes["repository"])
			wg.Done()
		})

		wg.Wait()
	})

	t.Run("LocalScanSuccess", func(t *testing.T) {
		repoDir := t.TempDir()
		err := exec.Command("git", "-C", repoDir, "init").Run() // #nosec:G204
		require.NoError(t, err)

		request := &proto.Request{
			ID:       "test-local-request",
			Kind:     proto.GitRepoRequestKind,
			Resource: repoDir,
		}

		err = os.WriteFile(
			filepath.Join(repoDir, "oops"),
			[]byte(`secret="I6gHcCmvOcbOMsLahRnrpTVk7-DUhzqOq9IzS1M7YoDWYkZ8pO9A7jc3Sky2cBEAYBLUpG6YPH7QgjmNry79Jg"`),
			0600,
		)
		require.NoError(t, err)

		err = exec.Command("git", "-C", repoDir, "add", "-A").Run() // #nosec:G204
		require.NoError(t, err)
		err = exec.Command(
			"git",
			"-C", repoDir,
			"-c",
			"user.name=LeakTK",
			"-c",
			"user.email=leaktk@example.com",
			"commit",
			"-am",
			"oops!",
			"--no-verify").Run() // #nosec:G204
		require.NoError(t, err)

		var wg sync.WaitGroup

		scanner := NewScanner(cfg)
		scanner.Send(request)
		wg.Add(1)

		go scanner.Recv(func(response *proto.Response) {
			assert.Equal(t, response.RequestID, request.ID)
			assert.Nil(t, response.Error)
			assert.Len(t, response.Results, 1)
			assert.Equal(t, "I6gHcCmvOcbOMsLahRnrpTVk7-DUhzqOq9IzS1M7YoDWYkZ8pO9A7jc3Sky2cBEAYBLUpG6YPH7QgjmNry79Jg", response.Results[0].Secret)
			assert.Contains(t, response.Results[0].Notes["commit_message"], "oops!")
			wg.Done()
		})
		wg.Wait()

		// Now confirm the repo hasn't been deleted
		assert.DirExists(t, repoDir)
	})

	t.Run("GitleaksDecode", func(t *testing.T) {
		scanner := NewScanner(cfg)
		request := &proto.Request{
			ID:       "test-request",
			Kind:     proto.JSONDataRequestKind,
			Resource: `{"value": "c2VjcmV0PSJJNmdIY0Ntdk9jYk9Nc0xhaFJucnBUVms3LURVaHpxT3E5SXpTMU03WW9EV1lrWjhwTzlBN2pjM1NreTJjQkVBWUJMVXBHNllQSDdRZ2ptTnJ5NzlKZyI="}`,
		}

		var wg sync.WaitGroup

		scanner.Send(request)
		wg.Add(1)

		go scanner.Recv(func(response *proto.Response) {
			assert.Nil(t, response.Error)
			assert.NotEmpty(t, response.Results)
			assert.Equal(t, "I6gHcCmvOcbOMsLahRnrpTVk7-DUhzqOq9IzS1M7YoDWYkZ8pO9A7jc3Sky2cBEAYBLUpG6YPH7QgjmNry79Jg", response.Results[0].Secret)
			assert.Equal(t, "value", response.Results[0].Location.Path)
			wg.Done()
		})

		wg.Wait()
	})

	t.Run("LocalArchiveSuccess", func(t *testing.T) {
		testFolder := "../../testdata/archive"
		cfg.Scanner.AllowLocal = true

		request := &proto.Request{
			ID:       "test-local-archive-request",
			Kind:     proto.FilesRequestKind,
			Resource: testFolder,
		}
		var wg sync.WaitGroup

		scanner := NewScanner(cfg)
		scanner.Send(request)
		wg.Add(1)

		go scanner.Recv(func(response *proto.Response) {
			assert.Equal(t, response.RequestID, request.ID)
			assert.Nil(t, response.Error)
			assert.Len(t, response.Results, 1)
			assert.Equal(t, "YXBpVmVyc2lvbjogdjEKZGF0YToKICBhd3NfYWNjZXNzX2tleV9pZDogUVV0SlFWaFlXRmhZV0ZoWVdGaFlXRmhZV0ZnPQogIGF3c19zZWNyZXRfYWNjZXNzX2tleTogVURSc1JqUlFlalprWjFwaFlsRjBLM0JrVWxCUUsyczNkbk01Um1GMFFWWnVkR2hZU3pkakNnPT0Ka2luZDogU2VjcmV0Cm1ldGFkYXRhOgogIG5hbWVzcGFjZTogZXhhbXBsZQogIG5hbWU6IGF3cy1jcmVkcwo=", response.Results[0].Secret)
			wg.Done()
		})
		wg.Wait()

	})

	t.Run("depth", func(t *testing.T) {
		tests := []struct {
			providedDepth      int
			maxDepth           int
			expectedScanDepth  int
			expectedCloneDepth int
		}{
			// No max was provided and no depth was set, so no depth is returned
			{0, 0, 0, 0},
			// A depth of one was provided but no max, so we over clone by one and scan one
			{3, 0, 3, 4},
			// No depth was provided but a max depth was set so we limit to the max (+1 for the over clone)
			{0, 10, 10, 11},
			// Depth provided but it's less than the max
			{8, 10, 8, 9},
			// Depth provided and it's more than the max
			{20, 10, 10, 11},
		}

		for _, tt := range tests {
			actualCloneDepth := cloneDepth(tt.providedDepth, tt.maxDepth)
			assert.Equal(t, tt.expectedCloneDepth, actualCloneDepth, "cloneDepth")
			actualScanDepth := scanDepth(tt.providedDepth, tt.maxDepth)
			assert.Equal(t, tt.expectedScanDepth, actualScanDepth, "scanDepth")
		}
	})
}
