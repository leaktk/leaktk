package collectors

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/leaktk/leaktk/internal/facts"
	"github.com/leaktk/leaktk/pkg/config"
)

type Collector struct {
}

func NewCollector() *Collector {
	return &Collector{}
}

func (*Collector) Facts(ctx context.Context, srcs config.Sources, yield FactYieldFunc) (err error) {
	// Yield fact kind mapping first (entity_id = 0)
	// This is so we don't have to pass the strings along for each fact kind
	fact := Fact{EntityID: 0, Timestamp: time.Now().Unix()}
	for fk, name := range facts.KindNames {
		fact.Kind = facts.Kind(fk)
		fact.Value = name
		if err = yield(fact); err != nil {
			return err
		}
	}

	eidOffset := uint32(1) // start entity IDs at this number
	for _, src := range srcs {
		switch src := src.(type) {
		case *config.AtlassianCloudAdminSource:
			eidOffset, err = atlassianCloudAdminFacts(ctx, src, eidOffset, yield)
		case *config.AtlassianCloudJiraSource:
			// TODO
			// eidOffset, err = atlassianCloudJiraFacts(ctx, src, eidOffset, yield)
			err = errors.New("unsupported source")
		case *config.LDAPSource:
			eidOffset, err = ldapFacts(ctx, src, eidOffset, yield)
		default:
			err = errors.New("unsupported source")
		}

		if err != nil {
			return fmt.Errorf("%w src_id=%q src_kind=%q", err, src.ID(), src.Kind())
		}
	}

	return err
}
