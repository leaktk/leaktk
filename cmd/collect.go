package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/leaktk/leaktk/internal/collector"
	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/logger"
)

func runCollect(cmd *cobra.Command, args []string) {
	srcs := make(map[string]config.Source, len(cfg.Sources))

	for i, src := range cfg.Sources {
		id := src.ID()
		if _, exists := srcs[id]; exists {
			logger.Fatal("duplicate source ID detected in config id=%q index=%d", id, i)
		}
		srcs[id] = src
	}

	leaktCollector := collector.NewCollector()
	writer := bufio.NewWriter(os.Stdout)
	defer func() { _ = writer.Flush() }()
	encoder := json.NewEncoder(writer)
	ctx := cmd.Context()

	for _, id := range args {
		src, exists := srcs[id]
		if !exists {
			logger.Fatal("source ID does not exist id=%q", id)
		}

		err := leaktCollector.Facts(ctx, src, func(fact collector.Fact, err error) error {
			if err == nil {
				err = encoder.Encode(fact)
			}
			if err != nil {
				return fmt.Errorf("could not encode fact: %w fact_entity_id=%d fact_kind=%q", err, fact.EntityID, fact.Kind)
			}
			return nil
		})

		if err != nil {
			logger.Fatal("collect failed: %v src_id=%q src_kind=%q", err, src.ID(), src.Kind())
		}
	}
}

func collectCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "collect <source-id>",
		Short: "Collect facts about a source",
		Args:  cobra.MinimumNArgs(1),
		Run:   runCollect,
	}
}
