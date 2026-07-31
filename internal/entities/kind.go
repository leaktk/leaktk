package enties

type Kind int

const (
	AtlassianCloudUserKind Kind = iota
	LDAPRecordKind
)

var KindNames = []string{
	"AtlassianCloudUser",
	"LDAPRecord",
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
