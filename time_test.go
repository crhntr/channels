package channels_test

import (
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/crhntr/channels"
)

func TestThrottle(t *testing.T) {
	t.Run("closed input closes output with no values", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int)
			close(in)

			out := channels.Throttle(in, time.Second)

			if got := slices.Collect(channels.PipeIter(out)); len(got) != 0 {
				t.Error("got: ", got, " exp: empty")
			}
		})
	})

	t.Run("first value is emitted immediately", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int)
			out := channels.Throttle(in, time.Hour)
			go func() {
				defer close(in)
				in <- 1
			}()

			start := time.Now()
			got := <-out
			elapsed := time.Since(start)

			if got != 1 {
				t.Error("got: ", got, " exp: ", 1)
			}
			if elapsed > time.Second {
				t.Error("got: ", elapsed, " expected less than a second")
			}
		})
	})

	t.Run("a burst within one interval collapses to the latest value", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int)
			out := channels.Throttle(in, time.Second)
			go func() {
				defer close(in)
				in <- 1
				in <- 2
				in <- 3
			}()

			got := slices.Collect(channels.PipeIter(out))
			if exp := []int{1, 3}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})

	t.Run("a steady stream emits the latest value at each interval boundary", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int)
			out := channels.Throttle(in, time.Second)
			go func() {
				defer close(in)
				for i := 1; i <= 6; i++ {
					in <- i
					time.Sleep(300 * time.Millisecond)
				}
			}()
			got := slices.Collect(channels.PipeIter(out))
			if exp := []int{1, 4, 6}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})

	t.Run("a value after an idle gap is emitted immediately", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int)
			out := channels.Throttle(in, time.Second)
			go func() {
				defer close(in)
				in <- 1
				time.Sleep(2500 * time.Millisecond)
				in <- 2
				time.Sleep(2 * time.Second)
			}()

			start := time.Now()
			var (
				got  []int
				when []time.Duration
			)
			for v := range out {
				got = append(got, v)
				when = append(when, time.Since(start))
			}

			if exp := []int{1, 2}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
			if len(when) == 2 {
				if exp := 2500 * time.Millisecond; when[1] != exp {
					t.Error("got: ", when[1], " exp: ", exp)
				}
			}
		})
	})
}
