//go:build windows

package auth

import "os/exec"

func OpenBrowser(url string) error {
	return exec.Command("cmd", "/c", "start", url).Start()
}
