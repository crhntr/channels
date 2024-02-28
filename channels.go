package channels

import (
	"reflect"
	"runtime"
	"slices"
	"sync"
)

func Drain[T any](c <-chan T) {
	for range c {
	}
}

func SendElements[T any](in []T) <-chan T {
	c := make(chan T)
	go func() {
		defer close(c)
		for _, v := range in {
			c <- v
		}
	}()
	return c
}

func ReceiveElements[T any](c <-chan T) []T {
	var list []T
	for v := range c {
		list = append(list, v)
	}
	return slices.Clip(list)
}

func CountReceived[T any](in <-chan T) int {
	count := 0
	for range in {
		count++
	}
	return count
}

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

func FanOut[T any](n uint16, in <-chan T) []<-chan T {
	channels := make([]chan T, n)
	for i := range channels {
		channels[i] = make(chan T)
	}
	go fanOut(in, sendOnly(channels))
	return receiveOnly(channels)
}

func defaultNumberOfWorkers(n uint16) uint16 {
	if n == 0 {
		return uint16(runtime.NumCPU())
	}
	return n
}

// Workers creates a channel that receives the output of f applied to each element of in.
// When f returns false, the result is not sent on the channel.
func Workers[T1, T2 any](n uint16, in <-chan T1, f func(T1) (T2, bool)) <-chan T2 {
	n = defaultNumberOfWorkers(n)
	c := make(chan T2)
	wg := sync.WaitGroup{}
	defer func() {
		wg.Wait()
		defer close(c)
	}()
	workerChannels := make([]<-chan T2, 0, n)
	for workerIndex := uint16(0); workerIndex < n; workerIndex++ {
		workerChannels = append(workerChannels, worker[T1, T2](in, f))
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

func ApplyElements[T1, T2 any](n uint16, in []T1, f func(T1) T2) []T2 {
	if int(n) > len(in) {
		n = uint16(len(in))
	}
	result := make([]T2, len(in))
	workerMapWithStatus(n, in, f, func(index int, _ int, _ T1, output T2) {
		result[index] = output
	})
	return result
}

func workerMapWithStatus[T1, T2 any](n uint16, in []T1, f func(T1) T2, result func(int, int, T1, T2)) {
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
			c <- valueIndex[T]{v, i}
		}
	}()
	return c
}
