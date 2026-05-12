package betterleaks

import (
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/betterleaks/betterleaks/config"
)

func ParseConfig(rawConfig string) (cfg *config.Config, err error) {
	var vc config.ViperConfig

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("config is invalid: %v", r)
		}
	}()

	_, err = toml.Decode(rawConfig, &vc)
	if err != nil {
		return
	}

	cfg, err = vc.Translate()
	if err != nil {
		err = fmt.Errorf("error loading config: %w", err)
		return
	}
	if err = validate(cfg); err != nil {
		err = fmt.Errorf("invalid config: %w", err)
		return
	}
	return
}

func celJoinOr(exprs ...string) string {
	if len(exprs) == 1 {
		return exprs[0]
	}

	toJoin := make([]string, 0, len(exprs))
	for _, expr := range exprs {
		expr = strings.TrimSpace(expr)
		if len(expr) > 0 {
			toJoin = append(toJoin, "("+expr+")")
		}
	}

	return strings.Join(toJoin, "||")
}

// AppendGlobalConfigs together ignoring the rules and not mutating the config
func AppendGlobalConfig(cfg, cfgExtra *config.Config) *config.Config {
	cfg = CopyConfig(cfg)
	cfg.Prefilter = celJoinOr(cfg.Prefilter, cfgExtra.Prefilter)
	cfg.Filter = celJoinOr(cfg.Filter, cfgExtra.Filter)
	return cfg
}

// CopyConfig handles steps needed to make a copy of the config that should be safe to mutate
func CopyConfig(cfg *config.Config) *config.Config {
	rulesCopy := make(map[string]config.Rule)
	maps.Copy(rulesCopy, cfg.Rules)

	kwCopy := make(map[string]struct{})
	maps.Copy(kwCopy, cfg.Keywords)

	kw2rulesCopy := make(map[string][]string)
	for k, v := range cfg.KeywordToRules {
		copy(kw2rulesCopy[k], v)
	}

	cfgCopy := config.Config{
		Title:                 cfg.Title,
		Description:           cfg.Description,
		Rules:                 rulesCopy,
		Filter:                cfg.Filter,
		Prefilter:             cfg.Prefilter,
		KeywordToRules:        kw2rulesCopy,
		Keywords:              kwCopy,
		MinVersion:            cfg.MinVersion,
		BetterleaksMinVersion: cfg.BetterleaksMinVersion,
		Path:                  cfg.Path,
		Extend: config.Extend{
			Path:       cfg.Extend.Path,
			URL:        cfg.Extend.URL,
			UseDefault: cfg.Extend.UseDefault,
		},
	}

	copy(cfgCopy.NoKeywordRules, cfg.NoKeywordRules)
	copy(cfgCopy.OrderedRules, cfg.OrderedRules)
	copy(cfgCopy.Extend.DisabledRules, cfg.Extend.DisabledRules)

	return &cfgCopy
}

func validate(cfg *config.Config) error {
	if len(cfg.Rules) == 0 && len(strings.TrimSpace(cfg.Prefilter))+len(strings.TrimSpace(cfg.Filter)) == 0 {
		return errors.New("no rules or filters")
	}
	return nil
}
