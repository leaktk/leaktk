package facts

type Kind int

const (
	IDKind Kind = iota
	ActiveKind
	EmailAddressKind
	EmailAddressVerifiedKind
	EntityKindKind
	NameKind
	RelatedEntityIDKind
	SourceIDKind
	URLKind
	UsernameKind
)

var KindNames = []string{
	"ID",
	"Active",
	"EmailAddress",
	"EmailAddressVerified",
	"EntityKind",
	"Name",
	"RelatedEntityID",
	"SourceID",
	"URL",
	"Username",
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
