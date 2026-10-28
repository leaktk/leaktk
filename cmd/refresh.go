package cmd

import (
	"github.com/spf13/cobra"

	"github.com/leaktk/leaktk/internal/httpclient"
	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/leaktk/leaktk/pkg/scanner"
)

func runRefresh(cmd *cobra.Command, args []string) {
	p := scanner.NewPatterns(cfg, httpclient.NewClient())
	if err := p.Refresh(cmd.Context()); err != nil {
		logger.Fatal("could not refresh patterns: %v", err)
	}
}

func refreshCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "refresh",
		Short: "Refresh patterns cache",
		Run:   runRefresh,
	}
}
