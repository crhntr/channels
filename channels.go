// Package channels provides generic utilities for Go channels
// and conversions between channels and iterators.
package channels

import (
	"iter"
	"reflect"
	"runtime"
	"slices"
)

// Drain receives and discards all values. Blocks until c closes.
func Drain[T any](c <-chan T) {
	for range c {
	}
}

// TapIter wraps a channel and cancel func as an iterator.
// The cancel func is called when iteration ends, whether
// by break or the channel closing.
func TapIter[T any](c <-chan T, cancel func()) iter.Seq[T] {
	return func(yield func(T) bool) {
		defer cancel()
		for v := range c {
			if !yield(v) {
				break
			}
		}
	}
}

// Tap converts an iterator into an unbuffered channel and a stop func.
// Call stop to cancel iteration early and close the channel.
// Reading from the returned channel applies backpressure to the iterator.
//
// See Pipe for a non-cancellable variant.
func Tap[T any](in iter.Seq[T]) (<-chan T, func()) {
	var (
		c    = make(chan T)
		stop = make(chan struct{})
	)
	go func() {
		defer close(c)
		for v := range in {
			select {
			case <-stop:
				return
			case c <- v:
			}
		}
	}()
	return c, func() {
		close(stop)
	}
}

// Pipe delivers iterator values to a new unbuffered channel.
// Reading from the returned channel controls iteration speed.
// The channel closes when the iterator is exhausted.
//
// See Tap for a cancellable variant.
func Pipe[T any](seq iter.Seq[T]) <-chan T {
	c := make(chan T)
	go func() {
		defer close(c)
		seq(func(val T) bool {
			c <- val
			return true
		})
	}()
	return c
}

// PipeIter returns an iterator that yields values from c until it closes.
func PipeIter[T any](c <-chan T) iter.Seq[T] {
	return func(yield func(T) bool) {
		for v := range c {
			if !yield(v) {
				break
			}
		}
	}
}

// Count returns the number of values received before the channel closes.
func Count[T any](in <-chan T) int {
	count := 0
	for range in {
		count++
	}
	return count
}

// FanIn merges multiple input channels into a single output channel.
// The output closes when every input has closed.
func FanIn[T any](channels ...<-chan T) <-chan T {
	c := make(chan T)
	go func() {
		defer close(c)
		cases := make([]reflect.SelectCase, len(channels))
		for i := range channels {
			cases[i] = reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(channels[i]),
			}
		}
		for len(cases) > 0 {
			chosen, value, ok := reflect.Select(cases)
			if !ok {
				cases = slices.Delete(cases, chosen, chosen+1)
				continue
			}
			c <- value.Interface().(T)
		}
	}()
	return c
}

// FanOut delivers each input value to every output channel.
// All n outputs must accept a value before the next is read,
// so throughput is limited by the slowest consumer.
func FanOut[T any](n uint16, in <-chan T) []<-chan T {
	channels := make([]chan T, n)
	for i := range channels {
		channels[i] = make(chan T)
	}
	go fanOut(in, sendOnly(channels))
	return receiveOnly(channels)
}

// Filter forwards values from in that satisfy keep.
// The output closes when in closes.
func Filter[T any](in <-chan T, keep func(T) bool) <-chan T {
	c := make(chan T)
	go func() {
		defer close(c)
		for v := range in {
			if keep(v) {
				c <- v
			}
		}
	}()
	return c
}

// Map returns a channel carrying f(v) for each value v from in.
// Backpressure from reading the output propagates to in.
func Map[T1, T2 any](in <-chan T1, f func(T1) T2) <-chan T2 {
	c := make(chan T2)
	go func() {
		defer close(c)
		for v := range in {
			c <- f(v)
		}
	}()
	return c
}

// Take forwards the first n values from in, then closes the output.
// It does not drain remaining values from in.
func Take[T any](in <-chan T, n int) <-chan T {
	c := make(chan T)
	go func() {
		defer close(c)
		for range n {
			v, ok := <-in
			if !ok {
				return
			}
			c <- v
		}
	}()
	return c
}

// Batch collects values from in into slices of the given size.
// A final partial batch is sent when in closes.
func Batch[T any](in <-chan T, size int) <-chan []T {
	c := make(chan []T)
	go func() {
		defer close(c)
		batch := make([]T, 0, size)
		for v := range in {
			batch = append(batch, v)
			if len(batch) == size {
				c <- batch
				batch = make([]T, 0, size)
			}
		}
		if len(batch) > 0 {
			c <- batch
		}
	}()
	return c
}

