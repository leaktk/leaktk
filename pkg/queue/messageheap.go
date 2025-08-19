package queue

// Message encapsulates a value with its priority
type Message[T any] struct {
	Priority int
	Value    T
}

// MessageHeap implements the container/heap interface to hold messages
type MessageHeap[T any] struct {
	data []*Message[T]
}

// NewMessageHeap returns an initialized MessageHeap of the specified capacity
func NewMessageHeap[T any](capacity int) *MessageHeap[T] {
	return &MessageHeap[T]{
		data: make([]*Message[T], 0, capacity),
	}
}

// Len returns how many items are on the heap
func (h *MessageHeap[T]) Len() int {
	return len(h.data)
}

// Less returns which item in the heap is smaller than the other
func (h *MessageHeap[T]) Less(i, j int) bool {
	return h.data[i].Priority > h.data[j].Priority
}

// Swap two items in the heap
func (h *MessageHeap[T]) Swap(i, j int) {
	h.data[i], h.data[j] = h.data[j], h.data[i]
}

// Push an item onto the heap
func (h *MessageHeap[T]) Push(msg any) {
	h.data = append(h.data, msg.(*Message[T]))
}

// Pop an item off the heap
func (h *MessageHeap[T]) Pop() any {
	n := len(h.data)
	msg := h.data[n-1]
	h.data[n-1] = nil // For GC purposes
	h.data = h.data[:n-1]

	return msg
}
