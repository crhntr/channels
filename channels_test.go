package channels_test

import (
	"strconv"
	"testing"

	"github.com/crhntr/channels"
)

func TestDrain_close(t *testing.T) {
	c := make(chan int)
	close(c)
	channels.Drain(c)
}

func TestDrain_receive(t *testing.T) {
	c := make(chan int, 1)
	c <- 1
	close(c)
	channels.Drain(c)
}

func TestSendSliceElements(t *testing.T) {
	t.Run("it does not block", func(t *testing.T) {
		channels.Drain(channels.SendSliceElements([]int{1, 2, 3}))
	})
	t.Run("it handles an empty list", func(t *testing.T) {
		channels.Drain(channels.SendSliceElements[int](nil))
	})
}

func TestCountReceived(t *testing.T) {
	for i := 0; i < 3; i++ {
		t.Run("it counts "+strconv.Itoa(i), func(t *testing.T) {
			n := channels.CountReceived(channels.SendSliceElements(make([]int, i)))
			if n != i {
				t.Fail()
			}
		})
	}
	t.Run("closed", func(t *testing.T) {
		c := make(chan int)
		close(c)
		n := channels.CountReceived(c)
		if n != 0 {
			t.Fail()
		}
	})
}

func TestFanIn(t *testing.T) {
	t.Run("2 channels", func(t *testing.T) {
		evens := channels.SendSliceElements([]int{2, 4, 6})
		odds := channels.SendSliceElements([]int{1, 3, 5})

		n := channels.CountReceived(channels.FanIn(evens, odds))
		if n != 6 {
			t.Fail()
		}
	})
	t.Run("0 channels", func(t *testing.T) {
		n := channels.CountReceived(channels.FanIn[int]())
		if n != 0 {
			t.Fail()
		}
	})

	t.Run("2 channels first closed", func(t *testing.T) {
		odds := channels.SendSliceElements([]int{1, 3, 5})
		closed := make(chan int)
		close(closed)

		n := channels.CountReceived(channels.FanIn(closed, odds))
		if n != 3 {
			t.Fail()
		}
	})

	t.Run("2 channels second closed", func(t *testing.T) {
		odds := channels.SendSliceElements([]int{1, 3, 5})
		closed := make(chan int)
		close(closed)

		n := channels.CountReceived(channels.FanIn(odds, closed))
		if n != 3 {
			t.Fail()
		}
	})

	t.Run("2 channels closed", func(t *testing.T) {
		closed1 := make(chan int)
		closed2 := make(chan int)
		close(closed1)
		close(closed2)

		n := channels.CountReceived(channels.FanIn(closed1, closed2))
		if n != 0 {
			t.Fail()
		}
	})
}

func TestFanOut(t *testing.T) {
	t.Run("it consistently counts", func(t *testing.T) {
		for i := 0; i < 10; i++ {
			zeros := channels.SendSliceElements(make([]int, 10))
			n := channels.CountReceived(channels.FanIn(channels.FanOut(5, zeros)...))
			if exp := 50; n != exp {
				t.Error("got: ", n, " exp: ", exp)
			}
		}
	})
	t.Run("it send the same value to all channels", func(t *testing.T) {
		in := make([]int, 100)
		for i := range in {
			in[i] = i
		}
		zeros := channels.SendSliceElements(in)
		const numberOfChannels = 2
		out := channels.ReceiveElements(channels.FanIn(channels.FanOut(numberOfChannels, zeros)...))
		for _, v := range in {
			if n := countEqual(out, v); n != numberOfChannels {
				t.Errorf("expected %d of value %d got %d", numberOfChannels, v, n)
			}
		}
	})
}

func countEqual[T comparable](slice []T, val T) int {
	n := 0
	for _, v := range slice {
		if v == val {
			n++
		}
	}
	return n
}

func countEqualFunc[T any](slice []T, val T, equal func(T, T) bool) int {
	n := 0
	for _, v := range slice {
		if equal(v, val) {
			n++
		}
	}
	return n
}
