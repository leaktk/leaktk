package gitleaks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zricethezav/gitleaks/v8/detect"
	"github.com/zricethezav/gitleaks/v8/report"
	"github.com/zricethezav/gitleaks/v8/sources"
)

var defaultRemote = &sources.RemoteInfo{}

// GitScanOpts configures ScanGit
type GitScanOpts struct {
	Branch   string
	Depth    int
	Remote   *sources.RemoteInfo
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

func ScanReader(ctx context.Context, detector *detect.Detector, reader io.Reader) ([]report.Finding, error) {
	return detector.DetectSource(
		ctx,
		&sources.File{
			Config:          &detector.Config,
			Content:         reader,
			MaxArchiveDepth: detector.MaxArchiveDepth,
		},
	)
}

func ScanURL(ctx context.Context, detector *detect.Detector, rawURL string, opts URLScanOpts) ([]report.Finding, error) {
	return detector.DetectSource(
		ctx,
		&URL{
			Config:           &detector.Config,
			FetchURLPatterns: opts.FetchURLPatterns,
			MaxArchiveDepth:  detector.MaxArchiveDepth,
			RawURL:           rawURL,
		},
	)
}

func ScanJSON(ctx context.Context, detector *detect.Detector, data string, opts JSONScanOpts) ([]report.Finding, error) {
	return detector.DetectSource(
		ctx,
		&JSON{
			Config:           &detector.Config,
			FetchURLPatterns: opts.FetchURLPatterns,
			MaxArchiveDepth:  detector.MaxArchiveDepth,
			RawMessage:       json.RawMessage(data),
		},
	)
}

func ScanFiles(ctx context.Context, detector *detect.Detector, path string) ([]report.Finding, error) {
	return detector.DetectSource(
		ctx,
		&sources.Files{
			Config:          &detector.Config,
			FollowSymlinks:  detector.FollowSymlinks,
			Path:            path,
			Sema:            detector.Sema,
			MaxArchiveDepth: detector.MaxArchiveDepth,
		},
	)
}

func ScanContainerImage(ctx context.Context, detector *detect.Detector, rawImageRef string, opts ContainerImageScanOpts) ([]report.Finding, error) {
	source := &ContainerImage{
		Arch:            opts.Arch,
		Config:          &detector.Config,
		Depth:           opts.Depth,
		Exclusions:      opts.Exclusions,
		MaxArchiveDepth: detector.MaxArchiveDepth,
		RawImageRef:     rawImageRef,
		Remote:          defaultRemote,
		Sema:            detector.Sema,
	}

	if len(opts.Since) > 0 {
		since, err := time.Parse(time.DateOnly, opts.Since)
		if err != nil {
			return nil, fmt.Errorf("could not parse option: since=%q", opts.Since)
		}

		source.Since = &since
	}

	return detector.DetectSource(ctx, source)
}

func ScanGit(ctx context.Context, detector *detect.Detector, gitDir string, opts GitScanOpts) ([]report.Finding, error) {
	gitCmd, err := newGitCmd(ctx, gitDir, opts)
	if err != nil {
		return nil, fmt.Errorf("could not create git command: %w", err)
	}

	var remote *sources.RemoteInfo
	if opts.Remote != nil {
		remote = opts.Remote
	} else {
		remote = defaultRemote
	}

	return detector.DetectSource(
		ctx,
		&sources.Git{
			Cmd:             gitCmd,
			Config:          &detector.Config,
			Remote:          remote,
			Sema:            detector.Sema,
			MaxArchiveDepth: detector.MaxArchiveDepth,
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
