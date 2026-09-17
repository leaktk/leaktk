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
	bldetect "github.com/betterleaks/betterleaks/detect"
	blreport "github.com/betterleaks/betterleaks/report"
	blsources "github.com/betterleaks/betterleaks/sources"
	blscm "github.com/betterleaks/betterleaks/sources/scm"

	"github.com/leaktk/leaktk/internal/fs"
	"github.com/leaktk/leaktk/internal/httpclient"
	"github.com/leaktk/leaktk/internal/sources"
	"github.com/leaktk/leaktk/pkg/id"
	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/leaktk/leaktk/pkg/proto"
)

type ScannerOpts struct {
	MaxArchiveDepth int
	MaxDecodeDepth  int
}

// GitScanOpts configures ScanGit
type GitScanOpts struct {
	Depth         int
	RevisionRange string
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

func ScanReader(ctx context.Context, request *proto.Request, blScanner *bldetect.Detector, reader io.Reader) ([]*proto.Result, error) {
	findings, err := blScanner.DetectSource(
		ctx,
		&blsources.File{
			Content:         reader,
			MaxArchiveDepth: blScanner.MaxArchiveDepth,
			ShouldSkip:      blScanner.SkipFunc(),
		},
	)

	return findingsToResults(request, findings), err
}

func ScanURL(ctx context.Context, request *proto.Request, blScanner *bldetect.Detector, rawURL string, opts URLScanOpts) ([]*proto.Result, error) {
	findings, err := blScanner.DetectSource(
		ctx,
		&URL{
			FetchURLPatterns: opts.FetchURLPatterns,
			MaxArchiveDepth:  blScanner.MaxArchiveDepth,
			RateLimit:        opts.RateLimit,
			RawURL:           rawURL,
			Sources:          opts.Sources,
			ShouldSkip:       blScanner.SkipFunc(),
		},
	)

	return findingsToResults(request, findings), err
}

func ScanJSON(ctx context.Context, request *proto.Request, blScanner *bldetect.Detector, data string, opts JSONScanOpts) ([]*proto.Result, error) {
	findings, err := blScanner.DetectSource(
		ctx,
		&JSON{
			FetchURLPatterns: opts.FetchURLPatterns,
			MaxArchiveDepth:  blScanner.MaxArchiveDepth,
			RateLimit:        opts.RateLimit,
			RawMessage:       json.RawMessage(data),
			Sources:          opts.Sources,
			ShouldSkip:       blScanner.SkipFunc(),
		},
	)

	return findingsToResults(request, findings), err
}

func ScanFiles(ctx context.Context, request *proto.Request, blScanner *bldetect.Detector, path string) ([]*proto.Result, error) {
	findings, err := blScanner.DetectSource(
		ctx,
		&blsources.Files{
			FollowSymlinks:  blScanner.FollowSymlinks,
			MaxArchiveDepth: blScanner.MaxArchiveDepth,
			Path:            path,
			Sema:            blScanner.Sema,
			ShouldSkip:      blScanner.SkipFunc(),
		},
	)

	return findingsToResults(request, findings), err
}

func ScanContainerImage(ctx context.Context, request *proto.Request, blScanner *bldetect.Detector, rawImageRef string, opts ContainerImageScanOpts) ([]*proto.Result, error) {
	source := &ContainerImage{
		Arch:            opts.Arch,
		Depth:           opts.Depth,
		Exclusions:      opts.Exclusions,
		MaxArchiveDepth: blScanner.MaxArchiveDepth,
		RawImageRef:     rawImageRef,
		Sema:            blScanner.Sema,
		ShouldSkip:      blScanner.SkipFunc(),
	}

	if len(opts.Since) > 0 {
		since, err := time.Parse(time.DateOnly, opts.Since)
		if err != nil {
			return nil, fmt.Errorf("could not parse option: since=%q", opts.Since)
		}

		source.Since = &since
	}

	findings, err := blScanner.DetectSource(ctx, source)
	return findingsToResults(request, findings), err
}

func ScanGit(ctx context.Context, request *proto.Request, blScanner *bldetect.Detector, gitDir string, opts GitScanOpts) ([]*proto.Result, error) {
	platform, remoteURL := blsources.ResolveRemote(ctx, blscm.UnknownPlatform, gitDir)

	gitCmd, err := newGitCmd(ctx, gitDir, opts)
	if err != nil {
		return nil, fmt.Errorf("could not create git command: %w", err)
	}

	findings, err := blScanner.DetectSource(
		ctx,
		&blsources.Git{
			Cmd:             gitCmd,
			MaxArchiveDepth: blScanner.MaxArchiveDepth,
			RemoteURL:       remoteURL,
			Platform:        platform,
			Sema:            blScanner.Sema,
			ShouldSkip:      blScanner.SkipFunc(),
		},
	)

	return findingsToResults(request, findings), err
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

func NewScanner(ctx context.Context, cfg *Config, opts ScannerOpts) (*bldetect.Detector, error) {
	blScanner := bldetect.NewDetectorContext(ctx, (*blconfig.Config)(cfg), bldetect.ValidationOptions{})
	blScanner.FollowSymlinks = false
	blScanner.IgnoreGitleaksAllow = false
	blScanner.MaxArchiveDepth = opts.MaxArchiveDepth
	blScanner.MaxDecodeDepth = opts.MaxDecodeDepth
	blScanner.MaxTargetMegaBytes = 0
	blScanner.NoColor = true
	blScanner.Redact = 0
	blScanner.Verbose = false
	return blScanner, nil
}

func LoadSourceConfig(blScanner *bldetect.Detector, sourcePath string) {
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

func findingsToResults(request *proto.Request, findings []blreport.Finding) []*proto.Result {
	results := make([]*proto.Result, len(findings))

	for i, finding := range findings {
		result := &proto.Result{
			ID: id.ID(
				request.Resource,
				finding.Attributes[AttrGitSHA],
				finding.Attributes[AttrPath],
				strconv.Itoa(finding.StartLine),
				strconv.Itoa(finding.StartColumn),
				strconv.Itoa(finding.EndLine),
				strconv.Itoa(finding.EndColumn),
				finding.RuleID,
			),
			Secret:  finding.Secret,
			Match:   finding.Match,
			Context: finding.Line,
			Entropy: finding.Entropy,
			Date:    finding.Attributes[AttrGitDate],
			Notes:   map[string]string{},
			Rule: proto.Rule{
				ID:          finding.RuleID,
				Description: finding.Description,
				// TODO: pre 1.0 tags should be moved up to result since
				// tags can be dynamic
				Tags: finding.Tags,
			},
			Location: proto.Location{
				Path: finding.Attributes[AttrPath],
				URL:  finding.Attributes[AttrURL],
				Start: proto.Point{
					Line:   finding.StartLine,
					Column: finding.StartColumn,
				},
				End: proto.Point{
					Line:   finding.EndLine,
					Column: finding.EndColumn,
				},
			},
		}

		switch request.Kind {
		case proto.GitRepoRequestKind:
			result.Notes["gitleaks_fingerprint"] = finding.Fingerprint
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

		results[i] = result
	}

	return results
}
