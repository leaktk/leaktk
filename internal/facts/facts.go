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

var bufPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

type Fact struct {
	EntityID enties.ID `json:"eid"   csv:"eid"`
	Kind     Kind      `json:"kind"  csv:"kind"`
	Value    string    `json:"value" csv:"value"`
}

// FactCSVHeader returns the CSV header row field names.
var FactCSVHeader = []string{"eid", "kind", "value"}

// MarshalCSV returns the fact as a CSV-encoded byte slice.
func (f Fact) MarshalCSV() ([]byte, error) {
	escaped := strings.ReplaceAll(f.Value, `"`, `""`)
	buf := bufPool.Get().(*bytes.Buffer)

	buf.Reset()
	buf.WriteString(strconv.Itoa(f.Entity.ID()))
	buf.WriteByte(',')
	buf.WriteString(strconv.Itoa(f.Kind.ID()))
	buf.WriteString(`,"`)
	buf.WriteString(escaped)
	buf.WriteString("\"\n")

	result := buf.Bytes()
	bufPool.Put(b)

	return result, nil
}

func YieldWithKV(f Fact, k FactKind, v string, err error, yield FactYieldFunc) error {
	if err != nil {
		return err
	}
	f.Kind = k
	f.Value = v
	return yield(f)
}
