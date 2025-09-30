package queue

import (
	"container/heap"
	"sync"
)

// PriorityQueue is like a channel but with dynamic buffering and returns items
// with the highest priority first
type PriorityQueue[T any] struct {
	heap        *MessageHeap[T]
	heapMutex   sync.Mutex
	out         chan *Message[T]
	msgCond     *sync.Cond
	maxSizeCond *sync.Cond
	maxSize     int
}

// NewPriorityQueue returns a PriorityQueue instance that is ready to send to
func NewPriorityQueue[T any](queueCapacity, maxSize int) *PriorityQueue[T] {
	pq := &PriorityQueue[T]{
		heap:        NewMessageHeap[T](queueCapacity),
		out:         make(chan *Message[T]),
		msgCond:     sync.NewCond(&sync.Mutex{}),
		maxSizeCond: sync.NewCond(&sync.Mutex{}),
		maxSize:     maxSize,
	}

	// Init the heap
	heap.Init(pq.heap)

	// Set up message forwarding
	go func() {
		for {
			if pq.Size() == 0 {
				pq.waitForMessage()
			}

			// Get the message but don't send it yet because sending can wait for
			// the receiver and we don't want to hold the lock for that long
			pq.heapMutex.Lock()
			// Sometimes with a lot of workers and very rapid bulk scanning, another
			// worker may snag the last item between the wait and this lock. So we
			// need to check the length again just to be sure to avoid any panics.
			if pq.heap.Len() == 0 {
				pq.heapMutex.Unlock()
				continue
			}

			msg := heap.Pop(pq.heap).(*Message[T])
			pq.heapMutex.Unlock()

			// Send the message to the out channel
			pq.out <- msg

			// Notify pq.Send that it can accept new messages when the queue has a
			// mazSize
			if pq.maxSize > 0 && pq.Size() < pq.maxSize {
				pq.signalQueueSpaceAvailable()
			}
		}
	}()

	return pq
}

// Send puts items on the queue
func (pq *PriorityQueue[T]) Send(msg *Message[T]) {
	// Wait for space if maxSize is set and the queue is full
	for pq.maxSize > 0 && pq.Size() >= pq.maxSize {
		pq.waitForSpaceOnQueue()
	}

	pq.heapMutex.Lock()
	heap.Push(pq.heap, msg)
	pq.heapMutex.Unlock()
	pq.signalMessageRecieved()
}

// Recv takes a function that can receive messages sent to the queue
func (pq *PriorityQueue[T]) Recv(fn func(*Message[T])) {
	for msg := range pq.out {
		fn(msg)
	}
}

func (pq *PriorityQueue[T]) waitForMessage() {
	pq.msgCond.L.Lock()
	pq.msgCond.Wait()
	pq.msgCond.L.Unlock()
}

func (pq *PriorityQueue[T]) signalMessageRecieved() {
	pq.msgCond.L.Lock()
	pq.msgCond.Signal()
	pq.msgCond.L.Unlock()
}

func (pq *PriorityQueue[T]) waitForSpaceOnQueue() {
	pq.maxSizeCond.L.Lock()
	pq.maxSizeCond.Wait()
	pq.maxSizeCond.L.Unlock()
}

func (pq *PriorityQueue[T]) signalQueueSpaceAvailable() {
	pq.maxSizeCond.L.Lock()
	pq.maxSizeCond.Signal()
	pq.maxSizeCond.L.Unlock()
}

// Size returns the current number of items in the queue
func (pq *PriorityQueue[T]) Size() int {
	pq.heapMutex.Lock()
	size := pq.heap.Len()
	pq.heapMutex.Unlock()
	return size
}
