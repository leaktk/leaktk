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

const posixStdioHookEvalLine = `eval "$(leaktk hook posix.stdio)"`

var hookRegex = regexp.MustCompile(`\bleaktk\s+hook\s+posix\.stdio\b`)

type PosixStdioHookOpts struct {
	Hook   hooks.Hook
	Bashrc bool
	Zshrc  bool
	Stdout bool
}

func PosixStdioHookInstall(ctx context.Context, cfg *config.Config, opts PosixStdioHookOpts) error {
	if opts.Stdout {
		fmt.Println(posixStdioHookEvalLine)
		return nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not detect user home directory: %w", err)
	}

	if opts.Bashrc {
		if err = appendToTargetFile(".bashrc", homeDir); err != nil {
			return fmt.Errorf("failed to write hook to .bashrc: %w", err)
		}
	}
	if opts.Zshrc {
		if err = appendToTargetFile(".zshrc", homeDir); err != nil {
			return fmt.Errorf("failed to write hook to .zshrc: %w", err)
		}
	}

	return nil
}

func appendToTargetFile(targetFile string, homeDir string) error {
	cleanPath := filepath.Clean(filepath.Join(homeDir, targetFile))

	file, err := os.OpenFile(cleanPath, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("failed to open or create target file: %w path=%q", err, cleanPath)
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	if len(hookRegex.FindReaderIndex(bufio.NewReader(file))) > 0 {
		return nil
	}

	if _, err = file.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("failed to seek end of file: %w", err)
	}
	if _, err = fmt.Fprintf(file, "\n%s\n", posixStdioHookEvalLine); err != nil {
		return fmt.Errorf("failed to write hook to configuration file: %w", err)
	}

	return nil
}
