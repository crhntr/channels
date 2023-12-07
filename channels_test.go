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
