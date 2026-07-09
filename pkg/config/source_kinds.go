package config

import (
	"fmt"
)

type SourceKind int

const (
	GitHubSourceKind SourceKind = iota
)

func (k *SourceKind) UnmarshalText(text []byte) error {
	rawKind := string(text)
	// TODO: turn this into a map
	// TODO: write unit tests
	switch rawKind {
	case "GitHub":
		*k = GitHubSourceKind
	default:
		return fmt.Errorf("invalid source kind: kind=%q", rawKind)
	}

	return nil
}
