package scanner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	bl "github.com/leaktk/leaktk/internal/betterleaks"

	"github.com/leaktk/leaktk/internal/fs"
	"github.com/leaktk/leaktk/internal/git"
	"github.com/leaktk/leaktk/internal/httpclient"
	"github.com/leaktk/leaktk/internal/sources"
	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/id"
	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/leaktk/leaktk/pkg/proto"
	"github.com/leaktk/leaktk/pkg/queue"
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
	clonesDir       string
	maxArchiveDepth int
	maxDecodeDepth  int
	maxScanDepth    int
	patterns        *Patterns
	rateLimit       *httpclient.RateLimit
	responseQueue   *queue.PriorityQueue[*proto.Response]
	scanQueue       *queue.PriorityQueue[*proto.Request]
	scanTimeout     time.Duration
	scanWorkers     int
	sources         sources.Sources
}

// NewScanner returns a initialized and listening scanner instance that should
// be closed when it's no longer needed.
func NewScanner(cfg *config.Config) *Scanner {
	scanner := &Scanner{
		allowLocal:      cfg.Scanner.AllowLocal,
		clonesDir:       filepath.Join(cfg.Scanner.Workdir, "clones"),
		maxArchiveDepth: cfg.Scanner.MaxArchiveDepth,
		maxDecodeDepth:  cfg.Scanner.MaxDecodeDepth,
		maxScanDepth:    cfg.Scanner.MaxScanDepth,
		patterns:        NewPatterns(&cfg.Scanner.Patterns, httpclient.NewClient()),
		rateLimit:       httpclient.NewRateLimit(),
		responseQueue:   queue.NewPriorityQueue[*proto.Response](initQueueCapacity, cfg.Scanner.MaxResponseQueueSize),
		scanQueue:       queue.NewPriorityQueue[*proto.Request](initQueueCapacity, cfg.Scanner.MaxScanQueueSize),
		scanTimeout:     time.Duration(cfg.Scanner.ScanTimeout) * time.Second,
		scanWorkers:     cfg.Scanner.ScanWorkers,
		sources:         cfg.Sources,
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

		// Copy so loadSourceConfig mutations don't affect other scans
		cfgCopy := *cfg
		blScanner, err := bl.NewScanner(ctx, &cfgCopy, bl.ScannerOpts{
			MaxArchiveDepth: s.maxArchiveDepth,
			MaxDecodeDepth:  s.maxDecodeDepth,
		})

		var results []*proto.Result
		switch request.Kind {
		case proto.GitRepoRequestKind:
			var gitRepoInfo git.RepoInfo

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
				gitRepoInfo, err = git.GetRepoInfo(ctx, request.Resource)
				if err != nil {
					logger.Critical("scan failed: could not get git repo info: %v id=%q", err, request.ID)
					removeTempGitFiles(request, gitRepoInfo)
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
						removeTempGitFiles(request, gitRepoInfo)
						s.respondWithError(request, &proto.Error{
							Code:    cloneErrorCode,
							Message: "clone operation timed out",
							Data:    request,
						})
					default:
						logger.Critical("scan failed: could not clone git repo: %v id=%q", err, request.ID)
						removeTempGitFiles(request, gitRepoInfo)
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
				gitRepoInfo.WorkingTreePath, err = tempCheckoutGitSourceConfigFiles(ctx, gitRepoInfo.GitDir, request.Opts.Branch)
				if err != nil {
					// Only log this as a debug item since it shouldn't result in fewer findings but
					// may result in more false positives
					logger.Debug("could not set up temp working tree for bare repo: %v id=%q", err, request.ID)
				}
			}

			// Load the checked out config from the working tree
			bl.LoadSourceConfig(blScanner, gitRepoInfo.WorkingTreePath)

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

			results, err = bl.ScanGit(ctx, request, blScanner, gitRepoInfo.GitDir, bl.GitScanOpts{
				RevisionRange: revisionRange,
				Depth:         scanDepth(request.Opts.Depth, s.maxScanDepth),
				Since:         request.Opts.Since,
				Staged:        request.Opts.Staged,
				Unstaged:      request.Opts.Unstaged,
			})

			// Remove temp files as soon as they're no longer needed
			removeTempGitFiles(request, gitRepoInfo)
		case proto.URLRequestKind:
			results, err = bl.ScanURL(ctx, request, blScanner, request.Resource, bl.URLScanOpts{
				FetchURLPatterns: splitFetchURLPatterns(request.Opts.FetchURLs),
				Sources:          s.sources,
				RateLimit:        s.rateLimit,
			})
		case proto.JSONDataRequestKind:
			results, err = bl.ScanJSON(ctx, request, blScanner, request.Resource, bl.JSONScanOpts{
				FetchURLPatterns: splitFetchURLPatterns(request.Opts.FetchURLs),
				Sources:          s.sources,
				RateLimit:        s.rateLimit,
			})
		case proto.TextRequestKind:
			results, err = bl.ScanReader(ctx, request, blScanner, strings.NewReader(request.Resource))
		case proto.StdinRequestKind:
			results, err = bl.ScanReader(ctx, request, blScanner, os.Stdin)
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
			bl.LoadSourceConfig(blScanner, request.Resource)
			results, err = bl.ScanFiles(ctx, request, blScanner, request.Resource)
		case proto.ContainerImageRequestKind:
			results, err = bl.ScanContainerImage(ctx, request, blScanner, request.Resource, bl.ContainerImageScanOpts{
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

		logger.Info("queueing response: id=%q queue_size=%d", request.ID, s.responseQueue.Size()+1)
		s.responseQueue.Send(&queue.Message[*proto.Response]{
			Priority: msg.Priority,
			Value: &proto.Response{
				ID:        id.ID(),
				Kind:      proto.ScanResultsResponseKind,
				RequestID: request.ID,
				Error:     scanErr,
				Results:   results,
				Resource:  request.Resource,
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

// removeTempGitFiles clears out any temp files or directories that were created for the scan
// and should be safe to remove after the scan is finished
func removeTempGitFiles(request *proto.Request, gitRepoInfo git.RepoInfo) {
	// Remove temp repo clone if it was a remote scan
	if !request.Opts.Local && fs.PathExists(gitRepoInfo.GitDir) {
		logger.Debug("removing temp git dir: path=%q", gitRepoInfo.GitDir)
		if err := os.RemoveAll(gitRepoInfo.GitDir); err != nil {
			logger.Error("could not remove temp git dir: %v path=%q id=%q", err, gitRepoInfo.GitDir, request.ID)
		}
	}

	// Remove temp git working tree created for accessing certain files from bare repos
	if gitRepoInfo.IsBare && fs.PathExists(gitRepoInfo.WorkingTreePath) {
		if err := os.RemoveAll(gitRepoInfo.WorkingTreePath); err != nil {
			logger.Error("error removing temp working tree: %v path=%q id=%q", err, gitRepoInfo.WorkingTreePath, request.ID)
		}
	}
}

func (s *Scanner) cloneGitRepo(ctx context.Context, cloneURL string, opts proto.Opts) (git.RepoInfo, error) {
	cloneArgs := []string{"clone"}
	gitRepoInfo := git.RepoInfo{}

	if len(opts.Proxy) > 0 {
		cloneArgs = append(cloneArgs, "--config")
		cloneArgs = append(cloneArgs, "http.proxy="+opts.Proxy)
	}

	// The --[no-]single-branch flags are still needed with mirror due to how
	// things like --depth and --shallow-since behave
	if len(opts.Branch) > 0 {
		if !git.RemoteRefExists(ctx, cloneURL, opts.Branch) {
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
	gitClone := git.CommandContext(ctx, cloneArgs...)
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

// tempCheckoutGitSourceConfigFiles is used for bare clones that don't already
// have working trees. The scanner currently expects certain files to exist
// on the file system for loading additional repo configuration. This creates
// a worktree in the repo that's unique to this scan that can be safely
// deleted after the scan completes. To keep things light, it only checks out
// the relevant config files and not the rest of the tree's content.
func tempCheckoutGitSourceConfigFiles(ctx context.Context, gitDir, gitRef string) (string, error) {
	worktreePath, err := os.MkdirTemp(gitDir, "leaktk-worktree.")
	if err != nil {
		return "", fmt.Errorf("could not create worktree directory: %w", err)
	}
	if len(gitRef) == 0 {
		gitRef = "HEAD"
	}
	cmd := git.CommandContext(ctx, "-C", gitDir, "--work-tree", worktreePath, "restore", "--source", gitRef, ".betterleaks*", ".gitleaks*")
	logger.Debug("executing: %s", cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return worktreePath, fmt.Errorf("could not checkout scanner config files: %w cmd=%q (%s)", err, cmd, string(out))
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
