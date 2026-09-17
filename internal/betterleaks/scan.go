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

	blconfig "github.com/betterleaks/betterleaks/config"
	blreport "github.com/betterleaks/betterleaks/v2/report"
	blscan "github.com/betterleaks/betterleaks/v2/scan"
	blsources "github.com/betterleaks/betterleaks/v2/sources"
	blscm "github.com/betterleaks/betterleaks/v2/sources/scm"

	"github.com/leaktk/leaktk/internal/fs"
	"github.com/leaktk/leaktk/internal/httpclient"
	"github.com/leaktk/leaktk/internal/logger"
	"github.com/leaktk/leaktk/internal/sources"
	"github.com/leaktk/leaktk/pkg/id"
	"github.com/leaktk/leaktk/pkg/proto"
)

type ScannerOpts struct {
	MatchContext   string
	MaxDecodeDepth int
	MinConfidence  string
	Workers        int
}

type ContainerImageScanOpts struct {
	Arch            string
	Depth           int
	Exclusions      []string
	MaxArchiveDepth int
	Since           string
}

type FilesScanOpts struct {
	FollowSymlinks  bool
	MaxArchiveDepth int
	Workers         int
}

type GitScanOpts struct {
	Depth           int
	MaxArchiveDepth int
	RevisionRange   string
	Since           string
	Staged          bool
	Unstaged        bool
	Workers         int
}

type JSONScanOpts struct {
	FetchURLPatterns []string
	MaxArchiveDepth  int
	RateLimit        *httpclient.RateLimit
	Sources          sources.Sources
}

type ReaderScanOpts struct {
	MaxArchiveDepth int
}

type URLScanOpts struct {
	FetchURLPatterns []string
	MaxArchiveDepth  int
	RateLimit        *httpclient.RateLimit
	Sources          sources.Sources
}

func ScanReader(ctx context.Context, request *proto.Request, blScanner *blscan.Scanner, reader io.Reader, opts ReaderScanOpts) ([]*proto.Result, error) {
	return scanSrc(ctx, blScanner, request, &blsources.File{
		Content:         reader,
		Logger:          logger.SLogger(),
		MaxArchiveDepth: opts.MaxArchiveDepth,
		ShouldSkip:      blScanner.SkipFunc(),
	})
}

func ScanURL(ctx context.Context, request *proto.Request, blScanner *blscan.Scanner, rawURL string, opts URLScanOpts) ([]*proto.Result, error) {
	return scanSrc(ctx, blScanner, request, &URL{
		FetchURLPatterns: opts.FetchURLPatterns,
		Logger:           logger.SLogger(),
		MaxArchiveDepth:  opts.MaxArchiveDepth,
		RateLimit:        opts.RateLimit,
		RawURL:           rawURL,
		ShouldSkip:       blScanner.SkipFunc(),
		Sources:          opts.Sources,
	})
}

func ScanJSON(ctx context.Context, request *proto.Request, blScanner *blscan.Scanner, data string, opts JSONScanOpts) ([]*proto.Result, error) {
	return scanSrc(ctx, blScanner, request, &JSON{
		FetchURLPatterns: opts.FetchURLPatterns,
		Logger:           logger.SLogger(),
		MaxArchiveDepth:  opts.MaxArchiveDepth,
		RateLimit:        opts.RateLimit,
		RawMessage:       json.RawMessage(data),
		ShouldSkip:       blScanner.SkipFunc(),
		Sources:          opts.Sources,
	})
}

func ScanFiles(ctx context.Context, request *proto.Request, blScanner *blscan.Scanner, path string, opts FilesScanOpts) ([]*proto.Result, error) {
	return scanSrc(ctx, blScanner, request, &blsources.Files{
		FollowSymlinks:  opts.FollowSymlinks,
		Logger:          logger.SLogger(),
		MaxArchiveDepth: opts.MaxArchiveDepth,
		Path:            path,
		ShouldSkip:      blScanner.SkipFunc(),
		Workers:         opts.Workers,
	})
}

