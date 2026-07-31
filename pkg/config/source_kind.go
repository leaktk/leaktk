package config

import "fmt"

type SourceKind uint32

var SourceKindNames = []string{
	"AtlassianCloudAdmin",
	"AtlassianCloudJira",
	"LDAP",
}

func (sk SourceKind) String() string {
	return SourceKindNames[sk]
}

func (sk *SourceKind) UnmarshalText(text []byte) error {
	s := string(text)
	for i, name := range SourceKindNames {
		if name == s {
			*sk = SourceKind(i)
			return nil
		}
	}

	return fmt.Errorf("unknown source kind: %q", s)
}

const (
	AtlassianCloudAdminSourceKind SourceKind = iota
	AtlassianCloudJiraSourceKind
	LDAPSourceKind
)
