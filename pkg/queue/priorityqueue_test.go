package queue

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPriorityQueue(t *testing.T) {
	t.Run("Send/Recv", func(t *testing.T) {
		messages := []*Message[string]{
			{
				Priority: 0,
				Value:    "E",
			},
			{
				Priority: 5,
				Value:    "D",
			},
			{
				Priority: 7,
				Value:    "B",
			},
			{
				Priority: 5,
				Value:    "C",
			},
			{
				Priority: 9,
				Value:    "A",
			},
		}

		pq := NewPriorityQueue[string](len(messages), 0)

		var wg sync.WaitGroup
		var actual []string

		for _, msg := range messages {
			wg.Add(1)
			pq.Send(msg)
		}

		go pq.Recv(func(msg *Message[string]) {
			actual = append(actual, msg.Value)
			wg.Done()
		})

		wg.Wait()
		expected := []string{"A", "B", "C", "D", "E"}
		assert.Equal(t, expected, actual)
	})
}

func TestPriorityQueueMaxSize(t *testing.T) {
	t.Run("Send blocks when queue is full", func(t *testing.T) {
		maxSize := 2
		pq := NewPriorityQueue[string](maxSize, maxSize)

		var wg sync.WaitGroup
		var actual []string

		// Fill the queue
		wg.Add(1)
		pq.Send(&Message[string]{Value: "A", Priority: 1})
		wg.Add(1)
		pq.Send(&Message[string]{Value: "B", Priority: 1})

		// This should block
		wg.Add(1)
		go func() {
			pq.Send(&Message[string]{Value: "C", Priority: 1})
		}()

		// Give the goroutine time to block
		// Wait up to 2 seconds but stop as soon as it's there
		for i := 0; pq.Size() != maxSize && i < 20; i++ {
			time.Sleep(100 * time.Millisecond)
		}

		// Check that the queue is full but not overfilled
		assert.Equal(t, maxSize, pq.Size())

		go pq.Recv(func(msg *Message[string]) {
			actual = append(actual, msg.Value)
			wg.Done()
		})

		wg.Wait()
		expected := []string{"A", "B", "C"}
		assert.Equal(t, expected, actual)
	})
}
