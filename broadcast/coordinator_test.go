package broadcast_test

import (
	"slices"
	"strconv"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/crhntr/channels"
	"github.com/crhntr/channels/broadcast"
)

func TestBroadcast_SubscribeAll(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		in := make(chan int)
		b := broadcast.New(in)

		ch, cancel := b.SubscribeAll(10)
		defer cancel()

		in <- 1
		in <- 2
		in <- 3
		close(in)

		synctest.Wait()

		got := slices.Collect(channels.PipeIter(ch))
		if exp := []int{1, 2, 3}; !slices.Equal(exp, got) {
			t.Error("got: ", got, " exp: ", exp)
		}
	})
}

func TestBroadcast_SubscribeAll_unbuffered_does_not_deadlock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		in := make(chan int)
		b := broadcast.New(in)

		ch, cancel := b.SubscribeAll(0)
		defer cancel()

		go func() {
			in <- 1
			in <- 2
			in <- 3
			close(in)
		}()

		var got []int
		for v := range ch {
			got = append(got, v)
		}
		if exp := []int{1, 2, 3}; !slices.Equal(exp, got) {
			t.Error("got: ", got, " exp: ", exp)
		}
	})
}

func TestBroadcast_SubscribeAll_slow_subscriber_blocks_others(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		in := make(chan int)
		b := broadcast.New(in)

		// Slow subscriber: buffer of 1, won't be drained.
		slow, cancelSlow := b.SubscribeAll(1)
		defer cancelSlow()

		in <- 1
		synctest.Wait()

		// Second send: coordinator receives it from in, then blocks on slow.ch <- 2.
		go func() { in <- 2 }()
		synctest.Wait()

		// The coordinator is now blocked delivering to slow. A new subscribe
		// should not complete because the coordinator can't process it.
		subscribed := make(chan struct{})
		go func() {
			ch, cancel := b.SubscribeAll(10)
			defer cancel()
			close(subscribed)
			for range ch {
			}
		}()

		synctest.Wait()

		select {
		case <-subscribed:
			t.Error("expected subscribe to block while coordinator is stuck on slow subscriber")
		default:
		}

		// Drain slow to unblock the coordinator.
		<-slow // value 1
		synctest.Wait()
		<-slow // value 2
		synctest.Wait()

		// Now the coordinator is free and subscribe should complete.
		<-subscribed

		close(in)
	})
}

func TestBroadcast_SubscribeLatest_drops_oldest_and_tracks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		in := make(chan int)
		b := broadcast.New(in)

		ch, cancel := b.SubscribeLatest(2)
		defer cancel()

		if got := b.DropCount(); got != 0 {
			t.Error("initial drops got: ", got, " exp: ", 0)
		}

		for i := 1; i <= 5; i++ {
			in <- i
			synctest.Wait()
		}

		if got := b.DropCount(); got != 3 {
			t.Error("drops got: ", got, " exp: ", 3)
		}

		close(in)
		synctest.Wait()

		got := slices.Collect(channels.PipeIter(ch))
		if exp := []int{4, 5}; !slices.Equal(exp, got) {
			t.Error("got: ", got, " exp: ", exp)
		}
	})
}

func TestBroadcast_SubscribeLatest_zero_uses_buffer_of_one(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		in := make(chan int)
		b := broadcast.New(in)

		ch, cancel := b.SubscribeLatest(0)
		defer cancel()

		in <- 1
		in <- 2
		in <- 3
		synctest.Wait()

		close(in)
		synctest.Wait()

		got := slices.Collect(channels.PipeIter(ch))
		if exp := []int{3}; !slices.Equal(exp, got) {
			t.Error("got: ", got, " exp: ", exp)
		}
	})
}

func TestBroadcast_SubscribeAll_filtered(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		in := make(chan int)
		b := broadcast.New(in)

		ch, cancel := b.SubscribeAll(10, func(v int) bool { return v%2 == 0 })
		defer cancel()

		for _, v := range []int{1, 2, 3, 4, 5, 6} {
			in <- v
		}
		close(in)
		synctest.Wait()

		got := slices.Collect(channels.PipeIter(ch))
		if exp := []int{2, 4, 6}; !slices.Equal(exp, got) {
			t.Error("got: ", got, " exp: ", exp)
		}
	})
}

