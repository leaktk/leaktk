package sources

import (
	"context"
	"net/http"

	"github.com/leaktk/leaktk/internal/urlutil"
)

type Source interface {
	ID() string
	Kind() Kind
	Load(data any) error
	Merge(o Source) error
}

type httpHeaderSetter interface {
	SetHeader(ctx context.Context, h http.Header) error
	AppliesTo(u *urlutil.URL) bool
}

type autoLoginner interface {
	Login(ctx context.Context) error
	AppliesTo(u *urlutil.URL) bool
}
