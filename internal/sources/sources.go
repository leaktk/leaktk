package sources

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"

	"github.com/leaktk/leaktk/internal/urlutil"
	"github.com/leaktk/leaktk/pkg/logger"
)

type Sources struct {
	values map[string]Source
}

func (ss *Sources) Len() int {
	return len(ss.values)
}

func (ss *Sources) Add(s Source) error {
	if s == nil {
		return errors.New("nil source")
	}

	if len(s.ID()) == 0 {
		return errors.New("missing ID")
	}

	if s.Kind() == UnknownKind {
		return errors.New("unknown Kind")
	}

	if ss.values == nil {
		ss.values = make(map[string]Source, 1)
	}

	es, exists := ss.values[s.ID()]
	if exists {
		if err := es.Merge(s); err != nil {
			return fmt.Errorf("add source: %w", err)
		}
	} else {
		ss.values[s.ID()] = s
	}

	return nil
}

func (ss *Sources) Get(id string) Source {
	return ss.values[id]
}

// SetHeader runs source.SetHeader(context.Context, req.Header) for each
// applicable source in order that they apper in the config.
// NOTE: If more than one source sets the same header, the last one wins.
func (ss *Sources) SetHeader(ctx context.Context, req *http.Request) error {
	for id, s := range ss.values {
		hs, isHeaderSetter := s.(httpHeaderSetter)

		if !isHeaderSetter {
			logger.Debug("skipping source: cannot set headers: source_id=%q", id)
			continue
		}

		if !hs.AppliesTo((*urlutil.URL)(req.URL)) {
			logger.Debug("skipping source: not applicable: source_id=%q", id)
			continue
		}

		if err := hs.SetHeader(ctx, req.Header); err != nil {
			return fmt.Errorf("source could not set header: %w source_id=%q", err, id)
		}
	}

	return nil
}

func (ss *Sources) Login(ctx context.Context, u *urlutil.URL) error {
	for id, s := range ss.values {
		al, isAutoLoginner := s.(autoLoginner)

		if !isAutoLoginner {
			logger.Debug("skipping source: login unsupported: source_id=%q", id)
			continue
		}

		if !al.AppliesTo(u) {
			logger.Debug("skipping source: not applicable: source_id=%q", id)
			continue
		}

		if err := al.Login(ctx); err != nil {
			return fmt.Errorf("login: %w source_id=%q", err, id)
		}

		return nil
	}

	return errors.New("login: no source found")
}

func (ss *Sources) Iter() iter.Seq2[string, Source] {
	return func(yield func(string, Source) bool) {
		for id, s := range ss.values {
			if !yield(id, s) {
				return
			}
		}
	}
}

func (ss *Sources) Load(d any) error {
	sMaps, ok := d.([]map[string]any)
	if !ok {
		return fmt.Errorf("could not load data: expected=\"[]map[string]any\" got=%T", d)
	}

	for i, sm := range sMaps {
		name, ok := sm["kind"].(string)
		if !ok {
			return fmt.Errorf("source kind must be a string: source_index=%d", i)
		}

		kind, ok := KindFromName(name)
		if !ok {
			return fmt.Errorf("unknown kind: kind_name=%q", name)
		}

		s, err := NewSourceFromKind(kind)
		if err != nil {
			return fmt.Errorf("%w: kind_name=%q source_index=%d", err, name, i)
		}

		if err = s.Load(sm); err != nil {
			return fmt.Errorf("source load: %w source_index=%d", err, i)
		}

		if err = ss.Add(s); err != nil {
			return fmt.Errorf("add source: %w source_index=%d", err, i)
		}
	}

	return nil
}

func (ss *Sources) UnmarshalTOML(data any) error {
	if err := ss.Load(data); err != nil {
		return fmt.Errorf("sources load: %w", err)
	}

	return nil
}
