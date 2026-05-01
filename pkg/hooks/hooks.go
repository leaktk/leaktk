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
type HookKind string
type Hook string

const (
	GitHookKind = HookKind("git")
)

const (
	GitPreCommitHook  = Hook(GitHookKind + ".pre-commit")
	GitPreReceiveHook = Hook(GitHookKind + ".pre-receive")
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
func (h Hook) Kind() HookKind {
	kind, _, found := strings.Cut(h.Name(), ".")
	if !found {
		logger.Fatal("invalid hookname format: hookname=%q", h.Name())
	}
	return HookKind(kind)
}

// Event returns name of the event/phase of a process that this hook alters
// (e.g. pre-commit, pre-receive, user-prompt-submit, etc...)
func (h Hook) Event() string {
	_, event, found := strings.Cut(h.Name(), ".")
	if !found {
		logger.Fatal("invalid hookname format: hookname=%q", h.Name())
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
