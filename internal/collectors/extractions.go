package collectors

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/leaktk/leaktk/pkg/config"
)

type compiledExtraction struct {
	attribute string
	pattern   *regexp.Regexp
	kind      string
	captures  []captureMapping
}

type captureMapping struct {
	subexpIndex int
	factKind    Kind
}

func compileExtractions(extractions []config.Extraction) ([]compiledExtraction, error) {
	compiled := make([]compiledExtraction, 0, len(extractions))
	for _, ext := range extractions {
		var captures []captureMapping
		for i, name := range ext.Pattern.SubexpNames() {
			if name == "" {
				continue
			}
			fk, ok := factKindByName(name)
			if !ok {
				return nil, fmt.Errorf("unknown fact kind in extraction capture group: %q", name)
			}
			captures = append(captures, captureMapping{subexpIndex: i, factKind: fk})
		}

		sort.Slice(captures, func(i, j int) bool {
			return captures[i].factKind < captures[j].factKind
		})

		compiled = append(compiled, compiledExtraction{
			attribute: ext.Attribute,
			pattern:   ext.Pattern,
			kind:      ext.Kind,
			captures:  captures,
		})
	}
	return compiled, nil
}

func extract(_ context.Context, parentEID, eidOffset uint32, extractions []compiledExtraction, text, sourceID string, yield FactYieldFunc) (uint32, error) {
	var err error
	fact := Fact{Timestamp: time.Now().Unix()}

	for _, ext := range extractions {
		for _, match := range ext.pattern.FindAllStringSubmatch(text, -1) {
			childEID := eidOffset
			eidOffset++

			fact.EntityID = childEID
			err = yieldKV(fact, EntityKindKind, ext.kind, yield, err)
			for _, cap := range ext.captures {
				if cap.subexpIndex < len(match) && len(match[cap.subexpIndex]) > 0 {
					err = yieldKV(fact, cap.factKind, match[cap.subexpIndex], yield, err)
				}
			}
			err = yieldKV(fact, SourceIDKind, sourceID, yield, err)

			fact.EntityID = parentEID
			err = yieldKV(fact, RelatedEntityIDKind, strconv.FormatUint(uint64(childEID), 10), yield, err)

			if err != nil {
				return eidOffset, fmt.Errorf("yield extraction facts: %w", err)
			}
		}
	}

	return eidOffset, nil
}
