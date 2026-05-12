package scanner

import (
	"bytes"
	"context"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/betterleaks/betterleaks/detect"
	"github.com/betterleaks/betterleaks/report"
	"github.com/betterleaks/betterleaks/sources"

	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/fs"
	"github.com/leaktk/leaktk/pkg/id"
	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/leaktk/leaktk/pkg/proto"
	"github.com/leaktk/leaktk/pkg/queue"
	"github.com/leaktk/leaktk/pkg/scanner/betterleaks"

	httpclient "github.com/leaktk/leaktk/pkg/http"
)

// Set initial queue capacity. The queue can grow over time if needed
const initQueueCapacity = 1024

const (
	noCode = iota
	cloneErrorCode
	configErrorCode
	localScanNotAllowedCode
	scanErrorCode
	sourceErrorCode
	timeoutErrorCode
)

// Scanner holds the config and state for the scanner processes
type Scanner struct {
	allowLocal      bool
	scanTimeout     time.Duration
	clonesDir       string
	maxArchiveDepth int
	maxDecodeDepth  int
	maxScanDepth    int
	patterns        *Patterns
	responseQueue   *queue.PriorityQueue[*proto.Response]
	scanQueue       *queue.PriorityQueue[*proto.Request]
	scanWorkers     int
}

// NewScanner returns a initialized and listening scanner instance that should
// be closed when it's no longer needed.
func NewScanner(cfg *config.Config) *Scanner {
	scanner := &Scanner{
		allowLocal:      cfg.Scanner.AllowLocal,
		scanTimeout:     time.Duration(cfg.Scanner.ScanTimeout) * time.Second,
		clonesDir:       filepath.Join(cfg.Scanner.Workdir, "clones"),
		maxArchiveDepth: cfg.Scanner.MaxArchiveDepth,
		maxDecodeDepth:  cfg.Scanner.MaxDecodeDepth,
		maxScanDepth:    cfg.Scanner.MaxScanDepth,
		patterns:        NewPatterns(&cfg.Scanner.Patterns, httpclient.NewClient()),
		responseQueue:   queue.NewPriorityQueue[*proto.Response](initQueueCapacity, cfg.Scanner.MaxResponseQueueSize),
		scanQueue:       queue.NewPriorityQueue[*proto.Request](initQueueCapacity, cfg.Scanner.MaxScanQueueSize),
		scanWorkers:     cfg.Scanner.ScanWorkers,
	}

	scanner.start()

	return scanner
}

// Recv sends scan responses to a callback function
func (s *Scanner) Recv(fn func(*proto.Response)) {
	s.responseQueue.Recv(func(msg *queue.Message[*proto.Response]) {
		fn(msg.Value)
	})
}

// Send accepts a request for scanning and puts it in the queues
func (s *Scanner) Send(request *proto.Request) {
	logger.Info("queueing scan: id=%q queue_size=%d", request.ID, s.scanQueue.Size()+1)
	s.scanQueue.Send(&queue.Message[*proto.Request]{
		Priority: request.Opts.Priority,
		Value:    request,
	})
}

// start kicks off the background workers
func (s *Scanner) start() {
	// Start workers
	for i := int(0); i < s.scanWorkers; i++ {
		go s.listen()
	}
}

