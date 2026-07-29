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
	srcsByID := make(map[string]config.Source, len(cfg.Sources))
	for i, src := range cfg.Sources {
		id := src.ID()
		if _, exists := srcsByID[id]; exists {
			logger.Fatal("duplicate source ID detected in config id=%q index=%d", id, i)
		}
		srcsByID[id] = src
	}

	srcs := make(config.Sources, 0, len(args))
	for _, id := range args {
		src, exists := srcsByID[id]
		if !exists {
			logger.Fatal("source ID does not exist id=%q", id)
		}
		srcs = append(srcs, src)
	}

	leaktCollector := collector.NewCollector()
	writer := bufio.NewWriter(os.Stdout)
	defer func() { _ = writer.Flush() }()
	encoder := json.NewEncoder(writer)
	ctx := cmd.Context()
	err := leaktCollector.Facts(ctx, srcs, func(fact collector.Fact) error {
		if err := encoder.Encode(fact); err != nil {
			return fmt.Errorf("could not encode fact: %w fact_entity_id=%d fact_kind=%q", err, fact.EntityID, fact.Kind)
		}
		return nil
	})

	if err != nil {
		logger.Fatal("collect failed: %v", err)
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
