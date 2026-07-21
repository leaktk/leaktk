package auth

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartCallbackServer(t *testing.T) {
	t.Run("ReceivesCodeAndState", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		addr, resultCh, shutdown, err := StartCallbackServer(ctx, "127.0.0.1:0")
		require.NoError(t, err)
		defer shutdown()

		require.NotEmpty(t, addr)

		resp, err := http.Get("http://" + addr + "/callback?code=test-code&state=test-state")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		select {
		case result := <-resultCh:
			assert.Equal(t, "test-code", result.Code)
			assert.Equal(t, "test-state", result.State)
			assert.Empty(t, result.Error)
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for callback result")
		}
	})

	t.Run("ReceivesError", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		addr, resultCh, shutdown, err := StartCallbackServer(ctx, "127.0.0.1:0")
		require.NoError(t, err)
		defer shutdown()

		resp, err := http.Get("http://" + addr + "/callback?error=access_denied")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		select {
		case result := <-resultCh:
			assert.Equal(t, "access_denied", result.Error)
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for callback result")
		}
	})

	t.Run("ShutdownStopsServer", func(t *testing.T) {
		ctx := context.Background()

		addr, _, shutdown, err := StartCallbackServer(ctx, "127.0.0.1:0")
		require.NoError(t, err)

		shutdown()

		// Give the server a moment to shut down
		time.Sleep(50 * time.Millisecond)

		_, err = http.Get("http://" + addr + "/callback?code=x")
		require.Error(t, err)
	})

	t.Run("ContextCancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		addr, _, _, err := StartCallbackServer(ctx, "127.0.0.1:0")
		require.NoError(t, err)

		cancel()

		// Give the server a moment to shut down
		time.Sleep(50 * time.Millisecond)

		_, err = http.Get("http://" + addr + "/callback?code=x")
		require.Error(t, err)
	})
}
