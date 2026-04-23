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

	wg.Add(1)
	leaktkScanner.Send(&proto.Request{
		ID:       fmt.Sprintf("%s.%s", hookname, id.ID()),
		Kind:     proto.GitRepoRequestKind,
		Resource: ".",
		Opts: proto.Opts{
			Local:  true,
			Staged: true,
		},
	})

	wg.Wait()
	if len(results) > 0 {
		gitHookDisplayResults(results)
		return 1, nil
	}

	return 0, nil
}
