package auth

import "github.com/pkg/browser"

func OpenBrowser(url string) error {
	return browser.OpenURL(url)
}
