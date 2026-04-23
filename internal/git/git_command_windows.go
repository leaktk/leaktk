//go:build windows

package git

import (
	"context"
	"os/exec"
)

// CommandContext for windows exists for compatibility with
// the unix version that does some extra pgroup managment
func CommandContext(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "git", args...)
}
