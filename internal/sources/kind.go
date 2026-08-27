package sources

type Kind int

const (
	AtlassianCloudAdminKind Kind = iota
	AtlassianCloudJiraKind
)

var KindNames = []string{
	"AtlassianCloudAdmin",
	"AtlassianCloudJira",
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

func KindFromName(name string) (k Kind, ok bool) {
	k, ok = kindNameMap[name]
	return
}
