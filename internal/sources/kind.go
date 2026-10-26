package sources

import (
	"errors"
	"fmt"

	"github.com/leaktk/leaktk/internal/httpclient"
)

type Kind int

const (
	UnknownKind Kind = iota
	AtlassianCloudAdminKind
	AtlassianCloudJiraKind
	LeakTKServerKind
)

var KindNames = []string{
	"Unknown",
	"AtlassianCloudAdmin",
	"AtlassianCloudJira",
	"LeakTKServer",
}

var kindNameMap = make(map[string]Kind, len(KindNames))

func init() {
	for i, name := range KindNames {
		kindNameMap[name] = Kind(i)
	}
}

func (k Kind) ID() int {
	return int(k)
}

func (k Kind) String() string {
	if int(k) < 0 || int(k) > len(KindNames) {
		return "<invalid>"
	}
	return KindNames[k]
}

func (k *Kind) UnmarshalText(text []byte) error {
	var ok bool
	*k, ok = kindNameMap[string(text)]
	if !ok {
		return fmt.Errorf("invalid source kind: kind=%q", string(text))
	}

	if *k == UnknownKind {
		return errors.New("missing source kind")
	}

	return nil
}

func KindFromName(name string) (k Kind, ok bool) {
	k, ok = kindNameMap[name]
	if k == UnknownKind {
		ok = false
	}
	return
}

func NewSourceFromKind(kind Kind) (Source, error) {
	return NewSourceFromKindID(kind, "")
}

func NewSourceFromKindID(kind Kind, id string) (Source, error) {
	switch kind {
	case AtlassianCloudAdminKind:
		return &AtlassianCloudAdmin{id: id, RateLimit: httpclient.NewRateLimit()}, nil
	case AtlassianCloudJiraKind:
		return &AtlassianCloudJira{id: id, RateLimit: httpclient.NewRateLimit()}, nil
	case LeakTKServerKind:
		return &LeakTKServer{id: id, RateLimit: httpclient.NewRateLimit()}, nil
	default:
		return nil, fmt.Errorf("unsupported kind: kind=%q", kind.String())
	}
}
