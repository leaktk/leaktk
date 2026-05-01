package cmd

import (
	"github.com/spf13/pflag"

	"github.com/leaktk/leaktk/pkg/logger"
)

func mustGetBool(flags *pflag.FlagSet, name string) bool {
	value, err := flags.GetBool(name)
	if err != nil {
		logger.Fatal("unable to get flag: name=%q", name)
	}
	return value
}

func mustGetString(flags *pflag.FlagSet, name string) string {
	value, err := flags.GetString(name)
	if err != nil {
		logger.Fatal("unable to get flag: name=%q", name)
	}
	return value
}
