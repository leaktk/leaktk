package hooks

import (
	"fmt"
	"strings"

	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/logger"
)

// Hook is a wrapper around a hookname to abstract parsing it
// consistently. All hooknames must follow this format:
// ```ebnf
// hookname = hookkind "." hookevent
//
// # All hook kinds must be accounted for in the cmd/install.go file
// hookkind = "git"
//
// # hookevent values depend on the events supported by
// # the hookkind
// hookevent = /a-z(?:[a-z0-9\-]+)?[a-z0-9]?/
// ```
type Hook string

const (
	GitPreCommitHook  = Hook("git.pre-commit")
	GitPreReceiveHook = Hook("git.pre-receive")
)

// Hooks defines all the hooks suported
var Hooks = []Hook{
	GitPreCommitHook,
	GitPreReceiveHook,
}

// Name returns the full name of the hook
func (h Hook) Name() string {
	return string(h)
}

// Kind returns the kind of hook (e.g. git, claude, gcs, etc..)
func (h Hook) Kind() string {
	kind, _, found := strings.Cut(string(h), ".")
	if !found {
		logger.Fatal("invalid hookname format: hookname=%q", hook.Name())
	}
	return kind
}

// Event returns name of the event/phase of a process that this hook alters
// (e.g. pre-commit, pre-recieve, user-prompt-submit, etc...)
func (h Hook) Event() string {
	_, event, found := strings.Cut(string(h), ".")
	if !found {
		logger.Fatal("invalid hookname format: hookname=%q", hook.Name())
	}
	return event
}

// Run executes the provided hook with its arguments
func Run(cfg *config.Config, hook Hook, args []string) (int, error) {
	switch hook {
	case GitPreReceiveHook:
		return gitPreReceiveRun(cfg, hook, args)
	case GitPreCommitHook:
		return gitPreCommitRun(cfg, hook, args)
	default:
		return 1, fmt.Errorf("invalid hookname: hookname=%q", hook.Name())
	}
}
