package hooks

import (
	"github.com/leaktk/leaktk/pkg/logger"

	"fmt"
	"os"
	"strings"
)

func posixStdioRun() (int, error) {
	hookScript := `
	# Set user configurable var defaults
	: "${LEAKTK_LOGGER_FILE:="/dev/null"}"

	# Define tmpdir and fifos using the current shell PID ($$)
	_leaktk_hook_posix_stdio_tmpdir="${TMPDIR:-"/tmp"}/leaktk.$$"
	_leaktk_hook_posix_stdio_stdout_fifo="${_leaktk_hook_posix_stdio_tmpdir}/stdout"
	_leaktk_hook_posix_stdio_stderr_fifo="${_leaktk_hook_posix_stdio_tmpdir}/stderr"

	# Create secure directory and named pipes
	mkdir -p -m 0700 "${_leaktk_hook_posix_stdio_tmpdir}"
	mkfifo           "${_leaktk_hook_posix_stdio_stdout_fifo}"
	mkfifo           "${_leaktk_hook_posix_stdio_stderr_fifo}"

	# Start background redaction daemons to listen to the pipes
	leaktk redact --kind Stdio < "${_leaktk_hook_posix_stdio_stdout_fifo}" 1>&1 2>"${LEAKTK_LOGGER_FILE}" &
	leaktk redact --kind Stdio < "${_leaktk_hook_posix_stdio_stderr_fifo}" 1>&2 2>"${LEAKTK_LOGGER_FILE}" &

	# Redirect stdout and stderr
	exec 1> "${_leaktk_hook_posix_stdio_stdout_fifo}"
	exec 2> "${_leaktk_hook_posix_stdio_stderr_fifo}"

	# Cleanup files from disk immediately. The open file descriptors in memory 
	# keep the pipes active until the background processes terminate.
	rm    "${_leaktk_hook_posix_stdio_stdout_fifo}"
	rm    "${_leaktk_hook_posix_stdio_stderr_fifo}"
	rmdir "${_leaktk_hook_posix_stdio_tmpdir}"

	# Unset environment cleanup variables
	unset _leaktk_hook_posix_stdio_tmpdir
	unset _leaktk_hook_posix_stdio_stdout_fifo
	unset _leaktk_hook_posix_stdio_stderr_fifo
	`
	_, err := fmt.Fprint(os.Stdout, strings.TrimSpace(hookScript))
	if err != nil {
		logger.Error("failed to print hook script: %v", err)
		return 1, err
	}

	return 0, nil
}
