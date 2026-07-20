package internal

import (
	"context"
	"sync/atomic"

	"golang.org/x/sys/cpu"
)

// RingBuffer implements a high-performance SPSC ring buffer
type RingBuffer[T any] struct {
	wi   uint32
	_    cpu.CacheLinePad
	ri   uint32
	_    cpu.CacheLinePad
	mask uint32
	buf  []T

	// Channels used purely for non-blocking notifications
	rNotify chan struct{}
	wNotify chan struct{}
}

func NewRingBuffer[T any](size uint32) *RingBuffer[T] {
	if (size & (size - 1)) != 0 {
		panic("buffer size must be a power of 2")
	}

	return &RingBuffer[T]{
		buf:     make([]T, size),
		mask:    size - 1,
		rNotify: make(chan struct{}, 1),
		wNotify: make(chan struct{}, 1),
	}
}

// ReadPtrIndex returns a pointer to the element in the ring buffer and its index
func (b *RingBuffer[T]) ReadPtrIndex(ctx context.Context) (*T, uint32, error) {
	for {
		ri := atomic.LoadUint32(&b.ri)
		wi := atomic.LoadUint32(&b.wi)

		if ri != wi {
			i := ri & b.mask
			return &b.buf[i], i, nil
		}

		// RingBuffer is empty; wait for a producer signal or context cancel
		select {
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		case <-b.rNotify:
			// Signaled that data is likely available, loop to re-evaluate
		}
	}
}

// ReadPtrIndex returns a pointer to the element in the ring buffer
func (b *RingBuffer[T]) ReadPtr(ctx context.Context) (*T, error) {
	ptr, _, err := b.ReadPtrIndex(ctx)
	return ptr, err
}

// ReadDone notifies the buffer that the element is free to use for a future write
func (b *RingBuffer[_]) ReadDone() {
	atomic.AddUint32(&b.ri, 1)

	// Non-blocking notify to the writer that slot space opened up
	select {
	case b.wNotify <- struct{}{}:
	default:
	}
}

// WritePtrIndex returns a pointer to the element in the ring buffer and its index
func (b *RingBuffer[T]) WritePtrIndex(ctx context.Context) (*T, uint32, error) {
	for {
		ri := atomic.LoadUint32(&b.ri)
		wi := atomic.LoadUint32(&b.wi)

		// Check if buffer is full
		if wi-ri != uint32(len(b.buf)) { // #nosec:G115
			i := wi & b.mask
			return &b.buf[i], i, nil
		}

		// RingBuffer is full; wait for a consumer signal or context cancel
		select {
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		case <-b.wNotify:
			// Signaled that space is likely available, loop to re-evaluate
		}
	}
}

// WritePtrIndex returns a pointer to the element in the ring buffer
func (b *RingBuffer[T]) WritePtr(ctx context.Context) (*T, error) {
	ptr, _, err := b.WritePtrIndex(ctx)
	return ptr, err
}

// WriteDone notifies the buffer that the element is ready to be read
func (b *RingBuffer[_]) WriteDone() {
	atomic.AddUint32(&b.wi, 1)

	// Non-blocking notify to the reader that new data arrived
	select {
	case b.rNotify <- struct{}{}:
	default:
	}
}

// Len returns the size of the buffer
func (b *RingBuffer[_]) Len() int {
	return len(b.buf)
}
