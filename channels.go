package channels

import (
	"reflect"
	"slices"
)

func Drain[T any](c <-chan T) {
	for range c {
	}
}

func SendEach[T any](in []T) <-chan T {
	c := make(chan T)
	go func() {
		defer close(c)
		for _, v := range in {
			c <- v
		}
	}()
	return c
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