func TestBroadcast_SubscribeLatest_filtered(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		in := make(chan int)
		b := broadcast.New(in)

		ch, cancel := b.SubscribeLatest(10, func(v int) bool { return v%2 != 0 })
		defer cancel()

		for i := 1; i <= 6; i++ {
			in <- i
		}
		close(in)
		synctest.Wait()

		got := slices.Collect(channels.PipeIter(ch))
		if exp := []int{1, 3, 5}; !slices.Equal(exp, got) {
			t.Error("got: ", got, " exp: ", exp)
		}
	})
}

func TestBroadcast_cancel_removes_subscriber_and_closes_channel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		in := make(chan int)
		b := broadcast.New(in)

		ch1, cancel1 := b.SubscribeAll(10)
		ch2, cancel2 := b.SubscribeAll(10)
		defer cancel2()

		in <- 1
		synctest.Wait()
		cancel1()

		in <- 2
		close(in)
		synctest.Wait()

		got1 := slices.Collect(channels.PipeIter(ch1))
		got2 := slices.Collect(channels.PipeIter(ch2))
		if exp := []int{1}; !slices.Equal(exp, got1) {
			t.Error("cancelled got: ", got1, " exp: ", exp)
		}
		if exp := []int{1, 2}; !slices.Equal(exp, got2) {
			t.Error("active got: ", got2, " exp: ", exp)
		}
	})
}

func TestBroadcast_double_cancel_is_noop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		in := make(chan int)
		b := broadcast.New(in)

		_, cancel := b.SubscribeAll(10)
		cancel()
		cancel() // should not panic

		close(in)
	})
}

func TestBroadcast_input_close_propagates_to_subscribers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		in := make(chan int)
		b := broadcast.New(in)

		ch1, _ := b.SubscribeAll(10)
		ch2, _ := b.SubscribeLatest(10)

		in <- 42
		close(in)
		synctest.Wait()

		got1 := slices.Collect(channels.PipeIter(ch1))
		got2 := slices.Collect(channels.PipeIter(ch2))
		if exp := []int{42}; !slices.Equal(exp, got1) {
			t.Error("SubscribeAll got: ", got1, " exp: ", exp)
		}
		if exp := []int{42}; !slices.Equal(exp, got2) {
			t.Error("SubscribeLatest got: ", got2, " exp: ", exp)
		}
	})
}

func TestBroadcast_double_close_is_noop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		in := make(chan int)
		b := broadcast.New(in)

		b.Close()
		b.Close() // should not panic
	})
}

func TestBroadcast_concurrent_close(t *testing.T) {
	for range 200 {
		in := make(chan int, 1)
		b := broadcast.New(in)
		in <- 1

		var wg sync.WaitGroup
		for range 10 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				b.Close()
			}()
		}
		wg.Wait()
		close(in)
	}
}

func TestBroadcast_Latest_returns_most_recent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		in := make(chan int)
		b := broadcast.New(in)

		if _, ok := b.Latest(); ok {
			t.Error("expected no value before any sends")
		}

		in <- 1
		synctest.Wait()
		if got, ok := b.Latest(); !ok || got != 1 {
			t.Error("got: ", got, " ok: ", ok, " exp: 1 true")
		}

		in <- 2
		in <- 3
		synctest.Wait()
		if got, ok := b.Latest(); !ok || got != 3 {
			t.Error("got: ", got, " ok: ", ok, " exp: 3 true")
		}

		close(in)
	})
}

func TestBroadcast_subscribe_after_close_returns_closed_channel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		in := make(chan int)
		b := broadcast.New(in)
		b.Close()

		ch1, cancel1 := b.SubscribeAll(10)
		defer cancel1()
		if _, ok := <-ch1; ok {
			t.Error("expected closed SubscribeAll channel")
		}

		ch2, cancel2 := b.SubscribeLatest(10)
		defer cancel2()
		if _, ok := <-ch2; ok {
			t.Error("expected closed SubscribeLatest channel")
		}
	})
}

func TestBroadcast_subscribe_after_input_closes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		in := make(chan int, 1)
		in <- 1
		close(in)
		b := broadcast.New(in)
		synctest.Wait()

		ch, cancel := b.SubscribeAll(10)
		defer cancel()
		if _, ok := <-ch; ok {
			t.Error("expected closed channel after input closed")
		}
	})
}

