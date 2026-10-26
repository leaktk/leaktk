package tomlutil

import (
	"io"

	"github.com/BurntSushi/toml"
)

func Unmarshal(d []byte, v any) error {
	return toml.Unmarshal(d, v)
}

func UnmarshalReader(r io.Reader, v any) error {
	_, err := toml.NewDecoder(r).Decode(v)
	return err
}

func Marshal(v any) ([]byte, error) {
	return toml.Marshal(v)
}
