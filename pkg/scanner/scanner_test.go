package scanner

import (
	"bytes"
	"context"
	"fmt"
	"github.com/leaktk/scanner/pkg/resource"
	"github.com/leaktk/scanner/pkg/response"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/leaktk/scanner/pkg/config"
)

// mockResource implements a dummy resource
type mockResource struct {
	cloneErr     error
	clonePath    string
	cloneTimeout time.Duration
	depth        uint16
	resource.BaseResource
}

func (m *mockResource) Kind() string {
	return "Mock"
}

func (m *mockResource) ReadFile(path string) ([]byte, error) {
	return []byte{}, nil
}

func (m *mockResource) Clone(path string) error {
	m.clonePath = path
	_ = os.MkdirAll(m.clonePath, 0700)
	return m.cloneErr
}

func (m *mockResource) ClonePath() string {
	return m.clonePath
}

func (m *mockResource) Depth() uint16 {
	return m.depth
}

func (m *mockResource) EnrichResult(result *response.Result) *response.Result {
	return result
}

func (m *mockResource) SetDepth(depth uint16) {
	m.depth = depth
}

func (m *mockResource) SetCloneTimeout(timeout time.Duration) {
	m.cloneTimeout = timeout
}

func (m *mockResource) Since() string {
	return ""
}

func (m *mockResource) String() string {
	return ""
}

func (m *mockResource) Walk(fn resource.WalkFunc) error {
	return fn("/", bytes.NewReader([]byte{}))
}

// mockBackend implements a dummy scanner backend

type mockBackend struct {
}

func (b *mockBackend) Name() string {
	return "mock"
}

func (b *mockBackend) Scan(resource resource.Resource) ([]*response.Result, error) {
	mockResource, _ := resource.(*mockResource)

	return []*response.Result{
		&response.Result{
			Notes: map[string]string{
				"depth":         fmt.Sprint(resource.Depth()),
				"clone_path":    resource.ClonePath(),
				"clone_timeout": fmt.Sprintf("%d", int(mockResource.cloneTimeout.Seconds())),
			},
		},
	}, nil
}

func TestScanner(t *testing.T) {
	tempDir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Scanner.CloneTimeout = 10
	cfg.Scanner.CloneWorkers = 2
	cfg.Scanner.MaxCloneQueueSize = 10
	cfg.Scanner.MaxScanDepth = 5
	cfg.Scanner.MaxScanQueueSize = 10
	cfg.Scanner.ScanWorkers = 2
	cfg.Scanner.Workdir = tempDir

	scanner := NewScanner(cfg)
	defer scanner.Close()

	scanner.backends = []Backend{
		&mockBackend{},
	}

	t.Run("Success", func(t *testing.T) {
		request := &Request{
			ID: "test-request",
			Resource: &mockResource{
				depth: 10, // This will be decreased by the MaxScanDepth setting
			},
		}

		_, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		scanner.Send(request)
		response := <-scanner.Responses()

		// Depth was reduced to the max scan depth
		assert.Equal(t, response.Results[0].Notes["depth"], fmt.Sprint(request.Resource.Depth()))
		assert.Equal(t, response.Results[0].Notes["clone_path"], request.Resource.ClonePath())
		assert.Equal(t, response.Results[0].Notes["clone_timeout"], fmt.Sprint(cfg.Scanner.CloneTimeout))
	})
}
