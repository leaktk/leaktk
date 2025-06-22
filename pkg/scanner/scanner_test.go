package scanner

// import (
// 	"bytes"
// 	"context"
// 	"encoding/json"
// 	"fmt"
// 	"os"
// 	"os/exec"
// 	"path/filepath"
// 	"sync"
// 	"testing"
// 	"time"
//
// 	"github.com/leaktk/leaktk/pkg/resource"
// 	"github.com/leaktk/leaktk/pkg/response"
//
// 	"github.com/stretchr/testify/assert"
//
// 	"github.com/leaktk/leaktk/pkg/config"
// )
//
// // mockResource implements a dummy resource
// type mockResource struct {
// 	cloneErr     error
// 	path         string
// 	cloneTimeout time.Duration
// 	depth        int
// 	resource.BaseResource
// }
//
// func (m *mockResource) Kind() string {
// 	return "Mock"
// }
//
// func (m *mockResource) ReadFile(path string) ([]byte, error) {
// 	return []byte{}, nil
// }
//
// func (m *mockResource) Clone(path string) error {
// 	m.path = path
// 	_ = os.MkdirAll(m.path, 0700)
// 	return m.cloneErr
// }
//
// func (m *mockResource) Path() string {
// 	return m.path
// }
//
// func (m *mockResource) Depth() int {
// 	return m.depth
// }
//
// func (m *mockResource) EnrichResult(result *response.Result) *response.Result {
// 	return result
// }
//
// func (m *mockResource) SetDepth(depth int) {
// 	m.depth = depth
// }
//
// func (m *mockResource) SetCloneTimeout(timeout time.Duration) {
// 	m.cloneTimeout = timeout
// }
//
// func (m *mockResource) Since() string {
// 	return ""
// }
//
// func (m *mockResource) String() string {
// 	return ""
// }
//
// func (m *mockResource) Objects(yield resource.ObjectsFunc) error {
// 	return yield(resource.Object{
// 		Path:    "/",
// 		Content: bytes.NewReader([]byte{}),
// 	})
// }
//
// func (m *mockResource) Priority() int {
// 	return 0
// }
//
// func (m *mockResource) IsLocal() bool {
// 	return false
// }
//
// // mockBackend implements a dummy scanner backend
//
// type mockBackend struct {
// }
//
// func (b *mockBackend) Name() string {
// 	return "mock"
// }
//
// func (b *mockBackend) Scan(_ context.Context, resource resource.Resource) ([]*response.Result, error) {
// 	mockResource, _ := resource.(*mockResource)
//
// 	return []*response.Result{
// 		&response.Result{
// 			Notes: map[string]string{
// 				"depth":         fmt.Sprint(resource.Depth()),
// 				"clone_path":    resource.Path(),
// 				"clone_timeout": fmt.Sprintf("%d", int(mockResource.cloneTimeout.Seconds())),
// 			},
// 		},
// 	}, nil
// }
//
// func TestScanner(t *testing.T) {
// 	tempDir := t.TempDir()
// 	cfg := config.DefaultConfig()
// 	cfg.Scanner.CloneTimeout = 10
// 	cfg.Scanner.CloneWorkers = 2
// 	cfg.Scanner.MaxScanDepth = 5
// 	cfg.Scanner.ScanWorkers = 2
// 	cfg.Scanner.Workdir = tempDir
// 	cfg.Scanner.Patterns.Gitleaks.ConfigPath = filepath.Join(tempDir, "gitleaks.toml")
//
// 	t.Run("Success", func(t *testing.T) {
// 		scanner := NewScanner(cfg)
// 		scanner.backends = []Backend{
// 			&mockBackend{},
// 		}
//
// 		request := &Request{
// 			ID: "test-request",
// 			Resource: &mockResource{
// 				depth: 10, // This will be decreased by the MaxScanDepth setting
// 			},
// 		}
//
// 		_, cancel := context.WithTimeout(context.Background(), 10*time.Second)
// 		defer cancel()
//
// 		var wg sync.WaitGroup
//
// 		scanner.Send(request)
// 		wg.Add(1)
//
// 		go scanner.Recv(func(response *response.Response) {
// 			// Depth was reduced to the max scan depth
// 			assert.Equal(t, response.Results[0].Notes["depth"], fmt.Sprint(request.Resource.Depth()))
// 			assert.Equal(t, response.Results[0].Notes["clone_path"], request.Resource.Path())
// 			assert.Equal(t, response.Results[0].Notes["clone_timeout"], fmt.Sprint(cfg.Scanner.CloneTimeout))
// 			wg.Done()
// 		})
//
// 		wg.Wait()
// 	})
//
// 	t.Run("LocalScanSuccess", func(t *testing.T) {
// 		repoDir := t.TempDir()
//
// 		err := exec.Command("git", "-C", repoDir, "init").Run()
// 		assert.NoError(t, err)
//
// 		request := &Request{
// 			ID: "test-local-request",
// 			Resource: resource.NewGitRepo(repoDir, &resource.GitRepoOpts{
// 				Local: true,
// 			}),
// 		}
//
// 		_, cancel := context.WithTimeout(context.Background(), 10*time.Second)
// 		defer cancel()
//
// 		var wg sync.WaitGroup
//
// 		scanner := NewScanner(cfg)
// 		scanner.Send(request)
// 		wg.Add(1)
//
// 		go scanner.Recv(func(response *response.Response) {
// 			assert.Equal(t, response.RequestID, request.ID)
// 			wg.Done()
// 		})
// 		wg.Wait()
//
// 		// Now confirm the repo hasn't been deleted
// 		assert.DirExists(t, repoDir)
// 	})
//
// 	t.Run("GitleaksDecode", func(t *testing.T) {
// 		scanner := NewScanner(cfg)
// 		resource, err := resource.NewResource(
// 			"JSONData",
// 			`{"value": "c2VjcmV0PSJJNmdIY0Ntdk9jYk9Nc0xhaFJucnBUVms3LURVaHpxT3E5SXpTMU03WW9EV1lrWjhwTzlBN2pjM1NreTJjQkVBWUJMVXBHNllQSDdRZ2ptTnJ5NzlKZyI="}`,
// 			json.RawMessage(""),
// 		)
// 		assert.NoError(t, err)
//
// 		request := &Request{
// 			ID:       "test-request",
// 			Resource: resource,
// 		}
//
// 		_, cancel := context.WithTimeout(context.Background(), 10*time.Second)
// 		defer cancel()
//
// 		var wg sync.WaitGroup
//
// 		scanner.Send(request)
// 		wg.Add(1)
//
// 		go scanner.Recv(func(response *response.Response) {
// 			// Confirm no crit errors
// 			for _, log := range response.Logs {
// 				assert.NotEqual(t, log.Severity, "CRITICAL")
// 			}
// 			// Should find the decoded secret
// 			assert.Equal(t, response.Results[0].Secret, "I6gHcCmvOcbOMsLahRnrpTVk7-DUhzqOq9IzS1M7YoDWYkZ8pO9A7jc3Sky2cBEAYBLUpG6YPH7QgjmNry79Jg")
// 			wg.Done()
// 		})
//
// 		wg.Wait()
//
// 	})
// }

