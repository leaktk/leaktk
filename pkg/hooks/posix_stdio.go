package hooks

import (
	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/logger"

	"fmt"
	"os"
	"strings"
)

func posixStdioRun(cfg *config.Config, hook Hook, _ []string) (int, error) {
	hookScript := `
	exec 3>&1
	exec 4>&2

	exec > >(leaktk redact --kind Stdio >&3)
	exec 2> >(leaktk redact --kind Stdio >&4)
	`

	_, err := fmt.Fprint(os.Stdout, strings.TrimSpace(hookScript))
	if err != nil {
		logger.Error("failed to print hook script: %v", err)
		return 1, err
	}

	return 0, nil
}
