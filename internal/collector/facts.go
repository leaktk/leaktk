package collector

import (
	"bytes"
	"slices"
	"strconv"
	"strings"
)

type FactKind uint32

var FactKindNames = []string{
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

const (
	FactTrueValue  = "true"
	FactFalseValue = "false"
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
	KindFactKind
	NameFactKind
	SourceIDFactKind
	URLFactKind
	UsernameFactKind
)

type Fact struct {
	EntityID  uint32   `csv:"eid"`
	Kind      FactKind `csv:"kind"`
	Timestamp int64    `csv:"ts"`
	Value     string   `csv:"value"`
}

// FactCSVHeader returns the CSV header row field names.
var FactCSVHeader = []string{"eid", "kind", "ts", "value"}

// MarshalCSV returns the fact as a CSV-encoded byte slice.
func (f Fact) MarshalCSV() ([]byte, error) {
	escaped := strings.ReplaceAll(f.Value, `"`, `""`)

	// 10 = max uint32 length; 20 = max int64 length; 6 = comma + quote + newline count
	buf := bytes.Buffer{}
	buf.Grow(10*2 + 20 + 7 + len(escaped))
	buf.WriteString(strconv.FormatUint(uint64(f.EntityID), 10))
	buf.WriteByte(',')
	buf.WriteString(strconv.FormatUint(uint64(f.Kind), 10))
	buf.WriteByte(',')
	buf.WriteString(strconv.FormatInt(f.Timestamp, 10))
	buf.WriteString(`,"`)
	buf.WriteString(escaped)
	buf.WriteString("\"\n")

	return buf.Bytes(), nil
}

type FactYieldFunc func(fact Fact) error

func factKindByName(name string) (FactKind, bool) {
	i := slices.Index(FactKindNames, name)
	if i < 0 {
		return 0, false
	}
	return FactKind(i), true
}

// helper for yielding values
func yieldKV(f Fact, fk FactKind, v string, yield FactYieldFunc, err error) error {
	if err != nil {
		return err
	}

	f.Kind = fk
	f.Value = v
	return yield(f)
}
