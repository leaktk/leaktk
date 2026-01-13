//go:build windows

package scanner

import (
	"context"
	"os/exec"
)

// gitCommand for windows exists for compatibility with
// the unix version that does some extra pgroup managment
func gitCommand(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "git", args...)
}