func TestBroadcast_SubscribeAll_multiple_subscribers_each_receive_all(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		in := make(chan int)
		b := broadcast.New(in)

		ch1, cancel1 := b.SubscribeAll(10)
		ch2, cancel2 := b.SubscribeAll(10)
		defer cancel1()
		defer cancel2()

		in <- 1
		in <- 2
		in <- 3
		close(in)

		synctest.Wait()

		got1 := slices.Collect(channels.PipeIter(ch1))
		got2 := slices.Collect(channels.PipeIter(ch2))
		if exp := []int{1, 2, 3}; !slices.Equal(exp, got1) {
			t.Error("subscriber 1 got: ", got1, " exp: ", exp)
		}
		if exp := []int{1, 2, 3}; !slices.Equal(exp, got2) {
			t.Error("subscriber 2 got: ", got2, " exp: ", exp)
		}
	})
}

func TestBroadcast_TapIter_collects_all_and_cancels(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		in := make(chan int)
		b := broadcast.New(in)

		ch, cancel := b.SubscribeAll(10)

		in <- 1
		in <- 2
		in <- 3
		close(in)
		synctest.Wait()

		got := slices.Collect(channels.TapIter(ch, cancel))
		if exp := []int{1, 2, 3}; !slices.Equal(exp, got) {
			t.Error("got: ", got, " exp: ", exp)
		}
	})
}

func TestBroadcast_TapIter_break_cancels_subscription(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		in := make(chan int)
		b := broadcast.New(in)

		ch, cancel := b.SubscribeAll(10)

		go func() {
			for i := 1; i <= 5; i++ {
				in <- i
			}
		}()

		var got []int
		for v := range channels.TapIter(ch, cancel) {
			got = append(got, v)
			if len(got) == 3 {
				break
			}
		}
		if exp := []int{1, 2, 3}; !slices.Equal(exp, got) {
			t.Error("got: ", got, " exp: ", exp)
		}

		close(in)
	})
}

func TestBroadcast_TapIter_with_SubscribeLatest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		in := make(chan int)
		b := broadcast.New(in)

		ch, cancel := b.SubscribeLatest(10)

		go func() {
			for i := 1; i <= 6; i++ {
				in <- i
			}
			close(in)
		}()

		var got []int
		for v := range channels.TapIter(ch, cancel) {
			got = append(got, v)
		}
		if exp := []int{1, 2, 3, 4, 5, 6}; !slices.Equal(exp, got) {
			t.Error("got: ", got, " exp: ", exp)
		}
	})
}

func BenchmarkBroadcast_SubscribeLatest_Send(b *testing.B) {
	for _, subCount := range []int{1, 10, 100} {
		b.Run(strconv.Itoa(subCount), func(b *testing.B) {
			in := make(chan int, 1)
			bc := broadcast.New(in)

			cancels := make([]func(), subCount)
			for i := range subCount {
				// SubscribeLatest never blocks the coordinator.
				_, cancels[i] = bc.SubscribeLatest(8)
			}

			for b.Loop() {
				in <- 0
			}

			for _, cancel := range cancels {
				cancel()
			}
			bc.Close()
		})
	}
}

func BenchmarkBroadcast_SubscribeLatest_SlidingDrop(b *testing.B) {
	in := make(chan int, 1)
	bc := broadcast.New(in)
	defer bc.Close()

	_, cancel := bc.SubscribeLatest(1)
	defer cancel()

	for b.Loop() {
		in <- 0
	}
}

func BenchmarkBroadcast_SubscribeCancel(b *testing.B) {
	in := make(chan int)
	bc := broadcast.New(in)
	defer bc.Close()

	for b.Loop() {
		_, cancel := bc.SubscribeLatest(1)
		cancel()
	}
}

func BenchmarkBroadcast_LargeValue(b *testing.B) {
	type large [4096]byte

	in := make(chan large, 1)
	bc := broadcast.New(in)
	defer bc.Close()

	_, cancel := bc.SubscribeLatest(8)
	defer cancel()

	var v large
	v[0] = 1

	for b.Loop() {
		in <- v
	}
}

