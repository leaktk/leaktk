package redactor

import (
	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/proto"
)

type Redactor struct {
	Config *config.Config
}

func NewRedactor(cfg *config.Config) *Redactor {
	return &Redactor{
		Config: cfg,
	}
}

func (r *Redactor) RedactText(resource string, response *proto.Response) (string, error) {
	// Redaction code here

	return "", nil
}
