package hooks

import (
	"fmt"

	"github.com/leaktk/leaktk/pkg/config"
)

// HookNames defines the list of hooks suported in this module
var HookNames = []string{
	"git.pre-commit",
}

// Run executes the provided hook with its arguments
func Run(cfg *config.Config, hookName string, args []string) (int, error) {
	switch hookName {
	case "git.pre-commit":
		return preCommitRun(cfg, hookName, args)
	default:
		return 1, fmt.Errorf("invalid hookname: hookname=%q", hookName)
	}
}
