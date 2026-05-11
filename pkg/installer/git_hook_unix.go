//go:build !windows

package installer

import (
	"github.com/leaktk/leaktk/pkg/fs"
)

// gitHookExists reports whether an existing git hook exists at a given path for determining if
// one should be replaced or skipped
func gitHookExists(path string) bool {
	return fs.IsExecutable(path)
}
