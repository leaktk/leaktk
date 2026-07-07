package redactor

import (
	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/proto"
	"strings"
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

	for _, result := range response.Results {
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
