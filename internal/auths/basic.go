package auths

import (
	"encoding/base64"
	"fmt"
	"net/http"
)

type BasicAuth struct {
	Username string `toml:"username"`
	Password string `toml:"password"`
}

func (a *BasicAuth) SetHeader(h http.Header) error {
	value := "Basic " + base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", a.Username, a.Password)))
	h.Set("Authorization", value)
	return nil
}
