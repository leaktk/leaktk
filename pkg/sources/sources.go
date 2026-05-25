package sources

import (
	"fmt"

	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/proto"
)

type Source interface {
	ScanRequests(yield func(*proto.Request))
}

func NewSource(srcCfg config.Source) (Source, error) {
	switch srcCfg.Kind {
	case config.GitHubSourceKind:
		return NewGitHub(srcCfg)
	default:
		return nil, fmt.Errorf("unrecognized source kind: kind=%q id=%q", srcCfg.Kind, srcCfg.ID)
	}
}
