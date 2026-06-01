package broadcast

import (
	"math/rand"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/crhntr/channels"
)

// Coordinator distributes values from one input channel to multiple subscribers.
type Coordinator[T any] struct {
	in        <-chan T
	sub       chan subRequest[T]
	unsub     chan *subscriber[T]
	closed    chan struct{}
	done      chan struct{} // closed when run exits
	drops     atomic.Int64
	latest    atomic.Value // stores latestValue[T]
	closeOnce sync.Once
}

type latestValue[T any] struct {
	v T
}

type subRequest[T any] struct {
	s    *subscriber[T]
	resp chan struct{}
}

type subscriber[T any] struct {
	ch       chan T
	all      bool // true = SubscribeAll (blocking), false = SubscribeLatest (sliding)
	filter   func(T) bool
	canceled chan struct{} // closed by cancel func to interrupt blocking sends
}

// New starts a coordinator goroutine that reads from in and delivers
// values to all active subscribers. When in closes, all subscriber channels
// are closed.
func New[T any](in <-chan T) *Coordinator[T] {
	b := &Coordinator[T]{
		in:     in,
		sub:    make(chan subRequest[T]),
		unsub:  make(chan *subscriber[T]),
		closed: make(chan struct{}),
		done:   make(chan struct{}),
	}
	go b.run()
	return b
}

func (bc *Coordinator[T]) run() {
	var subs []*subscriber[T]

	defer close(bc.done)
	defer func() {
		for _, s := range subs {
			if s.ch == nil {
				continue
			}
			close(s.ch)
		}
	}()

	for {
		select {
		case v, ok := <-bc.in:
			if !ok {
				return
			}
			bc.latest.Store(latestValue[T]{v: v})
			for _, s := range subs {
				if s.filter != nil && !s.filter(v) {
					continue
				}
				if s.all {
					select {
					case s.ch <- v:
					case <-s.canceled:
					}
				} else {
					if _, evicted := channels.Slide(s.ch, v); evicted {
						bc.drops.Add(1)
					}
				}
			}
		case req := <-bc.sub:
			subs = append(subs, req.s)
			close(req.resp)
		case s := <-bc.unsub:
			for i, ss := range subs {
				if ss == s {
					subs = slices.Delete(subs, i, i+1)
					close(s.ch)
					break
				}
			}
		case <-bc.closed:
			return
		}
	}
}

// SubscribeAll creates a subscription that delivers every value.
// When the buffer is full, the send blocks until the subscriber reads.
// An optional filter limits delivery to values where filter returns true.
// If any filter returns true, the value swill be sent.
// If more than one filter is provided, their order is shuffled.
func (bc *Coordinator[T]) SubscribeAll(n int, filter ...func(T) bool) (<-chan T, func()) {
	return bc.subscribe(&subscriber[T]{ch: make(chan T, n), all: true, canceled: make(chan struct{})}, filter)
}

// SubscribeLatest creates a subscription that delivers the latest values.
// When the buffer is full, the oldest value is evicted. Each eviction increments
// the drop counter. If n < 1, a buffer of 1 is used.
// An optional filter limits delivery to values where filter returns true.
// If any filter returns true, the value swill be sent.
// If more than one filter is provided, their order is shuffled.
func (bc *Coordinator[T]) SubscribeLatest(n int, filter ...func(T) bool) (<-chan T, func()) {
	if n < 1 {
		n = 1
	}
	return bc.subscribe(&subscriber[T]{ch: make(chan T, n)}, filter)
}

func swapFunc[T any, S ~[]T](in S) func(i, j int) {
	return func(i, j int) { in[i], in[j] = in[j], in[i] }
}

func (bc *Coordinator[T]) subscribe(s *subscriber[T], filter []func(T) bool) (<-chan T, func()) {
	if len(filter) == 1 {
		s.filter = filter[0]
	} else if len(filter) > 1 {
		shuffled := slices.Clone(filter)
		rand.Shuffle(len(shuffled), swapFunc(shuffled))
		s.filter = func(t T) bool {
			for _, f := range shuffled {
				if f(t) {
					return true
				}
			}
			return false
		}
	}
	req := subRequest[T]{s: s, resp: make(chan struct{})}
	select {
	case bc.sub <- req:
		<-req.resp
	case <-bc.done:
		close(s.ch)
		return s.ch, func() {}
	case <-bc.closed:
		close(s.ch)
		return s.ch, func() {}
	}
	return s.ch, bc.cancelFunc(s)
}

// Latest returns the most recent value delivered by the Coordinator.
// The bool is false if no value has been broadcast yet.
func (bc *Coordinator[T]) Latest() (T, bool) {
	v := bc.latest.Load()
	if v == nil {
		var zero T
		return zero, false
	}
	return v.(latestValue[T]).v, true
}

// DropCount returns the total number of values evicted from SubscribeLatest buffers.
func (bc *Coordinator[T]) DropCount() int64 {
	return bc.drops.Load()
}

// ResetDropCount returns the drop count and resets it to 0.
func (bc *Coordinator[T]) ResetDropCount() int64 {
	return bc.drops.Swap(0)
}

func (bc *Coordinator[T]) cancelFunc(s *subscriber[T]) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			if s.canceled != nil {
				close(s.canceled)
			}
			select {
			case bc.unsub <- s:
			case <-bc.done:
			case <-bc.closed:
			}
		})
	}
}

// Close stops the Coordinator and closes all subscriber channels.
// It is safe to call multiple times and concurrently.
func (bc *Coordinator[T]) Close() {
	bc.closeOnce.Do(func() {
		close(bc.closed)
	})
}
