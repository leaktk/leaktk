package collector

import (
	"context"
	"fmt"

	"github.com/leaktk/leaktk/pkg/config"
)

type Collector struct {
}

func NewCollector() *Collector {
	return &Collector{}
}

func (*Collector) Facts(ctx context.Context, src config.Source, yield func(fact Fact, err error) error) error {
	switch src := src.(type) {
	case *config.AtlassianCloudAdminSource:
		return atlassianCloudAdminFacts(ctx, src, yield)
	// TODO:
	// case *config.AtlassianCloudJiraSource:
	// 	return atlassianCloudJiraFacts(ctx, src, yield)
	default:
		return fmt.Errorf("unsupported source kind: kind=%q id=%q", src.Kind(), src.ID())
	}
}
