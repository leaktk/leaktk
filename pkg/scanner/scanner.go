package scanner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/betterleaks/betterleaks/detect"
	"github.com/betterleaks/betterleaks/report"

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

// GitRepoInfo is a collection of facts about a repo being scanned.
// See `man 7 gitglossary` for more information about the terms.
type GitRepoInfo struct {
	// Whether or not the repo is a bare repo
	IsBare bool
	// The path to the actual GIT_DIR folder
	GitDir string
	// The working tree for the repo (a temp one is created for bare repos)
	WorkingTreePath string
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

		detector := detect.NewDetectorContext(ctx, *cfg)
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
			var gitRepoInfo GitRepoInfo

			if request.Opts.Local {
				// Make sure local scans are allowed before continuing
				if !s.allowLocal {
					logger.Critical("scan failed: local scans are not allowed: id=%q", request.ID)
					s.respondWithError(request, &proto.Error{
						Code:    localScanNotAllowedCode,
						Message: "local scans not allowed",
						Data:    request,
					})

					return
				}

				// Load the gitRepoInfo from the repo
				gitRepoInfo, err = getGitRepoInfo(ctx, request.Resource)
				if err != nil {
					logger.Critical("scan failed: could not get git repo info: %v id=%q", err, request.ID)
					s.respondWithError(request, &proto.Error{
						Code:    sourceErrorCode,
						Message: "could not get git repo info",
						Data:    request,
					})

					return
				}
			} else {
				// Clone the repo and get its gitRepoInfo
				gitRepoInfo, err = s.cloneGitRepo(ctx, request.Resource, request.Opts)
				if err != nil {
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
			}

			// Handle setting up a temp worktree for accessing certain files in bare repos
			if gitRepoInfo.IsBare {
				// Create temp working tree for bare repos with just the config files included
				gitRepoInfo.WorkingTreePath, err = tempCheckoutGitSourceConfigFiles(ctx, gitRepoInfo.GitDir)
				if err != nil {
					// Only log this as a debug item since it shouldn't result in fewer findings but
					// may result in more false positives
					logger.Debug("could not set up temp working tree for bare repo: %v id=%q", err, request.ID)
				}

				/////////////////////////////
				// TODO: ensure this runs for sure before results are sent instead of here, sometimes the command exits before the cleanup finishes
				/////////////////////////////
				// Ensure the temp working tree is removed after the scan
				defer func() {
					if fs.PathExists(gitRepoInfo.WorkingTreePath) {
						// First try cleaning up the worktrees the proper way
						logger.Debug("removing temp git working tree: path=%q", gitRepoInfo.WorkingTreePath)
						err := gitCommand(ctx, "-C", gitRepoInfo.GitDir, "worktree", "remove", "--force", gitRepoInfo.WorkingTreePath).Run()
						if err != nil {
							logger.Error("error removing temp working tree: %v path=%q, id=%q", err, gitRepoInfo.WorkingTreePath, request.ID)
						}

						// This is a fallback to make real sure we clean up as much as we can
						if fs.PathExists(gitRepoInfo.WorkingTreePath) {
							logger.Warning("removing worktree some files manually: path=%q id=%q", gitRepoInfo.WorkingTreePath, request.ID)
							if err := os.RemoveAll(gitRepoInfo.WorkingTreePath); err != nil {
								logger.Error("could not remove temp working tree: %v path=%q id=%q", err, gitRepoInfo.WorkingTreePath, request.ID)
							}
							if err := gitCommand(ctx, "-C", gitRepoInfo.GitDir, "worktree", "prune").Run(); err != nil {
								logger.Error("could not prune worktrees: %v path=%q id=%q", err, gitRepoInfo.WorkingTreePath, request.ID)
							}
						}
					}
				}()
			}

			// Handle removing temp cloned git dir
			if !request.Opts.Local {
				defer func() {
					if fs.PathExists(gitRepoInfo.GitDir) {
						logger.Debug("removing temp git dir: path=%q", gitRepoInfo.GitDir)
						if err := os.RemoveAll(gitRepoInfo.GitDir); err != nil {
							logger.Error("could not remove temp gitdir: %v path=%q id=%q", err, gitRepoInfo.GitDir, request.ID)
						}
					}
				}()
			}

			// Load the checked out config from the working tree
			loadSourceConfig(detector, gitRepoInfo.WorkingTreePath)

			// If there are exclusions, create a revision range like:
			// ^{exclusion1} ^{exclusion2} {branch}
			revisionRange := request.Opts.Branch
			exclusionsLen := len(request.Opts.Exclusions)
			if exclusionsLen > 0 {
				items := make([]string, len(request.Opts.Exclusions)+1)
				for i, item := range request.Opts.Exclusions {
					items[i] = "^" + item
				}
				items[exclusionsLen] = request.Opts.Branch
				revisionRange = strings.Join(items, " ")
			}

			findings, err = betterleaks.ScanGit(ctx, detector, gitRepoInfo.GitDir, betterleaks.GitScanOpts{
				RevisionRange: revisionRange,
				Depth:         scanDepth(request.Opts.Depth, s.maxScanDepth),
				Since:         request.Opts.Since,
				Staged:        request.Opts.Staged,
				Unstaged:      request.Opts.Unstaged,
			})
		case proto.URLRequestKind:
			findings, err = betterleaks.ScanURL(ctx, detector, request.Resource, betterleaks.URLScanOpts{
				FetchURLPatterns: splitFetchURLPatterns(request.Opts.FetchURLs),
			})
		case proto.JSONDataRequestKind:
			findings, err = betterleaks.ScanJSON(ctx, detector, request.Resource, betterleaks.JSONScanOpts{
				FetchURLPatterns: splitFetchURLPatterns(request.Opts.FetchURLs),
			})
		case proto.TextRequestKind:
			findings, err = betterleaks.ScanReader(ctx, detector, strings.NewReader(request.Resource))
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
			findings, err = betterleaks.ScanFiles(ctx, detector, request.Resource)
		case proto.ContainerImageRequestKind:
			findings, err = betterleaks.ScanContainerImage(ctx, detector, request.Resource, betterleaks.ContainerImageScanOpts{
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

func getGitRepoInfo(ctx context.Context, path string) (GitRepoInfo, error) {
	info := GitRepoInfo{}
	cmd := gitCommand(
		ctx,
		"-C",
		path,
		"rev-parse",
		// The order of these flags affects the field order below
		"--absolute-git-dir",
		"--is-bare-repository",
	) // #nosec G204

	logger.Debug("executing: %s", cmd)
	rawInfo, err := cmd.Output()
	if err != nil {
		return info, err
	}

	fields := bytes.Fields(rawInfo)
	if len(fields) != 2 {
		return info, errors.New("could not load git repo info")
	}

	// Load the field data from above
	info.GitDir = string(fields[0])
	info.IsBare = bytes.Equal(fields[1], []byte("true"))
	return info, nil
}

func loadSourceConfig(detector *detect.Detector, sourcePath string) {
	if !fs.DirExists(sourcePath) {
		logger.Debug("skipping additional config: source path does not exist: path=%q", sourcePath)
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

func (s *Scanner) cloneGitRepo(ctx context.Context, cloneURL string, opts proto.Opts) (GitRepoInfo, error) {
	cloneArgs := []string{"clone"}
	gitRepoInfo := GitRepoInfo{}

	if len(opts.Proxy) > 0 {
		cloneArgs = append(cloneArgs, "--config")
		cloneArgs = append(cloneArgs, "http.proxy="+opts.Proxy)
	}

	// The --[no-]single-branch flags are still needed with mirror due to how
	// things like --depth and --shallow-since behave
	if len(opts.Branch) > 0 {
		if !remoteGitRefExists(ctx, cloneURL, opts.Branch) {
			return gitRepoInfo, fmt.Errorf("remote ref does not exist: ref=%q", opts.Branch)
		}
		gitRepoInfo.IsBare = true
		cloneArgs = append(cloneArgs, "--bare")
		cloneArgs = append(cloneArgs, "--single-branch")
		cloneArgs = append(cloneArgs, "--branch")
		cloneArgs = append(cloneArgs, opts.Branch)
	} else {
		gitRepoInfo.IsBare = true
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
	gitRepoInfo.GitDir = gitDir

	logger.Debug("executing: %s", gitClone)
	if output, err := gitClone.CombinedOutput(); err != nil {
		return gitRepoInfo, fmt.Errorf("git clone failed: %w cmd=%q output=%q", err, gitClone, output)
	}

	if ctx != nil && ctx.Err() == context.DeadlineExceeded {
		return gitRepoInfo, fmt.Errorf("clone timeout exceeded: %w", ctx.Err())
	}

	return gitRepoInfo, nil
}

func remoteGitRefExists(ctx context.Context, cloneURL, ref string) bool {
	cmd := gitCommand(ctx, "ls-remote", "--exit-code", "--quiet", cloneURL, ref) // #nosec G204
	logger.Debug("executing: %s", cmd)
	return cmd.Run() == nil
}

// tempCheckoutGitSourceConfigFiles is used for bare clones that don't already
// have working trees. The scanner currently expects certain files to exist
// on the file system for loading additional repo configuration. This creates
// a worktree in the repo that's unique to this scan that can be safely
// deleted after the scan completes. To keep things light, it only checks out
// the relevant config files and not the rest of the tree's content.
func tempCheckoutGitSourceConfigFiles(ctx context.Context, gitDir string) (string, error) {
	worktreePath, err := os.MkdirTemp(gitDir, "leaktk-worktree.")
	if err != nil {
		return "", fmt.Errorf("could not create worktree directory: %w", err)
	}
	cmd := gitCommand(ctx, "-C", gitDir, "worktree", "add", "--no-checkout", worktreePath) // #nosec G204

	logger.Debug("executing: %s", cmd)
	if err := cmd.Run(); err != nil {
		return worktreePath, fmt.Errorf("could not create worktree: %w cmd=%q", err, cmd)
	}

	cmd = gitCommand(ctx, "-C", worktreePath, "checkout", "-f", "HEAD", "--", ".gitleaks*") // #nosec G204
	logger.Debug("executing: %s", cmd)
	if output, err := cmd.CombinedOutput(); err != nil {
		return worktreePath, fmt.Errorf("could not checkout gitleaks files: %w cmd=%q output=%q", err, cmd, string(output))
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
