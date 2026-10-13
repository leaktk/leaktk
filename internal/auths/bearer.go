package auths

import (
	"context"
	"net/http"
)

type BearerAuth struct {
	Token string `toml:"token"`
}

func (a *BearerAuth) SetHeader(ctx context.Context, h http.Header) error {
	h.Set("Authorization", "Bearer "+a.Token)
	return nil
}
