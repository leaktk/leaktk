package installer

import (
	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/hooks"

	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

const hookEvalLine = `eval "$(leaktk hook posix.stdio)"`

type PosixStdioHookOpts struct {
	Hook   hooks.Hook
	Bashrc bool
	Zshrc  bool
	Stdout bool
}

func PosixStdioHookInstall(ctx context.Context, cfg *config.Config, opts PosixStdioHookOpts) error {
	if opts.Stdout {
		fmt.Println(hookEvalLine)
		return nil
	}

	var targetFilename string
	if opts.Bashrc {
		targetFilename = ".bashrc"
	} else if opts.Zshrc {
		targetFilename = ".zshrc"
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not detect user home directory: %w", err)
	}
	targetPath := filepath.Join(homeDir, targetFilename)

	file, err := os.OpenFile(targetPath, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("faile to open or create target file %s: %w", targetPath, err)
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	hookRegex, err := regexp.Compile(`\bleaktk\s+hook\s+posix\.stdio\b`)
	if err != nil {
		return fmt.Errorf("failed to compile idempotency regex: %w", err)
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if hookRegex.MatchString(scanner.Text()) {
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading target file %s: %w", targetPath, err)
	}

	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to read file metadata: %w", err)
	}

	var appendPrefix string
	if fileInfo.Size() > 0 {
		lastByte := make([]byte, 1)
		_, err := file.ReadAt(lastByte, fileInfo.Size()-1)
		if err == nil && lastByte[0] != '\n' {
			appendPrefix = "\n"
		}
	}
	if _, err = file.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("failed to seek to end of file: %w", err)
	}

	if _, err = fmt.Fprintf(file, "%s%s\n", appendPrefix, hookEvalLine); err != nil {
		return fmt.Errorf("failed to write hook to configuration file: %w", err)
	}

	return nil
}
