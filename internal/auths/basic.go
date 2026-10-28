package auths

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
)

type BasicAuth struct {
	Username string `toml:"username"`
	Password string `toml:"password"`
}

func (a *BasicAuth) SetHeader(ctx context.Context, h http.Header) error {
	value := "Basic " + base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", a.Username, a.Password)))
	h.Set("Authorization", value)
	return nil
}

func (a *BasicAuth) Load(d any) error {
	m, ok := d.(map[string]any)
	if !ok {
		return fmt.Errorf("could not load data: expected=\"map[string]any\" got=%T", d)
	}

	for k, v := range m {
		switch k {
		case "username":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("%s must be a string", k)
			}
			a.Username = s
		case "password":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("%s must be a string", k)
			}
			a.Password = s
		}
	}
	return nil
}

// Auth pairs should be merged with each other
func (a *BasicAuth) Merge(o *BasicAuth) error {
	if len(o.Username) != 0 || len(o.Password) != 0 {
		*a = *o
	}

	return nil
}
