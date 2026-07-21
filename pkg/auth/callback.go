package auth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

type CallbackResult struct {
	Code  string
	State string
	Error string
}

func StartCallbackServer(ctx context.Context, addr string) (string, <-chan CallbackResult, func(), error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return "", nil, nil, fmt.Errorf("could not start callback listener: %w", err)
	}

	resultCh := make(chan CallbackResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		result := CallbackResult{
			Code:  query.Get("code"),
			State: query.Get("state"),
			Error: query.Get("error"),
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, callbackHTML)

		select {
		case resultCh <- result:
		default:
		}
	})

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		_ = server.Serve(listener)
	}()

	shutdown := func() {
		_ = server.Shutdown(context.Background())
	}

	go func() {
		<-ctx.Done()
		shutdown()
	}()

	return listener.Addr().String(), resultCh, shutdown, nil
}

const callbackHTML = `<!DOCTYPE html>
<html>
<head><title>LeakTK Login</title></head>
<body>
<h2>Login successful</h2>
<p>You may close this tab and return to the terminal.</p>
</body>
</html>`
