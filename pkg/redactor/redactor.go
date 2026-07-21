package redactor

import (
	"sort"
	"strings"

	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/proto"
)

type Redactor struct {
	RedactionMark string
	RedactionWord string
}

type Span struct {
	Start int
	End   int
}

func NewRedactor(cfg *config.Config) *Redactor {
	return &Redactor{
		RedactionMark: cfg.Redactor.RedactionMark,
		RedactionWord: cfg.Redactor.RedactionWord,
	}
}

func computeLineOffsets(s string) []int {
	offsets := []int{0}
	offset := 1
	n := len(s)
	for offset < n {
		i := strings.IndexByte(s[offset:], '\n')
		if i < 0 {
			break
		}
		offset += i + 1
		offsets = append(offsets, offset)
	}

	return offsets
}

func positionToOffset(lineStarts []int, line, column int) int {
	if line <= 0 || line > len(lineStarts) {
		return 0
	}

	return lineStarts[line-1] + (column - 1)
}

func mergeSpans(spans []Span) []Span {
	if len(spans) == 0 {
		return spans
	}

	sort.Slice(spans, func(i, j int) bool {
		return spans[i].Start < spans[j].Start
	})

	merged := make([]Span, 0, len(spans))
	merged = append(merged, spans[0])

	var i int
	i = 0

	for _, s := range spans[1:] {
		if s.Start < merged[i].End {
			if s.End > merged[i].End {
				merged[i].End = s.End
			}
			continue
		}
		merged = append(merged, s)
		i++
	}

	return merged
}

func (r *Redactor) RedactText(resource string, response *proto.Response) (string, error) {
	if len(response.Results) == 0 {
		return resource, nil
	}

	lineStarts := computeLineOffsets(resource)

	spans := make([]Span, 0, len(response.Results))

	for _, result := range response.Results {
		start := positionToOffset(lineStarts, result.Location.Start.Line, result.Location.Start.Column)
		end := positionToOffset(lineStarts, result.Location.End.Line, result.Location.End.Column) + 1

		if start < 0 {
			start = 0
		}

		if end > len(resource) {
			end = len(resource)
		}

		if start >= end {
			continue
		}

		spans = append(spans, Span{
			Start: start,
			End:   end,
		})
	}

	if len(spans) == 0 {
		return resource, nil
	}

	spans = mergeSpans(spans)

	var b strings.Builder
	b.Grow(len(resource))

	cursor := 0

	for _, s := range spans {
		b.WriteString(resource[cursor:s.Start])

		if r.RedactionWord != "" {
			b.WriteString(r.RedactionWord)
		} else {
			mark := r.RedactionMark

			b.WriteString(strings.Repeat(mark, s.End-s.Start))
		}

		cursor = s.End
	}

	b.WriteString(resource[cursor:])

	return b.String(), nil
}
