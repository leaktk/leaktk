package docs

import (
	"os"
	"strings"
)

// BaseDocsURL is the URL that will be used to find docs by topic.
// This can be overridden if providing custom docs.
var BaseDocsURL = "https://github.com/leaktk/leaktk/blob/HEAD/docs/"

// DocsExt is the extension added on to the doc topic by DocURL.
// This can be overridden if providing custom docs.
var DocsExt = ".md"

// Topic is a topic that can be referenced in the docs.
// This is used to generate the URL for the topic.
type Topic string

// A list of topics to referenc
const (
	CommandNotFoundTopic = Topic("error_command_not_found")
	FalsePositivesTopic  = Topic("false_positives")
	FindingsTopic        = Topic("findings")
)

func init() {
	// Look up LEAKTK_DOCS_BASE_URL environment variable
	if base := strings.TrimSpace(os.Getenv("LEAKTK_DOCS_BASE_URL")); base != "" {
		BaseDocsURL = base
	}
	// Look up LEAKTK_DOCS_EXT environment variable
	if ext := strings.TrimSpace(os.Getenv("LEAKTK_DOCS_EXT")); ext != "" {
		DocsExt = ext
	}
	// Make sure the base URL ends with a slash
	if BaseDocsURL[len(BaseDocsURL)-1] != '/' {
		BaseDocsURL += "/"
	}
}

func DocURL(topic Topic) string {
	return BaseDocsURL + string(topic) + DocsExt
}
