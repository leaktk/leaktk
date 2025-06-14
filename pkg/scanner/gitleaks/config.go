package gitleaks

import (
	"errors"
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/zricethezav/gitleaks/v8/config"
)

func ParseConfig(rawConfig string) (*config.Config, error) {
	var vc config.ViperConfig
	var cfg config.Config
	var err error

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("gitleaks config is invalid: %v", r)
		}
	}()

	_, err = toml.Decode(rawConfig, &vc)
	if err != nil {
		return nil, err
	}

	cfg, err = vc.Translate()
	if err != nil {
		return nil, fmt.Errorf("error loading config: %w", err)
	}

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, err
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
