package betterleaks

import (
	"errors"
	"fmt"

	blconfig "github.com/betterleaks/betterleaks/config"
)

type Config blconfig.Config

func ParseConfig(rawConfig []byte) (*Config, error) {
	var err error
	var cfg *blconfig.Config

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("config is invalid: %v", r)
		}
	}()

	cfg, err = blconfig.ParseTOML(rawConfig, "")
	if err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if err = validate(cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return (*Config)(cfg), nil
}

func validate(cfg *blconfig.Config) error {
	if len(cfg.Rules) == 0 && cfg.Prefilter == "" && cfg.Filter == "" {
		return errors.New("no rules or filters")
	}

	return nil
}
