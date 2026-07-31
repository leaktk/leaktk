package sources

type Kind int

const (
	AtlassianCloudAdmingcKind Kind = iota
	AtlassianCloudJiragcKind
	LDAPKind
)

var KindNames = []string{
	"AtlassianCloudAdmin",
	"AtlassianCloudJira",
	"LDAP",
}

var kindNameMap map[string]Kind

func init() {
	for i, name := range KindNames {
		kindNameMap[name] = Kind(i)
	}
}

func (k Kind) ID() int {
	return int(k)
}

func (k Kind) String() string {
	if k < 0 || k > len(KindNames) {
		return "<invalid>"
	}
	return KindNames[k]
}

func KindFromName(name string) (Kind, bool) {
	return kindNameMap[name]
}
