package betterleaks

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

	bldetect "github.com/betterleaks/betterleaks/detect"
	blreport "github.com/betterleaks/betterleaks/report"
	blsources "github.com/betterleaks/betterleaks/sources"

	"github.com/leaktk/leaktk/internal/httpclient"
	"github.com/leaktk/leaktk/internal/sources"
)

var defaultRemote = &blsources.RemoteInfo{}

// GitScanOpts configures ScanGit
type GitScanOpts struct {
	RevisionRange string
	Depth         int
	Remote        *blsources.RemoteInfo
	Since         string
	Staged        bool
	Unstaged      bool
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
	Sources          sources.Sources
	RateLimit        *httpclient.RateLimit
}

// URLScanOpts configures ScanURL
type URLScanOpts struct {
	FetchURLPatterns []string
	Sources          sources.Sources
	RateLimit        *httpclient.RateLimit
}

func ScanReader(ctx context.Context, detector *bldetect.Detector, reader io.Reader) ([]blreport.Finding, error) {
	return detector.DetectSource(
		ctx,
		&blsources.File{
			Config:          &detector.Config,
			Content:         reader,
			MaxArchiveDepth: detector.MaxArchiveDepth,
		},
	)
}

func ScanURL(ctx context.Context, detector *bldetect.Detector, rawURL string, opts URLScanOpts) ([]blreport.Finding, error) {
	return detector.DetectSource(
		ctx,
		&URL{
			Config:           &detector.Config,
			FetchURLPatterns: opts.FetchURLPatterns,
			MaxArchiveDepth:  detector.MaxArchiveDepth,
			RateLimit:        opts.RateLimit,
			RawURL:           rawURL,
			Sources:          opts.Sources,
		},
	)
}

func ScanJSON(ctx context.Context, detector *bldetect.Detector, data string, opts JSONScanOpts) ([]blreport.Finding, error) {
	return detector.DetectSource(
		ctx,
		&JSON{
			Config:           &detector.Config,
			FetchURLPatterns: opts.FetchURLPatterns,
			MaxArchiveDepth:  detector.MaxArchiveDepth,
			RateLimit:        opts.RateLimit,
			RawMessage:       json.RawMessage(data),
			Sources:          opts.Sources,
		},
	)
}

func ScanFiles(ctx context.Context, detector *bldetect.Detector, path string) ([]blreport.Finding, error) {
	return detector.DetectSource(
		ctx,
		&blsources.Files{
			Config:          &detector.Config,
			FollowSymlinks:  detector.FollowSymlinks,
			Path:            path,
			Sema:            detector.Sema,
			MaxArchiveDepth: detector.MaxArchiveDepth,
		},
	)
}

func ScanContainerImage(ctx context.Context, detector *bldetect.Detector, rawImageRef string, opts ContainerImageScanOpts) ([]blreport.Finding, error) {
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

func ScanGit(ctx context.Context, detector *bldetect.Detector, gitDir string, opts GitScanOpts) ([]blreport.Finding, error) {
	gitCmd, err := newGitCmd(ctx, gitDir, opts)
	if err != nil {
		return nil, fmt.Errorf("could not create git command: %w", err)
	}

	var remote *blsources.RemoteInfo
	if opts.Remote != nil {
		remote = opts.Remote
	} else {
		remote = defaultRemote
	}

	return detector.DetectSource(
		ctx,
		&blsources.Git{
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

func newGitCmd(ctx context.Context, gitDir string, opts GitScanOpts) (gitCmd *blsources.GitCmd, err error) {
	if opts.Unstaged || opts.Staged {
		if gitCmd, err = blsources.NewGitDiffCmdContext(ctx, gitDir, opts.Staged); err != nil {
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

	if len(opts.RevisionRange) > 0 {
		logOpts = append(logOpts, opts.RevisionRange)
	} else {
		logOpts = append(logOpts, "--all")
	}

	if shallowCommits := shallowCommits(gitDir); len(shallowCommits) > 0 {
		logOpts = append(logOpts, "--not")
		logOpts = append(logOpts, shallowCommits...)
	}

	if gitCmd, err = blsources.NewGitLogCmdContext(ctx, gitDir, strings.Join(logOpts, " ")); err != nil {
		return nil, fmt.Errorf("could not create git log cmd: %w", err)
	}

	return gitCmd, err
}
