package cmd

import (
	"fmt"
	"os"
	"strings"

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
	if _, err := fmt.Fprintln(os.Stdout, strings.Join(collector.FactCSVHeader, ",")); err != nil {
		logger.Fatal("could not write CSV header: %v", err)
	}
	ctx := cmd.Context()
	err := leaktCollector.Facts(ctx, srcs, func(fact collector.Fact) error {
		row, err := fact.MarshalCSV()
		if err != nil {
			return fmt.Errorf("could not marshal fact: %w eid=%d kind=%q", err, fact.EntityID, fact.Kind)
		}
		if _, err := os.Stdout.Write(row); err != nil {
			return fmt.Errorf("could not write fact: %w eid=%d kind=%q", err, fact.EntityID, fact.Kind)
		}
		return nil
	})

	if err != nil {
		logger.Fatal("collect failed: %v", err)
	}
}

func collectCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "collect <source-id>...",
		Short: "Collect facts about configured sources",
		Long: `Collect facts about given source ids in the config and stream them to stdout.

Facts are structured as a CSV with a header row and one fact per line. Facts
are similar to RDF triples in that they have a subject (the eid), predicate
(kind), and object (value). They also have a timestamp (ts) for when the fact
was collected.

An entity ID should be treated as ephemeral between runs and is solely for
grouping facts. Entity ID 0 is a special ID used for mapping enum IDs to
values.`,
		Args: cobra.MinimumNArgs(1),
		Run:  runCollect,
	}
}
