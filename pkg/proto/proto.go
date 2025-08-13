package proto

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/leaktk/leaktk/pkg/logger"
)

const (
	ScanResultsResponseKind = "ScanResults"
)

// RequestKind provides an enum for setting Kind on request
type RequestKind int

const (
	ContainerImageRequestKind RequestKind = iota
	FilesRequestKind
	GitRepoRequestKind
	JSONDataRequestKind
	TextRequestKind
	URLRequestKind
)

var requestKindNames = []string{
	"ContainerImage",
	"Files",
	"GitRepo",
	"JSONData",
	"Text",
	"URL",
}

func (k RequestKind) String() string {
	if int(k) > -1 && int(k) < len(requestKindNames) {
		return requestKindNames[int(k)]
	}

	return ""
}

var requestKindNameMap = map[string]RequestKind{
	"ContainerImage": ContainerImageRequestKind,
	"Files":          FilesRequestKind,
	"GitRepo":        GitRepoRequestKind,
	"JSONData":       JSONDataRequestKind,
	"Text":           TextRequestKind,
	"URL":            URLRequestKind,
}

// Request is a request to LeakTK
type Request struct {
	ID       string      `json:"id"       toml:"id"       yaml:"id"`
	Kind     RequestKind `json:"kind"     toml:"kind"     yaml:"kind"`
	Resource string      `json:"resource" toml:"resource" yaml:"resource"`
	Opts     Opts        `json:"options"  toml:"options"  yaml:"options"`
}

// UnmarshalJSON sets r to a copy of data
func (r *Request) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("Request: UnmarshalJSON on nil pointer")
	}

	var tmp struct {
		ID       string `json:"id"`
		Kind     string `json:"kind"`
		Resource string `json:"resource"`
		Opts     Opts   `json:"options"`
	}

	if err := json.Unmarshal(data, &tmp); err != nil {
		logger.Debug("Request:\n%v", data)

		return fmt.Errorf("could not unmarshal request: %w", err)
	}

	if kind, isValidKind := requestKindNameMap[tmp.Kind]; isValidKind {
		r.ID = tmp.ID
		r.Kind = kind
		r.Resource = tmp.Resource
		r.Opts = tmp.Opts

		return nil
	}

	return fmt.Errorf("unsupported request kind: kind=%q", tmp.Kind)
}

// Error for returning in the response instead of results if there was a
// critical error causing the scan to fail
type Error struct {
	Code    int    `json:"code"           toml:"code"           yaml:"code"`
	Message string `json:"message"        toml:"message"        yaml:"message"`
	Data    any    `json:"data,omitempty" toml:"data,omitempty" yaml:"data,omitempty"`
}

// Error implements go's error interface for Response.Error
func (e *Error) Error() string {
	return fmt.Sprintf("%s code=%d", e.Message, e.Code)
}

// Response from the scanner with the scan result
type Response struct {
	ID        string    `json:"id"              toml:"id"              yaml:"id"`
	Kind      string    `json:"kind"            toml:"kind"            yaml:"kind"`
	RequestID string    `json:"request_id"      toml:"request_id"      yaml:"request_id"`
	Results   []*Result `json:"results"         toml:"results"         yaml:"results"`
	Error     *Error    `json:"error,omitempty" toml:"error,omitempty" yaml:"error,omitempty"`
}

// Opts for the different scan types; not all apply to each scan type
type Opts struct {
	Arch       string   `json:"arch"`
	Branch     string   `json:"branch"`
	Depth      int      `json:"depth"`
	Exclusions []string `json:"exclusions"`
	FetchURLs  string   `json:"fetch_urls"`
	Local      bool     `json:"local"`
	Priority   int      `json:"priority"`
	Proxy      string   `json:"proxy"`
	Since      string   `json:"since"`
	Staged     bool     `json:"staged"`
	Unstaged   bool     `json:"unstaged"`
}

// In the future we might have things like GitCommitMessage
// GithubPullRequest, etc
const (
	GenericResultKind          = "Generic"
	ContainerLayerResultKind   = "ContainerLayer"
	ContainerMetdataResultKind = "ContainerMetdata"
	GitCommitResultKind        = "GitCommit"
)

// Result of a scan
type Result struct {
	ID       string            `json:"id"       toml:"id"       yaml:"id"`
	Kind     string            `json:"kind"     toml:"kind"     yaml:"kind"`
	Secret   string            `json:"secret"   toml:"secret"   yaml:"secret"`
	Match    string            `json:"match"    toml:"match"    yaml:"match"`
	Context  string            `json:"context"  toml:"context"  yaml:"context"`
	Entropy  float32           `json:"entropy"  toml:"entropy"  yaml:"entropy"`
	Date     string            `json:"date"     toml:"date"     yaml:"date"`
	Rule     Rule              `json:"rule"     toml:"rule"     yaml:"rule"`
	Contact  Contact           `json:"contact"  toml:"contact"  yaml:"contact"`
	Location Location          `json:"location" toml:"location" yaml:"location"`
	Notes    map[string]string `json:"notes"    toml:"notes"    yaml:"notes"`
}

// Rule that triggered the result
type Rule struct {
	ID          string   `json:"id" toml:"id" yaml:"id"`
	Description string   `json:"description" toml:"description" yaml:"description"`
	Tags        []string `json:"tags" toml:"tags" yaml:"tags"`
}

// Contact for some resource when available
type Contact struct {
	Name  string `json:"name"  toml:"name"  yaml:"name"`
	Email string `json:"email" toml:"email" yaml:"email"`
}

// Location in the specific resource being scanned
type Location struct {
	Version string `json:"version" toml:"version" yaml:"version"`
	Path    string `json:"path"    toml:"path"    yaml:"path"`
	Start   Point  `json:"start"   toml:"start"   yaml:"start"`
	End     Point  `json:"end"     toml:"end"     yaml:"end"`
}

// Point just provides line & column coordinates for a Result in a text file
type Point struct {
	Line   int `json:"line"   toml:"line"   yaml:"line"`
	Column int `json:"column" toml:"column" yaml:"column"`
}