// Recent buffers up to n values between a fast writer and a slow reader,
// dropping the oldest when full. Remaining buffered values are
// delivered after in closes.
func Recent[T any](n int, in <-chan T) <-chan T {
	out := make(chan T)
	go func() {
		defer close(out)
		buf := make([]T, 0, n)
		for in != nil || len(buf) > 0 {
			var sendVal T
			if len(buf) > 0 {
				sendVal = buf[0]
			}
			select {
			case v, ok := <-in:
				if !ok {
					in = nil
					continue
				}
				if len(buf) == n {
					buf = slices.Delete(buf, 0, 1)
				}
				buf = append(buf, v)
			case out <- sendVal:
				buf = slices.Delete(buf, 0, 1)
			}
		}
	}()
	return out
}

func defaultNumberOfWorkers(n uint16) uint16 {
	if n == 0 {
		return uint16(runtime.NumCPU())
	}
	return n
}

// Workers starts n goroutines that concurrently apply f to values from in.
// When f returns false, the result is not forwarded.
// Pass 0 to use runtime.NumCPU workers.
// Output order is not preserved.
func Workers[T1, T2 any](n uint16, in <-chan T1, f func(T1) (T2, bool)) <-chan T2 {
	n = defaultNumberOfWorkers(n)
	workerChannels := make([]<-chan T2, 0, n)
	for workerIndex := uint16(0); workerIndex < n; workerIndex++ {
		workerChannels = append(workerChannels, worker(in, f))
	}
	return FanIn(workerChannels...)
}

func worker[T1, T2 any](in <-chan T1, f func(T1) (T2, bool)) <-chan T2 {
	c := make(chan T2)
	go func() {
		defer close(c)
		for v := range in {
			r, ok := f(v)
			if !ok {
				continue
			}
			c <- r
		}
	}()
	return c
}

type valueIndex[T any] struct {
	index int
	value T
}

// Apply transforms each element of in using n concurrent workers.
// Unlike Workers, results are returned in input order.
func Apply[T1, T2 any](n uint16, in []T1, f func(T1) T2) []T2 {
	if int(n) > len(in) {
		n = uint16(len(in))
	}
	result := make([]T2, len(in))
	workerMapWithIndexes(n, in, f, func(index int, _ int, _ T1, output T2) {
		result[index] = output
	})
	return result
}

func workerMapWithIndexes[T1, T2 any](n uint16, in []T1, f func(T1) T2, result func(int, int, T1, T2)) {
	inputs := sendSliceElementsWithIndex(in)
	outputs := Workers(n, inputs, func(i valueIndex[T1]) (valueIndex[T2], bool) {
		return valueIndex[T2]{value: f(i.value), index: i.index}, true
	})
	for o := range outputs {
		result(o.index, len(in), in[o.index], o.value)
	}
}

func fanOut[T any](in <-chan T, channels []chan<- T) {
	defer closeAll(channels)
	for v := range in {
		val := reflect.ValueOf(v)
		cases := make([]reflect.SelectCase, len(channels))
		for i := range channels {
			cases[i] = reflect.SelectCase{
				Dir:  reflect.SelectSend,
				Chan: reflect.ValueOf(channels[i]),
				Send: val,
			}
		}
		for len(cases) > 0 {
			chosen, _, ok := reflect.Select(cases)
			if !ok {
				cases = slices.Delete(cases, chosen, chosen+1)
			}
		}
	}
}

func closeAll[T any](channels []chan<- T) {
	for _, c := range channels {
		close(c)
	}
}

func receiveOnly[T any](in []chan T) []<-chan T {
	result := make([]<-chan T, len(in))
	for i := range in {
		result[i] = in[i]
	}
	return result
}

func sendOnly[T any](in []chan T) []chan<- T {
	result := make([]chan<- T, len(in))
	for i := range in {
		result[i] = in[i]
	}
	return result
}

func sendSliceElementsWithIndex[T any](in []T) <-chan valueIndex[T] {
	c := make(chan valueIndex[T])
	go func() {
		defer close(c)
		for i, v := range in {
			c <- valueIndex[T]{index: i, value: v}
		}
	}()
	return c
}

// Slide writes v to a channel with sliding-window semantics.
// On an unbuffered channel, it blocks until the value is read.
// On a buffered channel that is full, the oldest value is evicted.
// It returns the evicted value and true if eviction occurred.
func Slide[T any](ch chan T, v T) (T, bool) {
	if cap(ch) == 0 {
		ch <- v
		var zero T
		return zero, false
	}
	var (
		old     T
		evicted bool
	)
	select {
	case ch <- v:
	default:
		select {
		case old, evicted = <-ch:
		default:
		}
		select {
		case ch <- v:
		default:
		}
	}
	return old, evicted
}
