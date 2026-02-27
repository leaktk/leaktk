package gitleaks

import (
	"errors"
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/betterleaks/betterleaks/config"
)

func ParseConfig(rawConfig string) (cfg *config.Config, err error) {
	var vc config.ViperConfig

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("gitleaks config is invalid: %v", r)
		}
	}()

	_, err = toml.Decode(rawConfig, &vc)
	if err != nil {
		return
	}

	vcCfg, err := vc.Translate()
	if err != nil {
		err = fmt.Errorf("error loading config: %w", err)
		return
	}
	cfg = &vcCfg

	if err = validate(cfg); err != nil {
		err = fmt.Errorf("invalid config: %w", err)
		return
	}

	return
}

func validate(cfg *config.Config) error {
	if len(cfg.Rules) == 0 && len(cfg.Allowlists) == 0 {
		return errors.New("no rules or allowlists")
	}

	for _, a := range cfg.Allowlists {
		if len(a.Paths) == 0 && len(a.Regexes) == 0 && len(a.StopWords) == 0 && len(a.Commits) == 0 {
			return errors.New("an allowlist exists that doesn't allow anything")
		}
	}

	return nil
}
