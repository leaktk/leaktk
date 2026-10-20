//go:build darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd

package auth

import (
	"os/exec"
	"runtime"
)

func OpenBrowser(url string) error {
	var cmd string
	if runtime.GOOS == "darwin" {
		cmd = "open"
	} else {
		cmd = "xdg-open"
	}

	return exec.Command(cmd, url).Start() // #nosec G204
}
