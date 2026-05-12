package betterleaks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/betterleaks/betterleaks/detect"
	"github.com/betterleaks/betterleaks/sources"
	"github.com/betterleaks/betterleaks/sources/scm"

	"github.com/leaktk/leaktk/pkg/logger"
)

// GitScanOpts configures ScanGit
type GitScanOpts struct {
	Branch   string
	Depth    int
	Since    string
	Staged   bool
	Unstaged bool
}

// ContainerImageScanOpts configures ScanContainerImage
type ContainerImageScanOpts struct {
	Arch       string
	Depth      int
	Exclusions []string
	Since      string
}

// JSONScanOpts configures ScanJSON
type JSONScanOpts struct {
	FetchURLPatterns []string
}

// URLScanOpts configures ScanURL
type URLScanOpts struct {
	FetchURLPatterns []string
}

func ScanReader(ctx context.Context, detector *detect.Detector, reader io.Reader) iter.Seq[detect.Result] {
	return detector.Run(
		ctx,
		&sources.File{
			Content:         reader,
			MaxArchiveDepth: detector.MaxArchiveDepth,
			ShouldSkip:      detector.SkipFunc(),
		},
	)
}

func ScanURL(ctx context.Context, detector *detect.Detector, rawURL string, opts URLScanOpts) iter.Seq[detect.Result] {
	return detector.Run(
		ctx,
		&URL{
			FetchURLPatterns: opts.FetchURLPatterns,
			MaxArchiveDepth:  detector.MaxArchiveDepth,
			RawURL:           rawURL,
			ShouldSkip:       detector.SkipFunc(),
		},
	)
}

func ScanJSON(ctx context.Context, detector *detect.Detector, data string, opts JSONScanOpts) iter.Seq[detect.Result] {
	return detector.Run(
		ctx,
		&JSON{
			FetchURLPatterns: opts.FetchURLPatterns,
			MaxArchiveDepth:  detector.MaxArchiveDepth,
			RawMessage:       json.RawMessage(data),
		},
	)
}

func ScanFiles(ctx context.Context, detector *detect.Detector, path string) iter.Seq[detect.Result] {
	return detector.Run(
		ctx,
		&sources.Files{
			FollowSymlinks:  detector.FollowSymlinks,
			MaxArchiveDepth: detector.MaxArchiveDepth,
			Path:            path,
			Sema:            detector.Sema,
			ShouldSkip:      detector.SkipFunc(),
		},
	)
}

func ScanContainerImage(ctx context.Context, detector *detect.Detector, rawImageRef string, opts ContainerImageScanOpts) iter.Seq[detect.Result] {
	source := &ContainerImage{
		Arch:            opts.Arch,
		Depth:           opts.Depth,
		Exclusions:      opts.Exclusions,
		MaxArchiveDepth: detector.MaxArchiveDepth,
		RawImageRef:     rawImageRef,
		Sema:            detector.Sema,
		ShouldSkip:      detector.SkipFunc(),
	}

	if len(opts.Since) > 0 {
		since, err := time.Parse(time.DateOnly, opts.Since)
		if err != nil {
			logger.Error("could not parse option: %v since=%q", err, opts.Since)
			return nil
		}
		source.Since = &since
	}

	return detector.Run(ctx, source)
}

func ScanGit(ctx context.Context, detector *detect.Detector, gitDir string, opts GitScanOpts) iter.Seq[detect.Result] {
	gitCmd, err := newGitCmd(ctx, gitDir, opts)
	if err != nil {
		logger.Error("could not create git command: %v", err)
		return nil
	}

	return detector.Run(
		ctx,
		&sources.Git{
			Cmd:             gitCmd,
			MaxArchiveDepth: detector.MaxArchiveDepth,
			Platform:        scm.NoPlatform,
			Sema:            detector.Sema,
			ShouldSkip:      detector.SkipFunc(),
		},
	)
}

func shallowCommits(gitDir string) []string {
	var shallowCommits []string

	data, err := os.ReadFile(filepath.Join(gitDir, "shallow")) // #nosec G304
	if err != nil {
		return shallowCommits
	}

	for _, shallowCommit := range strings.Split(string(data), "\n") {
		if len(shallowCommit) > 0 {
			shallowCommits = append(shallowCommits, shallowCommit)
		}
	}

	return shallowCommits
}

func newGitCmd(ctx context.Context, gitDir string, opts GitScanOpts) (gitCmd *sources.GitCmd, err error) {
	if opts.Unstaged || opts.Staged {
		if gitCmd, err = sources.NewGitDiffCmdContext(ctx, gitDir, opts.Staged); err != nil {
			return nil, fmt.Errorf("could not create git diff cmd: %w", err)
		}
		return gitCmd, nil
	}

	logOpts := []string{"--full-history", "--ignore-missing"}

	if len(opts.Since) > 0 {
		logOpts = append(logOpts, "--since")
		logOpts = append(logOpts, opts.Since)
	}

	if opts.Depth > 0 {
		logOpts = append(logOpts, "--max-count")
		logOpts = append(logOpts, strconv.Itoa(opts.Depth))
	}

	if len(opts.Branch) > 0 {
		logOpts = append(logOpts, opts.Branch)
	} else {
		logOpts = append(logOpts, "--all")
	}

	if shallowCommits := shallowCommits(gitDir); len(shallowCommits) > 0 {
		logOpts = append(logOpts, "--not")
		logOpts = append(logOpts, shallowCommits...)
	}

	if gitCmd, err = sources.NewGitLogCmdContext(ctx, gitDir, strings.Join(logOpts, " ")); err != nil {
		return nil, fmt.Errorf("could not create git log cmd: %w", err)
	}

	return gitCmd, err
}
