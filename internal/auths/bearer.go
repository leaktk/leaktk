package auths

import (
	"context"
	"fmt"
	"net/http"
)

type BearerAuth struct {
	Token string `toml:"token"`
}

func (a *BearerAuth) Load(d any) error {
	m, ok := d.(map[string]any)
	if !ok {
		return fmt.Errorf("could not load data: expected=\"map[string]any\" got=%T", d)
	}

	for k, v := range m {
		switch k {
		case "token":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("%s must be a string", k)
			}
			a.Token = s
		}
	}

	return nil
}

func (a *BearerAuth) SetHeader(ctx context.Context, h http.Header) error {
	h.Set("Authorization", "Bearer "+a.Token)
	return nil
}

func (a *BearerAuth) Merge(o *BearerAuth) error {
	if len(o.Token) != 0 {
		*a = *o
	}
	return nil
}
