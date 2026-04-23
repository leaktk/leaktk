package docs

import (
	"strings"

	"github.com/leaktk/leaktk/pkg/version"
)

// BaseDocsURL is the URL that will be used to find docs by topic. It can contain
// the following variables that will be substituted by the DocURL function:
//
// - ${GIT_REF} - the git-ref of the current scanner build to pin to spcific docs
//
// This can be overridden if providing custom docs
var BaseDocsURL = "https://github.com/leaktk/leaktk/blob/${GIT_REF}/docs/"

// DocsExt is the extension added on to the doc topic by DocURL
// This can be overridden if providing custom docs
var DocsExt = ".md"

type Topic string

// A list of topics to referenc
const (
	CommandNotFoundTopic = Topic("errors/command_not_found")
	FalsePositivesTopic  = Topic("false_positives")
	FindingsTopic        = Topic("false_positives")
)

func DocURL(topic Topic) string {
	var b strings.Builder
	b.WriteString(BaseDocsURL)
	// Make sure there's a trailing slash
	if BaseDocsURL[len(BaseDocsURL)-1] != '/' {
		b.WriteByte('/')
	}
	b.WriteString(string(topic))
	b.WriteString(DocsExt)
	docURL := b.String()
	if strings.Contains(docURL, "${GIT_REF}") {
		// Get the git ref that this was build for
		gitRef := version.Commit
		if len(gitRef) == 0 {
			gitRef = "HEAD"
		}
		docURL = strings.ReplaceAll(docURL, "${GIT_REF}", gitRef)
	}
	return docURL
}
