package redactor

import (
	"strings"
	"sort"

	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/proto"
)

type Redactor struct {
	RedactionMark string
	RedactionWord string
}

func NewRedactor(cfg *config.Config) *Redactor {
	redactor := &Redactor{
		RedactionMark: cfg.Redactor.RedactionMark,
		RedactionWord: cfg.Redactor.RedactionWord,
	}

	return redactor
}

func (r *Redactor) RedactText(resource string, response *proto.Response) (string, error) {
	mark := r.RedactionMark
	word := r.RedactionWord

	if len(mark) == 0 && len(word) == 0 {
		mark = "*"
	}

	if len(response.Results) == 0 {
		return resource, nil
	}
	
	results := response.Results
	sort.Slice(results, func(i, j int) bool {
		return len(results[i].Secret) > len(results[j].Secret)
	})
	for _, result := range results {
		if len(result.Secret) == 0 {
			continue
		}

		if word != "" {
			resource = strings.ReplaceAll(resource, result.Secret, word)
		} else {
			mask := strings.Repeat(mark, len(result.Secret))
			resource = strings.ReplaceAll(resource, result.Secret, mask)
		}
	}
	return resource, nil
}
