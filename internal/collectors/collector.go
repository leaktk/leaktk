package collectors

import (
	"context"
	"errors"
	"fmt"

	"github.com/leaktk/leaktk/internal/facts"
	"github.com/leaktk/leaktk/internal/sources"
)

type Collector struct {
}

func NewCollector() *Collector {
	return &Collector{}
}

func (*Collector) Facts(ctx context.Context, srcs sources.Sources, yield facts.FactYieldFunc) (err error) {
	// Yield fact kind mapping first (entity_id = 0) to avoid passing the strings
	// along for each fact kind
	fact := facts.Fact{EntityID: 0}
	for fk, name := range facts.KindNames {
		fact.Kind = facts.Kind(fk)
		fact.Value = name
		if err = yield(fact); err != nil {
			return err
		}
	}

	// Each function should take this and return the new eidOffset.
	// NOTE: it is fine if EntityIDs are skipped and it may make the code easier
	// to always increment it even if not yielding values
	eidOffset := fact.EntityID
	for _, src := range srcs {
		switch src := src.(type) {
		case *sources.AtlassianCloudAdmin:
			eidOffset, err = atlassianCloudAdminFacts(ctx, src, eidOffset, yield)
		case *sources.AtlassianCloudJira:
			// valid source but nothing to collect here
		default:
			err = errors.New("unsupported source")
		}

		if err != nil {
			return fmt.Errorf("%w src_id=%q src_kind=%q", err, src.ID(), src.Kind())
		}
	}

	return err
}
