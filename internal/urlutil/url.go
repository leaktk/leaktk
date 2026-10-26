package urlutil

import (
	"net/url"
	"strconv"
)

// URL provides things like text marshing features to url.URL and other url related features
//
// Conventions:
// uu = a short varname for url.URL
// u = a short varname for urlutil.URL
type URL url.URL

func ParseURL(rawURL string) (*URL, error) {
	uu, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	return (*URL)(uu), nil
}

func MustParseURL(rawURL string) *URL {
	uu, err := url.Parse(rawURL)
	if err != nil {
		panic(`sources: ParseURL(` + strconv.Quote(rawURL) + `): ` + err.Error())
	}
	return (*URL)(uu)
}

func (u *URL) UnmarshalText(text []byte) error {
	uu, err := url.Parse(string(text))
	if err != nil {
		return err
	}
	*u = URL(*uu)
	return nil
}

func (u *URL) MarshalText() ([]byte, error) {
	return []byte(u.String()), nil
}

func (u *URL) String() string {
	return (*url.URL)(u).String()
}

func (u *URL) JoinPath(elem ...string) *URL {
	uu := (*url.URL)(u)
	uu = uu.JoinPath(elem...)
	return (*URL)(uu)
}

func (u *URL) Query() url.Values {
	return (*url.URL)(u).Query()
}