func ScanContainerImage(ctx context.Context, request *proto.Request, blScanner *blscan.Scanner, rawImageRef string, opts ContainerImageScanOpts) ([]*proto.Result, error) {
	src := ContainerImage{
		Arch:            opts.Arch,
		Depth:           opts.Depth,
		Exclusions:      opts.Exclusions,
		Logger:          logger.SLogger(),
		MaxArchiveDepth: opts.MaxArchiveDepth,
		RawImageRef:     rawImageRef,
		ShouldSkip:      blScanner.SkipFunc(),
	}

	if len(opts.Since) > 0 {
		since, err := time.Parse(time.DateOnly, opts.Since)
		if err != nil {
			return nil, fmt.Errorf("could not parse option: since=%q", opts.Since)
		}

		src.Since = &since
	}

	return scanSrc(ctx, blScanner, request, &src)
}

func ScanGit(ctx context.Context, request *proto.Request, blScanner *blscan.Scanner, gitDir string, opts GitScanOpts) ([]*proto.Result, error) {
	platform, remoteURL := blsources.ResolveRemote(ctx, blscm.UnknownPlatform, gitDir)

	gitCmd, err := newGitCmd(ctx, gitDir, opts)
	if err != nil {
		return nil, fmt.Errorf("could not create git command: %w", err)
	}

	return scanSrc(ctx, blScanner, request, &blsources.Git{
		Cmd:             gitCmd,
		Logger:          logger.SLogger(),
		MaxArchiveDepth: opts.MaxArchiveDepth,
		Platform:        platform,
		RemoteURL:       remoteURL,
		ShouldSkip:      blScanner.SkipFunc(),
		Workers:         opts.Workers,
	})

}

