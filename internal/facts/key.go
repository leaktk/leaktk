package facts

type Key int

const (
	IDKey Key = iota
	ActiveKey
	EmailAddressKey
	EmailAddressVerifiedKey
	KindKey
	NameKey
	SourceIDKey
	URLKey
	UsernameKey
)

var KeyNames = []string{
	"ID",
	"Active",
	"EmailAddress",
	"EmailAddressVerified",
	"Kind",
	"Name",
	"SourceID",
	"URL",
	"Username",
}

var keyNameMap = make(map[string]Key, len(KeyNames))

func init() {
	for i, name := range KeyNames {
		keyNameMap[name] = Key(i)
	}
}

func (k Key) ID() int {
	return int(k)
}

func (k Key) String() string {
	if int(k) < 0 || int(k) > len(KeyNames) {
		return "<invalid>"
	}
	return KeyNames[k]
}

func KeyFromName(name string) (k Key, ok bool) {
	k, ok = keyNameMap[name]
	return
}
