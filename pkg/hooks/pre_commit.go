package hooks

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/id"
	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/leaktk/leaktk/pkg/proto"
	"github.com/leaktk/leaktk/pkg/scanner"
)

const preCommitResultsWarningHeader = `
Findings:
`

const preCommitResultWarningTemplate = `
- Description  : %s
  Path         : %s
  Line Number  : %d
  Encoding(s)  : %s
`

const preCommitResultsWarningFooter = `
==============================================================================
COMMIT BLOCKED: POTENTIAL SECRETS DETECTED
------------------------------------------------------------------------------
Please remove any sensitive information listed above and try again.

For excluding non-sensitive findings:
https://github.com/leaktk/leaktk/blob/main/docs/false_positives.md

For more information on interpreting these results:
https://github.com/leaktk/leaktk/blob/main/docs/findings.md
==============================================================================
`

func preCommitRun(cfg *config.Config, hookName string, _ []string) error {
	var wg sync.WaitGroup
	var response *proto.Response

	leaktkScanner := scanner.NewScanner(cfg)
	request := proto.Request{
		ID:       fmt.Sprintf("%s.%s", hookName, id.ID()),
		Kind:     proto.GitRepoRequestKind,
		Resource: ".",
		Opts: proto.Opts{
			Local:  true,
			Staged: true,
		},
	}

	// Prints the output of the scanner as they come
	go leaktkScanner.Recv(func(resp *proto.Response) {
		// Confirm that there is only one response to one request;
		// anything other than that would be a bug.
		if response != nil {
			logger.Fatal("unexpected additional response returned during scan: id=%q", resp.ID)
		}

		response = resp
		wg.Done()
	})

	wg.Add(1)
	leaktkScanner.Send(&request)
	wg.Wait()
	leaksFound := len(response.Results) > 0

	// Display any results if found before doing error handling to show
	// partial results if they exist
	if leaksFound {
		preCommitDisplayResults(response.Results)
	}

	// Exit with non-zero code if the response had an error or if leaks were found
	if response.Error != nil {
		logger.Fatal("response contains error: %w", response.Error)
	}
	if leaksFound {
		os.Exit(1)
	}

	return nil
}

func preCommitResultEncodings(result *proto.Result) string {
	var encodings strings.Builder

	encodingPrefix := "decoded:"
	encodingPrefixLen := len(encodingPrefix)
	tags := result.Rule.Tags

	for _, tag := range tags {
		if strings.HasPrefix(tag, encodingPrefix) {
			if encodings.Len() > 0 {
				encodings.WriteString(", ")
			}

			encodings.WriteString(tag[encodingPrefixLen:])
		}
	}

	return encodings.String()
}

func preCommitDisplayResults(results []*proto.Result) {
	fmt.Fprint(os.Stderr, preCommitResultsWarningHeader)
	for _, result := range results {
		fmt.Fprintf(
			os.Stderr,
			preCommitResultWarningTemplate,
			result.Rule.Description,
			result.Location.Path,
			result.Location.Start.Line,
			preCommitResultEncodings(result),
		)
	}
	fmt.Fprint(os.Stderr, preCommitResultsWarningFooter)
}
