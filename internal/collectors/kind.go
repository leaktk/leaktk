package collectors

type Kind int

const (
	AtlassianCloudUserKind Kind = iota
)

var KindNames = []string{
	"AtlassianCloudUser",
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
	if int(k) < 0 || int(k) > len(KindNames) {
		return "<invalid>"
	}
	return KindNames[k]
}

func KindFromName(name string) (k Kind, ok bool) {
	k, ok = kindNameMap[name]
	return
}