// Issue 1: Cancel while coordinator is blocked delivering to THIS subscriber.
// cancel() closes s.canceled which unblocks the coordinator's select. Then
// cancel sends on b.unsub. The coordinator must not send on s.ch after unsub
// closes it, and must not double-close s.ch.
func TestBroadcast_CancelWhileBlockedOnSameSubscriber(t *testing.T) {
	for range 500 {
		in := make(chan int, 1) // buffered so goroutine never blocks on in
		bc := broadcast.New(in)

		// Unbuffered SubscribeAll — coordinator blocks on s.ch <- v.
		ch, cancel := bc.SubscribeAll(0)

		in <- 1

		// cancel unblocks coordinator via s.canceled.
		cancel()

		// ch should be closed. Drain should not block.
		for range ch {
		}

		bc.Close()
		close(in)
	}
}

// Issue 2: Cancel races with run exit. cancel() closes s.canceled and then
// tries b.unsub. If run already exited (closed b.done), cancel selects b.done.
// But run's defer already closed s.ch. Verify no double-close panic.
func TestBroadcast_CancelRacesWithRunExit(t *testing.T) {
	for range 500 {
		in := make(chan int, 1)
		bc := broadcast.New(in)

		_, cancel := bc.SubscribeAll(4)
		in <- 1
		close(in) // triggers run exit

		// cancel might arrive before or after run exits
		cancel()
	}
}

// Issue 3: Cancel while coordinator is blocked delivering to subscriber 1.
// Cancelling subscriber 1 unblocks the coordinator, which then moves on
// to deliver to subscriber 2. But cancel's unsub send also blocks until
// the coordinator finishes delivery and returns to its top-level select.
// The reader for subscriber 2 must be ready in a separate goroutine.
func TestBroadcast_CancelFirstUnbufferedUnblocksSecond(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		in := make(chan int)
		bc := broadcast.New(in)

		_, cancel1 := bc.SubscribeAll(0)
		ch2, cancel2 := bc.SubscribeAll(0)
		defer cancel2()

		go func() { in <- 42 }()

		// Reader for subscriber 2 must be ready before cancel1,
		// because cancel1's unsub send blocks until the coordinator
		// finishes delivering to all subscribers (including sub2).
		got := make(chan int, 1)
		go func() { got <- <-ch2 }()

		synctest.Wait()
		cancel1()
		synctest.Wait()

		if v := <-got; v != 42 {
			t.Error("got: ", v, " exp: ", 42)
		}

		close(in)
	})
}

// Issue 4: After cancel closes s.canceled, the subscriber stays in the subs
// slice until the coordinator processes the unsub message. If values arrive
// between cancel and unsub processing, the coordinator hits <-s.canceled
// (already closed) and drops values. This is expected but must not panic.
func TestBroadcast_ValuesBetweenCancelAndUnsub(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		in := make(chan int)
		bc := broadcast.New(in)

		ch1, cancel1 := bc.SubscribeAll(1)
		ch2, cancel2 := bc.SubscribeAll(10)
		defer cancel2()

		in <- 1
		synctest.Wait()
		<-ch1 // drain so coordinator can continue

		// Cancel subscriber 1. Its canceled channel is now closed.
		cancel1()

		// Send more values — coordinator will try to deliver to sub1
		// (still in slice), hit <-s.canceled, and skip. Must not panic.
		in <- 2
		in <- 3
		synctest.Wait()

		close(in)
		synctest.Wait()

		// Sub2 should have all values.
		var got []int
		for v := range ch2 {
			got = append(got, v)
		}
		if len(got) < 2 {
			t.Error("expected sub2 to receive values, got: ", got)
		}
	})
}

// Issue 5: Close() while coordinator is blocked delivering to an unbuffered
// SubscribeAll. Close closes b.closed. But the coordinator is in the inner
// select (s.ch <- v / s.canceled), which does NOT select on b.closed.
// The coordinator is stuck until someone reads from s.ch or cancel is called.
func TestBroadcast_CloseWhileBlockedOnUnbufferedSubscriber(t *testing.T) {
	for range 200 {
		in := make(chan int, 1) // buffered so we don't need a goroutine
		bc := broadcast.New(in)

		ch, cancel := bc.SubscribeAll(0)

		in <- 1

		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			bc.Close()
		}()

		// Cancel to unblock coordinator via s.canceled.
		cancel()
		for range ch {
		}

		wg.Wait()
		close(in)
	}
}

