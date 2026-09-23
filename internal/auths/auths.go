package auths

import "net/http"

type HTTPAuth interface {
	SetHeader(h http.Header) error
}
