package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/leaktk/leaktk/internal/fs"
	"github.com/leaktk/leaktk/pkg/logger"
)

type valuePath string
type valueEnvVar string
type valueLiteral string

func loadValueFile(dst *string, path string) error {
	path = filepath.Clean(path)
	if !fs.FileExists(path) {
		return nil
	}

	logger.Debug("loading value: path=%q", path)
	value, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w path=%q", err, path)
	}
	*dst = strings.TrimSpace(string(value))
	return nil
}

func loadValueEnvVar(dst *string, varname string) {
	value, ok := os.LookupEnv(varname)
	if !ok {
		return
	}
	*dst = strings.TrimSpace(value)
}

func loadValueSearch(dst *string, valueSrcs ...any) error {
	for _, vs := range valueSrcs {
		switch vs := vs.(type) {
		case valuePath:
			err := loadValueFile(dst, string(vs))
			if err != nil {
				return err
			}
		case valueEnvVar:
			loadValueEnvVar(dst, string(vs))
		case valueLiteral:
			*dst = string(vs)
		default:
			return fmt.Errorf("unexpected value source: type=%T value=%q", vs, vs)
		}

		if len(*dst) > 0 {
			return nil
		}
	}

	return nil
}
