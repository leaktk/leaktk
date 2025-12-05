package monitor

import "github.com/leaktk/leaktk/pkg/config"

type Source interface {
}

func NewSource(cfg config.Source) Source {
	return nil
}