// Watch the scan queue for requests
func (s *Scanner) listen() {
	s.scanQueue.Recv(func(msg *queue.Message[*proto.Request]) {
		request := msg.Value
		logger.Info("starting scan: id=%q", request.ID)

		// Capture panics and return them as errors
		defer func() {
			if r := recover(); r != nil {
				logger.Critical("scan failed: panicked: %v id=%q", r, request.ID)
				logger.Trace("stack trace:\n%s", debug.Stack())
				s.respondWithError(request, &proto.Error{
					Code:    scanErrorCode,
					Message: fmt.Sprintf("scan failed: panicked: %v", r),
					Data:    request,
				})
			}
		}()

		ctx := context.Background()
		if s.scanTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, s.scanTimeout)
			defer cancel()
		}

		cfg, err := s.patterns.Gitleaks(ctx)
		if err != nil {
			logger.Critical("scan failed: could load scanner config: %v id=%q", err, request.ID)
			s.respondWithError(request, &proto.Error{
				Code:    configErrorCode,
				Message: "could not load scanner config",
				Data:    request,
			})

			return
		}

		detector := detect.NewDetectorContext(ctx, betterleaks.CopyConfig(cfg), detect.ValidationOptions{
			Enabled:      false,
			Workers:      0,
			Debug:        false,
			ExtractEmpty: false,
			StatusFilter: "",
		})

		detector.FollowSymlinks = false
		detector.IgnoreGitleaksAllow = false
		detector.MaxArchiveDepth = s.maxArchiveDepth
		detector.MaxDecodeDepth = s.maxDecodeDepth
		detector.MaxTargetMegaBytes = 0
		detector.NoColor = true
		detector.Redact = 0
		detector.Verbose = false
		detector.SkipFindingAppend = true

		var blResults iter.Seq[detect.Result]
		switch request.Kind {
		case proto.GitRepoRequestKind:
			sourcePath := request.Resource
			gitDir := request.Resource

			if !request.Opts.Local {
				if sourcePath, gitDir, err = s.cloneGitRepo(ctx, request.Resource, request.Opts); err != nil {
					select {
					case <-ctx.Done():
						s.respondWithError(request, &proto.Error{
							Code:    cloneErrorCode,
							Message: "clone operation timed out",
							Data:    request,
						})
					default:
						logger.Critical("scan failed: could not clone git repo: %v id=%q", err, request.ID)
						s.respondWithError(request, &proto.Error{
							Code:    cloneErrorCode,
							Message: "could not clone git repo",
							Data:    request,
						})
					}

					return
				}
			} else if !s.allowLocal {
				logger.Critical("scan failed: local scans not allowed: id=%q", request.ID)
				s.respondWithError(request, &proto.Error{
					Code:    localScanNotAllowedCode,
					Message: "local scans not allowed",
					Data:    request,
				})

				return
			}

			gitDir, err = absGitDir(ctx, gitDir)
			if err != nil {
				logger.Critical("scan failed: could not determine gitdir: %v id=%q", err, request.ID)
				s.respondWithError(request, &proto.Error{
					Code:    sourceErrorCode,
					Message: "could not determine gitdir",
					Data:    request,
				})

				return
			}

			if !request.Opts.Local {
				defer (func() {
					if fs.PathExists(sourcePath) {
						if err := os.RemoveAll(sourcePath); err != nil {
							logger.Error("could not remove source path: %v path=%q id=%q", err, sourcePath, request.ID)
						}
					}
					if fs.PathExists(gitDir) {
						if err := os.RemoveAll(gitDir); err != nil {
							logger.Error("could not remove gitdir: %v path=%q id=%q", err, gitDir, request.ID)
						}
					}
				})()
			}

			loadSourceConfig(detector, sourcePath)
			blResults = betterleaks.ScanGit(ctx, detector, gitDir, betterleaks.GitScanOpts{
				Branch:   request.Opts.Branch,
				Depth:    scanDepth(request.Opts.Depth, s.maxScanDepth),
				Since:    request.Opts.Since,
				Staged:   request.Opts.Staged,
				Unstaged: request.Opts.Unstaged,
			})
		case proto.URLRequestKind:
			blResults = betterleaks.ScanURL(ctx, detector, request.Resource, betterleaks.URLScanOpts{
				FetchURLPatterns: splitFetchURLPatterns(request.Opts.FetchURLs),
			})
		case proto.JSONDataRequestKind:
			blResults = betterleaks.ScanJSON(ctx, detector, request.Resource, betterleaks.JSONScanOpts{
				FetchURLPatterns: splitFetchURLPatterns(request.Opts.FetchURLs),
			})
		case proto.TextRequestKind:
			blResults = betterleaks.ScanReader(ctx, detector, strings.NewReader(request.Resource))
		case proto.FilesRequestKind:
			if !s.allowLocal {
				logger.Critical("scan failed: local scans not allowed: id=%q", request.ID)
				s.respondWithError(request, &proto.Error{
					Code:    localScanNotAllowedCode,
					Message: "local scans not allowed",
					Data:    request,
				})

				return
			}
			loadSourceConfig(detector, request.Resource)
			blResults = betterleaks.ScanFiles(ctx, detector, request.Resource)
		case proto.ContainerImageRequestKind:
			blResults = betterleaks.ScanContainerImage(ctx, detector, request.Resource, betterleaks.ContainerImageScanOpts{
				Arch:  request.Opts.Arch,
				Depth: scanDepth(request.Opts.Depth, s.maxScanDepth),
				Since: request.Opts.Since,
			})
		default:
			logger.Warning("unexpected request kind: %s", request.Kind)
		}

		var scanErr *proto.Error

		if err != nil {
			select {
			case <-ctx.Done():
				s.respondWithError(request, &proto.Error{
					Code:    timeoutErrorCode,
					Message: "operation timed out",
					Data:    request,
				})
				return
			default:
				scanErr = &proto.Error{
					Code:    scanErrorCode,
					Message: err.Error(),
					Data:    request,
				}
				logger.Error("scan error: %v id=%q", scanErr, request.ID)
			}
		}

		var results []*proto.Result
		for blResult := range blResults {
			if err := blResult.Err; err != nil {
				logger.Error("betterleaks: finding error: %v id=%q", err, request.ID)
			} else {
				results = append(results, findingToResult(request, &blResult.Finding))
			}
		}

		logger.Info("queueing response: id=%q queue_size=%d", request.ID, s.responseQueue.Size()+1)
		s.responseQueue.Send(&queue.Message[*proto.Response]{
			Priority: msg.Priority,
			Value: &proto.Response{
				ID:        id.ID(),
				Kind:      proto.ScanResultsResponseKind,
				RequestID: request.ID,
				Error:     scanErr,
				Results:   results,
			},
		})
	})
}

