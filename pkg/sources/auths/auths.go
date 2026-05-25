package auths

import (
	"fmt"
)

type BearerToken struct {
	Kind  string
	Token string
}

func NewAuth(Auth map[string]string) (BearerToken, error) {
	if Auth["kind"] != "BearerToken" {
		return BearerToken{}, fmt.Errorf("unrecognised auth kind: kind=%q", Auth["kind"])
	}

	return BearerToken{Auth["kind"], Auth["token"]}, nil
}
