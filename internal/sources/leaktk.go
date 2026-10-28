package sources

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/leaktk/leaktk/internal/auths"
	"github.com/leaktk/leaktk/internal/httpclient"
	"github.com/leaktk/leaktk/internal/urlutil"
)

type LeakTKServer struct {
	id             string
	BaseURL        urlutil.URL
	RateLimit      *httpclient.RateLimit
	auths.OIDCAuth // imlements httpHeaderSetter
	Save           func(s *LeakTKServer) error
}

func NewLeakTKServer(id string) *LeakTKServer {
	return &LeakTKServer{id: id, RateLimit: httpclient.NewRateLimit()}
}

// Load allows filling out details from the server from the provided data
// (see switch cases for details on supported data types).
func (s *LeakTKServer) Load(d any) error {
	m, ok := d.(map[string]any)
	if !ok {
		return fmt.Errorf("could not load data: expected=\"map[string]any\" got=%T", d)
	}

	for k, v := range m {
		switch k {
		case "id":
			id, ok := v.(string)
			if !ok {
				return errors.New("id must be a string")
			}
			s.id = id
		case "base_url":
			rawURL, ok := v.(string)
			if !ok {
				return errors.New("base_url must be a string")
			}
			err := s.BaseURL.UnmarshalText([]byte(rawURL))
			if err != nil {
				return fmt.Errorf("invalid base_url: %w base_url=%q", err, rawURL)
			}
		}
	}

	return s.OIDCAuth.Load(m)
}

func (s *LeakTKServer) Dump() map[string]string {
	m := map[string]string{
		"id":       s.id,
		"base_url": s.BaseURL.String(),
		"kind":     s.Kind().String(),
	}

	for k, v := range s.OIDCAuth.Dump() {
		m[k] = v
	}

	return m
}

func (s *LeakTKServer) ID() string {
	return s.id
}

func (s *LeakTKServer) Kind() Kind {
	return LeakTKServerKind
}

func (s *LeakTKServer) Merge(o Source) error {
	oLTKS, ok := o.(*LeakTKServer)
	if !ok {
		return fmt.Errorf("cannot merge %s sources into %s", o.Kind(), s.Kind())
	}

	if len(oLTKS.BaseURL.Host) != 0 {
		s.BaseURL = oLTKS.BaseURL
	}

	if oLTKS.Save != nil {
		s.Save = oLTKS.Save
	}

	return s.OIDCAuth.Merge(&oLTKS.OIDCAuth)
}

func (s *LeakTKServer) AppliesTo(u *urlutil.URL) bool {
	return strings.EqualFold(s.BaseURL.Host, u.Host) && strings.HasPrefix(u.Path, s.BaseURL.Path)
}

func (s *LeakTKServer) Login(ctx context.Context) error {
	var err error

	if len(s.Issuer) == 0 {
		resp, err := httpclient.NewClient().Get(s.BaseURL.JoinPath("userinfo").String())
		if err != nil {
			return fmt.Errorf("discover issuer: %w", err)
		}
		_ = resp.Body.Close()
		if h := resp.Header.Get("WWW-Authenticate"); len(h) != 0 {
			if err = s.OIDCAuth.FromHeader(h); err != nil {
				return fmt.Errorf("discover issuer: www-authenticate header: %w", err)
			}
		}

		if len(s.Issuer) == 0 {
			return errors.New("discover issuer: not found")
		}
	}

	err = s.OIDCAuth.Login(ctx)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}

	// If source has a save callback, save the credentials after login
	if s.Save == nil {
		return nil
	}
	return s.Save(s)
}
