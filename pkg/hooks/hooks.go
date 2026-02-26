package hooks

import (
	"fmt"

	"github.com/leaktk/leaktk/pkg/config"
)

// Names defines the list of hooks suported in this module
// all names follow this format:
// ```ebnf
// hookname = hookkind "." hookaction
//
// # All hook kinds must be accounted for in the cmd/install.go file
// hookkind = "git"
//
// # hookaction values depend on the actions supported by
// # the hookkind
// hookaction = /a-z(?:[a-z0-9\-]+)?[a-z0-9]?/
// ```
var Names = []string{
	"git.pre-commit",
}

// Run executes the provided hook with its arguments
func Run(cfg *config.Config, hookname string, args []string) (int, error) {
	switch hookname {
	case "git.pre-commit":
		return preCommitRun(cfg, hookname, args)
	default:
		return 1, fmt.Errorf("invalid hookname: hookname=%q", hookname)
	}
}
