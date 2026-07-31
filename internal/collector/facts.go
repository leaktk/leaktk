package collector

type FactKind uint32

var FactKindNames = []string{
	"ID",
	"Active",
	"EmailAddress",
	"EmailAddressVerified",
	"Name",
	"SourceID",
	"URL",
	"Username",
}

const (
	FactTrueValue  = "t"
	FactFalseValue = "f"
)

func (fk FactKind) ID() uint32 {
	return uint32(fk)
}

func (fk FactKind) String() string {
	return FactKindNames[fk]
}

const (
	IDFactKind FactKind = iota
	ActiveFactKind
	EmailAddressFactKind
	EmailAddressVerifiedFactKind
	NameFactKind
	SourceIDFactKind
	URLFactKind
	UsernameFactKind
)

type Fact struct {
	EntityID  uint32   `json:"eid"   csv:"eid"`
	Kind      FactKind `json:"kind"  csv:"kind"`
	Timestamp int64    `json:"ts"    csv:"ts"`
	Value     string   `json:"value" csv:"value"`
}

type FactYieldFunc func(fact Fact) error

// helper for yielding values
func yieldKV(f Fact, fk FactKind, v string, yield FactYieldFunc, err error) error {
	if err != nil {
		return err
	}

	f.Kind = fk
	f.Value = v
	return yield(f)
}
