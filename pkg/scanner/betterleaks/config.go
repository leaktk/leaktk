package betterleaks

import (
	"errors"
	"fmt"

	"github.com/betterleaks/betterleaks/config"
)

func ParseConfig(rawConfig []byte) (cfg *config.Config, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("config is invalid: %v", r)
		}
	}()

	if cfg, err = config.ParseTOML(rawConfig, ""); err != nil {
		err = fmt.Errorf("invalid config: %w", err)
		return
	}

	if err = validate(cfg); err != nil {
		err = fmt.Errorf("invalid config: %w", err)
		return
	}

	return
}

func validate(cfg *config.Config) error {
	if len(cfg.Rules) == 0 && cfg.Prefilter == "" && cfg.Filter == "" {
		return errors.New("no rules or filters")
	}

	return nil
}
