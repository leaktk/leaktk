package monitor

import (
	"github.com/leaktk/leaktk/pkg/proto"
	"github.com/leaktk/leaktk/pkg/queue"
	"github.com/leaktk/leaktk/pkg/sources"
)

type Monitor struct {
	scanRequestQueue *queue.PriorityQueue[*proto.Request]
	sources          []sources.Source
}

func NewMonitor(sources []sources.Source) *Monitor {
	// It's expected the queue should flush pretty quick but this gives
	// some extra room in case it hasn't flused by the time the monitor has a
	// new scan request ready
	maxQueueSize := len(sources) * 2
	// Go ahead and initailzie all the memory we're going to use
	initQueueSize := maxQueueSize

	return &Monitor{
		scanRequestQueue: queue.NewPriorityQueue[*proto.Request](initQueueSize, maxQueueSize),
		sources:          sources,
	}
}

// ScanRequests starts the monitor and yields scan requests
func (m *Monitor) ScanRequests(yield func(*proto.Request)) {

}