func (s *Scanner) respondWithError(request *proto.Request, err *proto.Error) {
	logger.Info("queueing response: id=%q queue_size=%d", request.ID, s.responseQueue.Size()+1)
	logger.Error("scan error: %v id=%q", err, request.ID)
	s.responseQueue.Send(&queue.Message[*proto.Response]{
		Priority: request.Opts.Priority,
		Value: &proto.Response{
			ID:        id.ID(),
			Kind:      proto.ScanResultsResponseKind,
			RequestID: request.ID,
			Error:     err,
		},
	})
}

func findingToResult(request *proto.Request, finding *report.Finding) *proto.Result {
	result := &proto.Result{
		Secret:  finding.Secret,
		Match:   finding.Match,
		Context: finding.Line,
		Entropy: finding.Entropy,
		Date:    finding.Date,
		Notes:   map[string]string{},
		Rule: proto.Rule{
			ID:          finding.RuleID,
			Description: finding.Description,
			// TODO: pre 1.0 tags should be moved up to result since
			// tags can be dynamic
			Tags: finding.Tags,
		},
		Location: proto.Location{
			Path: finding.File,
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
		result.Kind = proto.GitCommitResultKind
		result.Location.Version = finding.Attr(sources.AttrGitSHA)
		result.Contact.Name = finding.Attr(sources.AttrGitAuthorName)
		result.Contact.Email = finding.Attr(sources.AttrGitAuthorEmail)
		result.Notes["commit_message"] = finding.Attr(sources.AttrGitMessage)
		result.Notes["repository"] = request.Resource
	case proto.ContainerImageRequestKind:
		result.Location.Version = finding.Attr(betterleaks.AttrContainerDigest)
		result.Contact.Name = finding.Attr(betterleaks.AttrContainerAuthorName)
		result.Contact.Email = finding.Attr(betterleaks.AttrContainerAuthorEmail)
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

	// Build the result ID now that all the fields are normalized above
	result.ID = id.ID(
		request.Resource,
		result.Location.Version,
		result.Location.Path,
		strconv.Itoa(result.Location.Start.Line),
		strconv.Itoa(result.Location.Start.Column),
		strconv.Itoa(result.Location.End.Line),
		strconv.Itoa(result.Location.End.Column),
		result.Rule.ID,
	)

	return result
}

func absGitDir(ctx context.Context, path string) (string, error) {
	cmd := gitCommand(ctx, "-C", path, "rev-parse", "--absolute-git-dir") // #nosec G204
	logger.Debug("executing: %s", cmd)
	stdout, err := cmd.Output()

	return string(bytes.TrimSpace(stdout)), err
}

func loadSourceConfig(detector *detect.Detector, sourcePath string) {
	if !fs.DirExists(sourcePath) {
		return
	}
	additionalConfigPath := filepath.Join(sourcePath, ".gitleaks.toml")
	rawAdditionalConfig, err := os.ReadFile(additionalConfigPath) // #nosec G304
	if err == nil && len(rawAdditionalConfig) > 0 {
		logger.Debug("applying additional config: path=%q", additionalConfigPath)
		additionalConfig, err := betterleaks.ParseConfig(string(rawAdditionalConfig))
		if err != nil {
			logger.Error("could not parse additional config: %s", err)
		} else {
			detector.Config = betterleaks.AppendGlobalConfig(detector.Config, additionalConfig)
		}
	} else {
		logger.Debug("no additional config")
	}

	baselinePath := filepath.Join(sourcePath, ".gitleaksbaseline")
	if fs.FileExists(baselinePath) {
		logger.Debug("applying .gitleaksbaseline: path=%q", baselinePath)
		if err := detector.AddBaseline(baselinePath, sourcePath); err != nil {
			logger.Error("could not add baseline: %v", err)
		}
	}

	ignorePath := filepath.Join(sourcePath, ".gitleaksignore")
	if fs.FileExists(ignorePath) {
		logger.Debug("applying .gitleaksignore: path=%q", ignorePath)
		if err := detector.AddGitleaksIgnore(ignorePath); err != nil {
			logger.Error("could not add gitleaksignore: %v", err)
		}
	}
}

func (s *Scanner) cloneGitRepo(ctx context.Context, cloneURL string, opts proto.Opts) (string, string, error) {
	cloneArgs := []string{"clone"}

	if len(opts.Proxy) > 0 {
		cloneArgs = append(cloneArgs, "--config")
		cloneArgs = append(cloneArgs, "http.proxy="+opts.Proxy)
	}

	// The --[no-]single-branch flags are still needed with mirror due to how
	// things like --depth and --shallow-since behave
	if len(opts.Branch) > 0 {
		if !remoteGitRefExists(ctx, cloneURL, opts.Branch) {
			return "", "", fmt.Errorf("remote ref does not exist: ref=%q", opts.Branch)
		}

		cloneArgs = append(cloneArgs, "--bare")
		cloneArgs = append(cloneArgs, "--single-branch")
		cloneArgs = append(cloneArgs, "--branch")
		cloneArgs = append(cloneArgs, opts.Branch)
	} else {
		cloneArgs = append(cloneArgs, "--mirror")
		cloneArgs = append(cloneArgs, "--no-single-branch")
	}

	if len(opts.Since) > 0 {
		cloneArgs = append(cloneArgs, "--shallow-since")
		cloneArgs = append(cloneArgs, opts.Since)

		if opts.Depth > 0 {
			logger.Warning(
				"cloning with since=%q instead of depth=%d; since=%q and depth=%d will be applied to the scan: clone_url=%q",
				opts.Since,
				cloneDepth(opts.Depth, s.maxScanDepth),
				opts.Since,
				scanDepth(opts.Depth, s.maxScanDepth),
				cloneURL,
			)
		}
	} else if depth := cloneDepth(opts.Depth, s.maxScanDepth); depth > 0 {
		cloneArgs = append(cloneArgs, "--depth")
		cloneArgs = append(cloneArgs, strconv.Itoa(depth))
	}

	// Include the clone URL
	gitDir := filepath.Join(s.clonesDir, id.ID())
	cloneArgs = append(cloneArgs, cloneURL, gitDir)
	gitClone := gitCommand(ctx, cloneArgs...)

	logger.Debug("executing: %s", gitClone)
	if output, err := gitClone.CombinedOutput(); err != nil {
		return "", gitDir, fmt.Errorf("git clone failed: %v cmd=%q output=%q", err, gitClone, output)
	}

	if ctx != nil && ctx.Err() == context.DeadlineExceeded {
		return "", "", fmt.Errorf("clone timeout exceeded: %v", ctx.Err())
	}

	sourcePath, err := checkoutGitSourceConfigFiles(ctx, gitDir)
	if err != nil {
		logger.Debug("could not checkout source config files: %v clone_url=%q", err, cloneURL)
	}

	return sourcePath, gitDir, nil
}

func remoteGitRefExists(ctx context.Context, cloneURL, ref string) bool {
	cmd := gitCommand(ctx, "ls-remote", "--exit-code", "--quiet", cloneURL, ref) // #nosec G204
	logger.Debug("executing: %s", cmd)
	return cmd.Run() == nil
}

func checkoutGitSourceConfigFiles(ctx context.Context, gitDir string) (string, error) {
	worktreePath := filepath.Join(gitDir, "leaktk-scan-source")

	cmd := gitCommand(ctx, "-C", gitDir, "worktree", "add", "--no-checkout", worktreePath) // #nosec G204
	logger.Debug("executing: %s", cmd)
	if err := cmd.Run(); err != nil {
		return worktreePath, fmt.Errorf("could not create worktree: %v cmd=%q", err, cmd)
	}

	cmd = gitCommand(ctx, "-C", worktreePath, "checkout", "-f", "HEAD", "--", ".gitleaks*") // #nosec G204
	logger.Debug("executing: %s", cmd)
	if err := cmd.Run(); err != nil {
		return worktreePath, fmt.Errorf("could not checkout gitleaks files: %v cmd=%q", err, cmd)
	}

	return worktreePath, nil
}

func splitFetchURLPatterns(patterns string) []string {
	if len(patterns) == 0 {
		return []string{}
	}

	return strings.Split(patterns, ":")
}

// cloneDepth provides the depth to clone. If there is no max it returns 0.
// Clones should be one more than the desired scan depth
func cloneDepth(providedDepth, maxDepth int) int {
	if depth := scanDepth(providedDepth, maxDepth); depth > 0 {
		return depth + 1
	}
	return 0
}

// scanDepth provides the depth to scan. If there is no max it returns 0.
func scanDepth(providedDepth, maxDepth int) int {
	if maxDepth > 0 {
		if providedDepth > 0 {
			return min(providedDepth, maxDepth)
		}

		return maxDepth
	}

	return providedDepth
}
