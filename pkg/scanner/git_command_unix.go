//go:build !windows

package scanner

import (
	"context"
	"os/exec"
	"syscall"
)

// gitCommand sets extra things on the command like the pgid
// and cancel function to ensure the command doesn't hang
// when in weird states
func gitCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		// kill the negative pid to kill the whole process group
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	return cmd
}
