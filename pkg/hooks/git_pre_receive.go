package hooks

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/id"
	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/leaktk/leaktk/pkg/proto"
	"github.com/leaktk/leaktk/pkg/scanner"
)

// pre-receive line format:
// <old-oid> SP <new-oid> SP <ref-name> LF
// OIDs are 40 characters and 255 seems to be a limit for ref names
// So 4096 should be plenty and align it with a page of memory
const gitPreReceiveMaxLineLimit = 4096

var emptyOID = []byte("0000000000000000000000000000000000000000")

func gitPreReceiveRun(cfg *config.Config, hook Hook, _ []string) (int, error) {
	var resultsMutex sync.Mutex
	var results []*proto.Result
	var wg sync.WaitGroup

	leaktkScanner := scanner.NewScanner(cfg)

	// Prints the output of the scanner as they come
	go leaktkScanner.Recv(func(response *proto.Response) {
		if response.Error != nil {
			logger.Fatal("scan response contains error: %v", response.Error)
		}

		if len(response.Results) > 0 {
			resultsMutex.Lock()
			results = append(results, response.Results...)
			resultsMutex.Unlock()
		}
		wg.Done()
	})

	refsReader := bufio.NewReaderSize(os.Stdin, 4096)
	for {
		line, isPrefix, err := refsReader.ReadLine()
		if err != nil {
			if err == io.EOF {
				break
			}
			logger.Fatal("error reading from stdin: %v", err)
		}

		if isPrefix {
			logger.Fatal("line too large: len(<old-oid> SP <new-oid> SP <ref-name> LF) must be < %d", gitPreReceiveMaxLineLimit)
		}

		oldIDStart := 0
		oldIDEnd := bytes.IndexByte(line, ' ')
		if oldIDEnd != 40 {
			logger.Debug("unexpected oldIDEnd: expected=40 actual=%d", oldIDEnd)
			logger.Fatal("expected line to start with '[0-9a-z]{40} ': line=%q", line)
		}

		newIDStart := oldIDEnd + 1
		newIDEnd := newIDStart + bytes.IndexByte(line[newIDStart:], ' ')
		if newIDEnd != 81 {
			logger.Debug("unexpected newIDEnd: expected=81 actual=%d", newIDEnd)
			logger.Fatal("expected line to start with '[0-9a-z]{40} [0-9a-z]{40}': line=%q", line)
		}

		newID := line[newIDStart:newIDEnd]
		if bytes.Equal(newID, emptyOID) {
			logger.Debug("skipping delete-ref line: line=%q", line)
			continue
		}

		// Create exclusions list from the oldID if it points to a non-empty object ID
		var exclusions []string
		if oldID := line[oldIDStart:oldIDEnd]; !bytes.Equal(oldID, emptyOID) {
			exclusions = []string{string(oldID)}
		}

		wg.Add(1)
		leaktkScanner.Send(&proto.Request{
			ID:       fmt.Sprintf("leaktk.%s.%s", hook.Name(), id.ID()),
			Kind:     proto.GitRepoRequestKind,
			Resource: ".",
			Opts: proto.Opts{
				Local:      true,
				Branch:     string(newID),
				Exclusions: exclusions,
			},
		})
	}

	wg.Wait()
	if len(results) > 0 {
		gitHookDisplayResults(results)
		return 1, nil
	} else {
		logger.Info("no secrets detected")
	}

	return 0, nil
}
