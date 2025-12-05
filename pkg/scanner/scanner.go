package scanner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zricethezav/gitleaks/v8/detect"
	"github.com/zricethezav/gitleaks/v8/report"

	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/fs"
	"github.com/leaktk/leaktk/pkg/id"
	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/leaktk/leaktk/pkg/proto"
	"github.com/leaktk/leaktk/pkg/queue"
	"github.com/leaktk/leaktk/pkg/scanner/gitleaks"

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

// NewScanner returns a initialized and listening scanner instance
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

		ctx := context.Background()
		if s.scanTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, s.scanTimeout)
			defer cancel()
		}

		cfg, err := s.patterns.Gitleaks(ctx)
		if err != nil {
			logger.Critical("scan failed: could load gitleaks config: %v id=%q", err, request.ID)
			s.respondWithError(request, &proto.Error{
				Code:    configErrorCode,
				Message: "could not load gitleaks config",
				Data:    request,
			})

			return
		}

		detector := detect.NewDetector(*cfg)
		detector.FollowSymlinks = false
		detector.IgnoreGitleaksAllow = false
		detector.MaxArchiveDepth = s.maxArchiveDepth
		detector.MaxDecodeDepth = s.maxDecodeDepth
		detector.MaxTargetMegaBytes = 0
		detector.NoColor = true
		detector.Redact = 0
		detector.Verbose = false

		var findings []report.Finding
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

			gitDir, err = absGitDir(gitDir)
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
			findings, err = gitleaks.ScanGit(ctx, detector, gitDir, gitleaks.GitScanOpts{
				Branch:   request.Opts.Branch,
				Depth:    request.Opts.Depth,
				Since:    request.Opts.Since,
				Staged:   request.Opts.Staged,
				Unstaged: request.Opts.Unstaged,
			})
		case proto.URLRequestKind:
			findings, err = gitleaks.ScanURL(ctx, detector, request.Resource, gitleaks.URLScanOpts{
				FetchURLPatterns: splitFetchURLPatterns(request.Opts.FetchURLs),
			})
		case proto.JSONDataRequestKind:
			findings, err = gitleaks.ScanJSON(ctx, detector, request.Resource, gitleaks.JSONScanOpts{
				FetchURLPatterns: splitFetchURLPatterns(request.Opts.FetchURLs),
			})
		case proto.TextRequestKind:
			findings, err = gitleaks.ScanReader(ctx, detector, strings.NewReader(request.Resource))
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
			findings, err = gitleaks.ScanFiles(ctx, detector, request.Resource)
		case proto.ContainerImageRequestKind:
			findings, err = gitleaks.ScanContainerImage(ctx, detector, request.Resource, gitleaks.ContainerImageScanOpts{
				Arch:  request.Opts.Arch,
				Depth: request.Opts.Depth,
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

		results := make([]*proto.Result, len(findings))
		for i, finding := range findings {
			results[i] = findingToResult(request, &finding)
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
		ID: id.ID(
			request.Resource,
			finding.Commit,
			finding.File,
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
		Date:    finding.Date,
		Notes:   map[string]string{},
		Contact: proto.Contact{
			Name:  finding.Author,
			Email: finding.Email,
		},
		Rule: proto.Rule{
			ID:          finding.RuleID,
			Description: finding.Description,
			// TODO: pre 1.0 tags should be moved up to result since
			// tags can be dynamic
			Tags: finding.Tags,
		},
		Location: proto.Location{
			Version: finding.Commit,
			Path:    finding.File,
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
		result.Notes["commit_message"] = finding.Message
		result.Notes["repository"] = request.Resource
		result.Kind = proto.GitCommitResultKind
	case proto.ContainerImageRequestKind:
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

func absGitDir(path string) (string, error) {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--absolute-git-dir") // #nosec G204
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
		additionalConfig, err := gitleaks.ParseConfig(string(rawAdditionalConfig))
		if err != nil {
			logger.Error("could not parse additional config: %s", err)
		} else {
			detector.Config.Allowlists = append(detector.Config.Allowlists, additionalConfig.Allowlists...)
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
		if !remoteGitRefExists(cloneURL, opts.Branch) {
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
	}

	if opts.Depth > 0 {
		cloneArgs = append(cloneArgs, "--depth")
		// Add 1 to the clone depth to avoid scanning a grafted commit
		cloneArgs = append(cloneArgs, strconv.Itoa(opts.Depth+1))
	}

	// Include the clone URL
	gitDir := filepath.Join(s.clonesDir, id.ID())
	cloneArgs = append(cloneArgs, cloneURL, gitDir)
	gitClone := exec.CommandContext(ctx, "git", cloneArgs...)

	if output, err := gitClone.CombinedOutput(); err != nil {
		return "", gitDir, fmt.Errorf("git clone failed: %v cmd=%q output=%q", err, gitClone, output)
	}

	if ctx != nil && ctx.Err() == context.DeadlineExceeded {
		return "", "", fmt.Errorf("clone timeout exceeded: %v", ctx.Err())
	}

	sourcePath, err := checkoutGitSourceConfigFiles(gitDir)
	if err != nil {
		logger.Debug("could not checkout source config files: %v clone_url=%q", err, cloneURL)
	}

	return sourcePath, gitDir, nil
}

func remoteGitRefExists(cloneURL, ref string) bool {
	cmd := exec.Command("git", "ls-remote", "--exit-code", "--quiet", cloneURL, ref) // #nosec G204
	return cmd.Run() == nil
}

func checkoutGitSourceConfigFiles(gitDir string) (string, error) {
	worktreePath := filepath.Join(gitDir, "leaktk-scan-source")

	cmd := exec.Command("git", "-C", gitDir, "worktree", "add", "--no-checkout", worktreePath) // #nosec G204
	if err := cmd.Run(); err != nil {
		return worktreePath, fmt.Errorf("could not create worktree: %v cmd=%q", err, cmd)
	}

	cmd = exec.Command("git", "-C", worktreePath, "checkout", "-f", "HEAD", "--", ".gitleaks*") // #nosec G204
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
