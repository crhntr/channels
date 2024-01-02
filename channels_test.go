package channels_test

import (
	"math"
	"net/http"
	"slices"
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

func TestSendElements(t *testing.T) {
	t.Run("it does not block", func(t *testing.T) {
		channels.Drain(channels.SendElements([]int{1, 2, 3}))
	})
	t.Run("it handles an empty list", func(t *testing.T) {
		channels.Drain(channels.SendElements[int](nil))
	})
}

func TestCountReceived(t *testing.T) {
	for i := 0; i < 3; i++ {
		t.Run("it counts "+strconv.Itoa(i), func(t *testing.T) {
			n := channels.CountReceived(channels.SendElements(make([]int, i)))
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
		evens := channels.SendElements([]int{2, 4, 6})
		odds := channels.SendElements([]int{1, 3, 5})

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
		odds := channels.SendElements([]int{1, 3, 5})
		closed := make(chan int)
		close(closed)

		n := channels.CountReceived(channels.FanIn(closed, odds))
		if n != 3 {
			t.Fail()
		}
	})

	t.Run("2 channels second closed", func(t *testing.T) {
		odds := channels.SendElements([]int{1, 3, 5})
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
			zeros := channels.SendElements(make([]int, 10))
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
		zeros := channels.SendElements(in)
		const numberOfChannels = 2
		out := channels.ReceiveElements(channels.FanIn(channels.FanOut(numberOfChannels, zeros)...))
		for _, v := range in {
			if n := countEqual(out, v); n != numberOfChannels {
				t.Errorf("expected %d of value %d got %d", numberOfChannels, v, n)
			}
		}
	})
}

func TestWorkerMap(t *testing.T) {
	t.Run("it works", func(t *testing.T) {
		in := []int{http.StatusOK, http.StatusNotFound, http.StatusTeapot, http.StatusSeeOther, http.StatusInternalServerError}
		out := channels.ApplyElements(2, in, http.StatusText)
		if exp := []string{
			http.StatusText(http.StatusOK),
			http.StatusText(http.StatusNotFound),
			http.StatusText(http.StatusTeapot),
			http.StatusText(http.StatusSeeOther),
			http.StatusText(http.StatusInternalServerError),
		}; !slices.Equal(exp, out) {
			t.Error("got: ", out, " exp: ", exp)
		}
	})

	t.Run("it handles zero input", func(t *testing.T) {
		in := []float64{25}
		out := channels.ApplyElements(0, in, math.Sqrt)
		if exp := []float64{5}; !slices.Equal(exp, out) {
			t.Error("got: ", out, " exp: ", exp)
		}
	})

	t.Run("it does not need to make too many routines", func(t *testing.T) {
		in := []float64{25}
		out := channels.ApplyElements(10000, in, math.Sqrt)
		if exp := []float64{5}; !slices.Equal(exp, out) {
			t.Error("got: ", out, " exp: ", exp)
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
