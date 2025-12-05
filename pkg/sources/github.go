package sources

import "github.com/leaktk/leaktk/pkg/config"

type GitHub struct {
}

func NewGitHub(srcCfg config.Source) (Source, error) {
	return GitHub{}, nil
}
