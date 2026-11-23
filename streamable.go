package glowflow

import "sync"

type Streamable interface {
	GetStream() <-chan any
}

type StreamableWrapper[T any] struct {
	onceLock sync.Once
	dataChan chan *T
	stream   chan any
}

func NewStreamableWrapper[T any](dataChan chan *T) Streamable {
	return &StreamableWrapper[T]{
		dataChan: dataChan,
		onceLock: sync.Once{},
	}
}
func (x *StreamableWrapper[T]) GetStream() <-chan any {
	x.onceLock.Do(func() {
		x.stream = make(chan any)
		go func() {
			defer close(x.stream)
			for item := range x.dataChan {
				x.stream <- item
			}
		}()
	})
	return x.stream
}
