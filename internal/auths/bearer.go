package auths

import "net/http"

type BearerAuth struct {
	Token string `toml:"token"`
}

func (a *BearerAuth) SetHeader(h http.Header) error {
	h.Set("Authorization", "Bearer "+a.Token)
	return nil
}
