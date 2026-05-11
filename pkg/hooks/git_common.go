package hooks

import (
	"fmt"
	"os"
	"strings"

	"github.com/leaktk/leaktk/pkg/docs"
	"github.com/leaktk/leaktk/pkg/proto"
)

const gitHookResultsWarningHeader = `
Findings:
`

const gitHookResultWarningTemplate = `
- Description  : %s
  Commit:      : %s
  Path         : %s
  Line Number  : %d
  Encoding(s)  : %s
`

const gitHookResultsWarningFooter = `
==============================================================================
COMMIT BLOCKED: POTENTIAL SECRETS DETECTED
------------------------------------------------------------------------------
Please remove any sensitive information listed above and try again.

For excluding non-sensitive findings:
%s

For more information on interpreting these results:
%s
==============================================================================
`

func gitHookResultEncodings(result *proto.Result) string {
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

func gitHookDisplayResults(results []*proto.Result) {
	fmt.Fprint(os.Stderr, gitHookResultsWarningHeader)
	for _, result := range results {
		fmt.Fprintf(
			os.Stderr,
			gitHookResultWarningTemplate,
			result.Rule.Description,
			result.Location.Version,
			result.Location.Path,
			result.Location.Start.Line,
			gitHookResultEncodings(result),
		)
	}

	fmt.Fprintf(
		os.Stderr,
		gitHookResultsWarningFooter,
		docs.DocURL(docs.FalsePositivesTopic),
		docs.DocURL(docs.FindingsTopic),
	)
}
