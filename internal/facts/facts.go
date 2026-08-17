package facts

import (
	"bytes"
	"strconv"
	"strings"
	"sync"
)

const (
	FactValueTrue  = "true"
	FactValueFalse = "false"
)

var (
	bufPool = sync.Pool{
		New: func() any {
			return new(bytes.Buffer)
		},
	}
	FactCSVHeader = []string{"eid", "kind", "value"}
)

type (
	Fact struct {
		EntityID int    `json:"eid"   csv:"eid"`
		Kind     Kind   `json:"kind"  csv:"kind"`
		Value    string `json:"value" csv:"value"`
	}

	FactYieldFunc func(f Fact) error

	FactBool bool
)

func (b FactBool) Value() bool {
	return bool(b)
}

func (b FactBool) String() string {
	if b.Value() {
		return FactValueTrue
	}
	return FactValueFalse
}

func (f Fact) MarshalCSV() ([]byte, error) {
	escaped := strings.ReplaceAll(f.Value, `"`, `""`)
	buf := bufPool.Get().(*bytes.Buffer)

	buf.Reset()
	buf.WriteString(strconv.Itoa(f.EntityID))
	buf.WriteByte(',')
	buf.WriteString(strconv.Itoa(f.Kind.ID()))
	buf.WriteString(`,"`)
	buf.WriteString(escaped)
	buf.WriteString("\"\n")

	result := buf.Bytes()
	bufPool.Put(buf)

	return result, nil
}

func YieldWithKV(f Fact, k Kind, v string, err error, yield FactYieldFunc) error {
	if err != nil {
		return err
	}
	f.Kind = k
	f.Value = v
	return yield(f)
}
