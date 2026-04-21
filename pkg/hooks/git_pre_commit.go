package hooks

import (
	"fmt"
	"sync"

	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/id"
	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/leaktk/leaktk/pkg/proto"
	"github.com/leaktk/leaktk/pkg/scanner"
)

func gitPreCommitRun(cfg *config.Config, hookname string, _ []string) (int, error) {
	var wg sync.WaitGroup
	var response *proto.Response

	leaktkScanner := scanner.NewScanner(cfg)
	request := proto.Request{
		ID:       fmt.Sprintf("%s.%s", hookname, id.ID()),
		Kind:     proto.GitRepoRequestKind,
		Resource: ".",
		Opts: proto.Opts{
			Local:  true,
			Staged: true,
		},
	}

	// Prints the output of the scanner as they come
	go leaktkScanner.Recv(func(resp *proto.Response) {
		// Confirm that there is only one response to one request;
		// anything other than that would be a bug.
		if response != nil {
			logger.Fatal("unexpected additional response returned during scan: id=%q", resp.ID)
		}

		response = resp
		wg.Done()
	})

	wg.Add(1)
	leaktkScanner.Send(&request)
	wg.Wait()
	leaksFound := len(response.Results) > 0

	// Display any results if found before doing error handling to show
	// partial results if they exist
	if leaksFound {
		gitHookDisplayResults(response.Results)
	}

	// Return non-zero status code if the response had an error or if leaks were found
	if response.Error != nil {
		return 1, fmt.Errorf("response contains error: %w", response.Error)
	}
	if leaksFound {
		return 1, nil
	}

	return 0, nil
}
