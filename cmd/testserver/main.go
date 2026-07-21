package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	dexIssuer := "http://127.0.0.1:5556/dex"

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if len(token) == 0 {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s"`, dexIssuer))
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = fmt.Fprintln(w, "unauthorized")
			return
		}
		_, _ = fmt.Fprintf(w, "authenticated: %s\n", r.URL.Path)
	})

	addr := "127.0.0.1:8580"
	fmt.Fprintf(os.Stderr, "test pattern server on %s (dex issuer: %s)\n", addr, dexIssuer)
	if err := http.ListenAndServe(addr, nil); err != nil { // #nosec G114
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