// Issue 6: Close does NOT interrupt the coordinator's inner delivery select.
// If the coordinator is blocked on `s.ch <- v` and nobody cancels or reads,
// Close() returns immediately but the coordinator goroutine is leaked.
func TestBroadcast_CloseDoesNotUnblockInnerSelect(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		in := make(chan int)
		bc := broadcast.New(in)

		// Unbuffered subscriber, no reader.
		_, cancel := bc.SubscribeAll(0)
		defer cancel()

		go func() { in <- 1 }()
		synctest.Wait()

		// Coordinator is now blocked in: select { case s.ch <- v: case <-s.canceled: }
		// Close() closes b.closed but coordinator won't see it — it's in the INNER select.
		bc.Close()
		synctest.Wait()

		// The coordinator is STILL blocked. b.done is NOT closed.
		// This is the leak. Verify by checking that subscribe detects
		// the closed state (via b.closed, not b.done).
		ch, cancel2 := bc.SubscribeLatest(1)
		cancel2()
		_, ok := <-ch
		if ok {
			t.Error("expected closed channel from subscribe after Close")
		}
	})
}

// Issue 7: Send on closed channel. After unsub closes s.ch, can the
// coordinator's Slide still write to it? The coordinator removes s from
// subs in the unsub handler, so the next delivery iteration won't see it.
// But what if a value is being delivered RIGHT NOW to other subscribers,
// and this subscriber was already iterated?
// Answer: unsub only runs between deliveries (in the top-level select),
// never mid-delivery. So this should be safe.
func TestBroadcast_UnsubBetweenDeliveries(t *testing.T) {
	for range 500 {
		in := make(chan int)
		bc := broadcast.New(in)

		_, cancel := bc.SubscribeLatest(1)
		ch2, cancel2 := bc.SubscribeLatest(10)
		defer cancel2()

		go func() {
			for i := range 100 {
				in <- i
			}
			close(in)
		}()

		// Cancel first subscriber mid-stream.
		cancel()

		// Drain second subscriber.
		for range ch2 {
		}
	}
}

// Issue 8: Concurrent cancel and Close — cancel's once.Do closes s.canceled
// then tries b.unsub. Close closes b.closed. If Close wins, run exits,
// defer closes s.ch and b.done. Then cancel's b.unsub select hits b.done.
// If cancel wins, unsub removes s, closes s.ch. Then Close triggers run exit,
// defer skips s (already removed). No double-close either way.
func TestBroadcast_ConcurrentCancelAndClose(t *testing.T) {
	for range 500 {
		in := make(chan int, 1)
		bc := broadcast.New(in)

		_, cancel := bc.SubscribeAll(4)
		in <- 1

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			cancel()
		}()
		go func() {
			defer wg.Done()
			bc.Close()
		}()
		wg.Wait()
		close(in)
	}
}

// Issue 9: SubscribeAll cancel that never reaches the coordinator because run
// already exited. cancel closes s.canceled (harmless) then selects b.done.
// s.ch was already closed by run's defer. Verify no panic.
func TestBroadcast_CancelAfterRunExited(t *testing.T) {
	for range 500 {
		in := make(chan int, 3)
		in <- 1
		in <- 2
		in <- 3
		close(in)

		bc := broadcast.New(in)

		ch, cancel := bc.SubscribeAll(10)
		// Drain — run will exit when it sees in is closed.
		for range ch {
		}

		// cancel after run has exited and s.ch is closed by defer
		cancel()
	}
}

// Issue 10: Hammer everything at once — subscribe, cancel, send, close, Latest,
// DropCount from many goroutines.
func TestBroadcast_FullConcurrencyStress(t *testing.T) {
	in := make(chan int)
	bc := broadcast.New(in)

	var wg sync.WaitGroup

	// Sender
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range 1000 {
			select {
			case in <- i:
			default:
			}
		}
		close(in)
	}()

	// Subscribers churning
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				ch, cancel := bc.SubscribeAll(2)
				select {
				case <-ch:
				default:
				}
				cancel()
			}
		}()
	}

	// SubscribeLatest churning
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				ch, cancel := bc.SubscribeLatest(2)
				select {
				case <-ch:
				default:
				}
				cancel()
			}
		}()
	}

	// Latest and DropCount readers
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 500 {
			bc.Latest()
			bc.DropCount()
			bc.ResetDropCount()
		}
	}()

	// Close from another goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		bc.Close()
	}()

	wg.Wait()
}
