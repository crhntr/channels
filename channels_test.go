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

func TestSendEach(t *testing.T) {
	t.Run("it does not block", func(t *testing.T) {
		channels.Drain(channels.SendEach([]int{1, 2, 3}))
	})
	t.Run("it handles an empty list", func(t *testing.T) {
		channels.Drain(channels.SendEach[int](nil))
	})
}

func TestCountReceived(t *testing.T) {
	for i := 0; i < 3; i++ {
		t.Run("it counts "+strconv.Itoa(i), func(t *testing.T) {
			n := channels.CountReceived(channels.SendEach(make([]int, i)))
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
		evens := channels.SendEach([]int{2, 4, 6})
		odds := channels.SendEach([]int{1, 3, 5})

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
		odds := channels.SendEach([]int{1, 3, 5})
		closed := make(chan int)
		close(closed)

		n := channels.CountReceived(channels.FanIn(closed, odds))
		if n != 3 {
			t.Fail()
		}
	})

	t.Run("2 channels second closed", func(t *testing.T) {
		odds := channels.SendEach([]int{1, 3, 5})
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
