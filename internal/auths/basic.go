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

// Auth pairs should be merged with each other
func (a *BasicAuth) Merge(o *BasicAuth) error {
	if len(o.Username) != 0 || len(o.Password) != 0 {
		*a = *o
	}

	return nil
}
