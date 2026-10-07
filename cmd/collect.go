package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/leaktk/leaktk/internal/collectors"
	"github.com/leaktk/leaktk/internal/facts"
	"github.com/leaktk/leaktk/internal/logger"
	"github.com/leaktk/leaktk/internal/sources"
)

func runCollect(cmd *cobra.Command, srcIDs []string) {
	leaktCollector := collectors.NewCollector()
	ctx := cmd.Context()

	idMap := make(map[string]bool, len(srcIDs))
	for _, srcID := range srcIDs {
		idMap[srcID] = true
	}

	srcs := make(sources.Sources, 0, len(cfg.Sources))
	for _, src := range cfg.Sources {
		if idMap[src.ID()] {
			srcs = append(srcs, src)
		}
	}

	fmt.Println(strings.Join(facts.FactCSVHeader, ","))
	err := leaktCollector.Facts(ctx, srcs, func(f facts.Fact) error {
		row, err := f.MarshalCSV()
		if err != nil {
			return fmt.Errorf("could not marshal fact: %w eid=%d key=%q", err, f.EntityID, f.Key)
		}
		fmt.Print(string(row))
		return nil
	})

	if err != nil {
		logger.Fatal("collect failed: %v", err)
	}
}

func collectCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "collect [flags] <source-id>...",
		Short: "Collect facts about configured sources",
		Long: `Collect facts about given source ids in the config and stream them to stdout.

Facts are structured as a CSV with a header row and one fact per line. Facts
are similar to RDF triples in that they have a subject (the eid), predicate
(key), and object (value).

An entity ID should be treated as ephemeral between runs and is solely for
grouping facts. Entity ID 0 is a special ID used for mapping enum IDs to
values.`,
		Args: cobra.MinimumNArgs(1),
		Run:  runCollect,
	}
}