func scanSrc(ctx context.Context, blScanner *blscan.Scanner, request *proto.Request, src blsources.Source) ([]*proto.Result, error) {
	results := make([]*proto.Result, 0, 16)
	_, err := blScanner.Scan(ctx, src, func(f blreport.Finding) error {
		results = append(results, findingToResult(request, &f))
		return nil
	})
	return results, err
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

func NewScanner(ctx context.Context, cfg Config, opts ScannerOpts) (*blscan.Scanner, error) {
	return blscan.New(blconfig.Config(cfg),
		blscan.WithLogger(logger.SLogger()),
		blscan.WithWorkers(opts.Workers),
		blscan.WithMatchContext(opts.MatchContext),
		blscan.WithMaxDecodeDepth(opts.MaxDecodeDepth),
		blscan.WithMinimumConfidence(blscan.Confidence(opts.MinConfidence)),
		blscan.WithPrecompile(),
		blscan.WithIgnoreAllowComments(false),
	)
}

func LoadSourceConfig(blScanner *blscan.Scanner, sourcePath string) {
	if !fs.DirExists(sourcePath) {
		logger.Debug("skipping additional config: source path does not exist: path=%q", sourcePath)
		return
	}

	additionalConfigPath := filepath.Join(sourcePath, ".betterleaks.toml")
	rawAdditionalConfig, err := os.ReadFile(additionalConfigPath) // #nosec G304
	if err != nil || len(rawAdditionalConfig) == 0 {
		additionalConfigPath = filepath.Join(sourcePath, ".gitleaks.toml")
		rawAdditionalConfig, err = os.ReadFile(additionalConfigPath) // #nosec G304
	}
	if err == nil && len(rawAdditionalConfig) > 0 {
		logger.Debug("applying additional config: path=%q", additionalConfigPath)
		additionalConfig, err := ParseConfig(rawAdditionalConfig)
		if err != nil {
			logger.Error("could not parse additional config: %s", err)
		} else {
			blScanner.Config.Prefilter = mergeExpressions(blScanner.Config.Prefilter, additionalConfig.Prefilter)
			blScanner.Config.Filter = mergeExpressions(blScanner.Config.Filter, additionalConfig.Filter)
			if err := blScanner.Config.CompileFilters(nil); err != nil {
				logger.Error("could not compile merged filters: %s", err)
			}
		}
	} else {
		logger.Debug("no additional config")
	}

	baselinePath := filepath.Join(sourcePath, ".gitleaksbaseline")
	if fs.FileExists(baselinePath) {
		logger.Debug("applying .gitleaksbaseline: path=%q", baselinePath)
		if err := blScanner.AddBaseline(baselinePath, sourcePath); err != nil {
			logger.Error("could not add baseline: %v", err)
		}
	}

	ignorePath := filepath.Join(sourcePath, ".gitleaksignore")
	if fs.FileExists(ignorePath) {
		logger.Debug("applying .gitleaksignore: path=%q", ignorePath)
		if err := blScanner.AddGitleaksIgnore(ignorePath); err != nil {
			logger.Error("could not add gitleaksignore: %v", err)
		}
	}
}

func mergeExpressions(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return "(" + a + ") || (" + b + ")"
}

func findingToResult(request *proto.Request, finding *blreport.Finding) *proto.Result {
	result := &proto.Result{
		ID: id.ID(
			request.Resource,
			finding.Attributes[AttrGitSHA],
			finding.Location.Path,
			strconv.Itoa(finding.Location.StartLine),
			strconv.Itoa(finding.Location.StartColumn),
			strconv.Itoa(finding.Location.EndLine),
			strconv.Itoa(finding.Location.EndColumn),
			finding.RuleID,
		),
		// TODO: change Secret to Target or make a similar match struct
		Secret:   finding.Match.Value,
		Match:    finding.Match.Full,
		Captures: finding.Match.Captures,
		Context:  finding.MatchContext,
		Entropy:  finding.Entropy,
		Date:     finding.Attributes[AttrGitDate],
		Notes:    map[string]string{},
		Rule: proto.Rule{
			ID:          finding.RuleID,
			Description: finding.Description,
			// TODO: pre 1.0 tags should be moved up to result since
			// tags can be dynamic
			Tags: finding.Tags,
		},
		Location: proto.Location{
			Path: finding.Location.Path,
			URL:  finding.Attributes[AttrURL],
			Start: proto.Point{
				Line:   finding.Location.StartLine,
				Column: finding.Location.StartColumn,
			},
			End: proto.Point{
				Line:   finding.Location.EndLine,
				Column: finding.Location.EndColumn,
			},
		},
	}

	switch request.Kind {
	case proto.GitRepoRequestKind:
		result.Notes["commit_message"] = finding.Attributes[AttrGitMessage]
		result.Notes["repository"] = request.Resource
		result.Kind = proto.GitCommitResultKind
		result.Location.Version = finding.Attributes[AttrGitSHA]
		result.Contact = proto.Contact{
			Name:  finding.Attributes[AttrGitAuthorName],
			Email: finding.Attributes[AttrGitAuthorEmail],
		}
	case proto.ContainerImageRequestKind:
		result.Location.Version = finding.Attributes[AttrOCIImageDigest]
		authorName := finding.Attributes[AttrOCIImageAuthorName]
		authorEmail := finding.Attributes[AttrOCIImageAuthorEmail]
		maintainerName := finding.Attributes[AttrOCIImageMaintainerName]
		maintainerEmail := finding.Attributes[AttrOCIImageMaintainerEmail]

		// Prefer the one with the email else fall back on the one with the name
		// Prefer author over maintainer for the contact
		if len(authorEmail) > 0 {
			result.Contact = proto.Contact{Name: authorName, Email: authorEmail}
		} else if len(maintainerEmail) > 0 {
			result.Contact = proto.Contact{Name: maintainerName, Email: maintainerEmail}
		} else if len(authorName) > 0 {
			result.Contact = proto.Contact{Name: authorName, Email: authorEmail}
		} else if len(maintainerName) > 0 {
			result.Contact = proto.Contact{Name: maintainerName, Email: maintainerEmail}
		}

		manifest := ""
		parts := strings.Split(result.Location.Path, "/")
		if len(parts) > 1 {
			if strings.Contains(result.Location.Path, "layers/") {
				loc := strings.Split(result.Location.Path, "!")
				if len(loc) > 1 {
					result.Location.Path = loc[1]
					result.Kind = proto.ContainerLayerResultKind
				}
			}
			manifest = parts[1]
			result.Kind = proto.ContainerMetdataResultKind
		}
		if manifest != "" {
			result.Notes["image"] = request.Resource + "@" + manifest
		} else {
			result.Notes["image"] = request.Resource
		}

	case proto.URLRequestKind:
		result.Notes["url"] = request.Resource
		result.Kind = proto.GenericResultKind
	default:
		result.Kind = proto.GenericResultKind
	}

	return result
}
