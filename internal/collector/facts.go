package collector

import (
	"encoding/json"
	"fmt"
)

type FactKind uint32

var FactKindNames = []string{
	"URL",
	"EmailAddr",
	"ID",
	"Name",
	"SourceID",
	"Username",
}

func (fk FactKind) String() string {
	return FactKindNames[fk]
}

func (fk FactKind) MarshalJSON() ([]byte, error) {
	return json.Marshal(fk.String())
}

func (fk *FactKind) UnmarshalText(text []byte) error {
	s := string(text)
	for i, name := range FactKindNames {
		if name == s {
			*fk = FactKind(i)
			return nil
		}
	}

	return fmt.Errorf("unknown fact kind: %q", s)
}

const (
	URLFactKind FactKind = iota
	EmailAddrFactKind
	IDFactKind
	NameFactKind
	SourceID
	UsernameFactKind
)

type Fact struct {
	EntityID  uint32   `json:"eid"`
	Kind      FactKind `json:"knd"`
	Timestamp int64    `json:"ts"`
	Value     string   `json:"val"`
}
