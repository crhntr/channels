package channels

import (
	"reflect"
	"slices"
)

func Drain[T any](c <-chan T) {
	for range c {
	}
}

func SendSliceElements[T any](in []T) <-chan T {
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

func FanOut[T any](n int, in <-chan T) []<-chan T {
	channels := make([]chan T, n)
	for i := range channels {
		channels[i] = make(chan T)
	}
	go fanOut(in, sendOnly(channels))
	return receiveOnly(channels)
}

func fanOut[T any](in <-chan T, channels []chan<- T) {
	defer closeAll(channels)
	for v := range in {
		val := reflect.ValueOf(v)
		cases := make([]reflect.SelectCase, len(channels))
		for i := range channels {
			if channels[i] == nil {
				continue
			}
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
