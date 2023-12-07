package channels_test

import (
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
	n := channels.CountReceived(channels.SendEach[int]([]int{1, 2, 3}))
	if n != 3 {
		t.Fatal()
	}
}