// func TestGit(t *testing.T) {
// 	t.Run("BranchNotProvied", func(t *testing.T) {
// 		tempDir := t.TempDir()
//
// 		gitRepo := NewGitRepo("https://github.com/leaktk/fake-leaks.git", &GitRepoOpts{
// 			Depth: 1,
// 		})
//
// 		err := gitRepo.Clone(tempDir)
// 		assert.NoError(t, err)
// 		assert.Greater(t, len(gitRepo.Refs()), 1)
// 	})
//
// 	t.Run("BranchProvied", func(t *testing.T) {
// 		tempDir := t.TempDir()
//
// 		gitRepo := NewGitRepo("https://github.com/leaktk/fake-leaks.git", &GitRepoOpts{
// 			Branch: "main",
// 			Depth:  1,
// 		})
//
// 		err := gitRepo.Clone(tempDir)
// 		assert.NoError(t, err)
// 		assert.Equal(t, len(gitRepo.Refs()), 1)
// 	})
//
// 	t.Run("BranchInvalid", func(t *testing.T) {
// 		tempDir := t.TempDir()
//
// 		gitRepo := NewGitRepo("https://github.com/leaktk/fake-leaks.git", &GitRepoOpts{
// 			Branch: "invalid-branch-e7d33d3a-6057-432a-863b-0ed844ee1f7b",
// 			Depth:  1,
// 		})
//
// 		err := gitRepo.Clone(tempDir)
// 		assert.Error(t, err)
// 	})
// }
