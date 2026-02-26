package installer

import (
	"github.com/leaktk/leaktk/pkg/config"
)

// GitHookOpts contains flags and options to pass to the git hook installer
type GitHookOpts struct {
	// Name is the name of the hook to install
	Name string

	// Dir is the path to install the hook under. It can be used in addition to the other Dir flags
	Dir string

	// Force replaces existing hooks instead of skipping them
	Force bool

	// Recursive installs hooks in repositories under the top level Dir path
	Recursive bool

	// GlobalTemplateDir installs the hook in the global git template
	GlobalTemplateDir bool

	// UserTemplateDir installs the hook in the user's git init.templateDir (and creates it if it doesn't exist)
	UserTemplateDir bool
}

// const gitHookTemplate = `
// #!/bin/sh
// #TemplateID: 6b563e53-cf8c-49d1-8b4c-a03835c7daa9
// #CreatedBy: %s
// #CreatedOn: %s
// exec leaktk hook %s "$@"
// `

// GitHookInstall is the install function for git hooks
func GitHookInstall(cfg *config.Config, opts GitHookOpts) error {
	// createdOn := time.Now().Format(time.RFC3339)
	// createdBy := fmt.Sprintf("leaktk-%s", version.Version)
	// script = fmt.Sprintf(gitHookTemplate, createdBy, createdOn, opts.Name)

	// TODO (test at each stage):
	// - find install targets
	// - filter based on force flag
	// - setup the script

	return nil
}
