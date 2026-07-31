package facts

import (
	"iter"
	"regexp"

	"github.com/leaktk/leaktk/pkg/logger"
)

// Extractors are grouped by the field name they extract from
type Extractors map[string][]Extractor

type Extractor struct {
	EntityKind string
	Patterns   []*regexp.Regexp
}

func (e Extractor) Extract(s string) iter.Seq2[Kind, string] {
	return func(yield func(Kind, string) bool) {
		for _, p := range e.Patterns {
			subexpNames := p.SubexpNames()
			if len(subexpNames) == 0 {
				logger.Error("skipping pattern: extractor pattern has no subexpressions: pattern=%q", p)
				continue
			}

			factKindValid := false
			factKinds := make([]Kind, len(subexpNames))
			for i, name := range subexpNames {
				if factKinds[i], factKindValid = KindFromName(name); !factKindValid {
					logger.Error("skipping pattern: invalid fact kind name: fact_kind_name=%q pattern=%q", name, p)
					continue
				}
			}

			for _, match := range p.FindAllStringSubmatch(s, -1) {
				for i, value := range match[1:] {
					if len(value) == 0 {
						continue
					}

					if !yield(factKinds[i], value) {
						return
					}
				}
			}
		}
	}
}
