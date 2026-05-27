package channels

import "time"

// Throttle emits at most one value per interval d, always the most recent.
// The first value, and the first after an idle gap, is emitted immediately;
// values arriving within d of an emit overwrite a pending slot and are sent
// when the interval elapses. A pending value is flushed when c closes.
func Throttle[T any](c <-chan T, d time.Duration) <-chan T {
	out := make(chan T)
	go func() {
		defer close(out)
		timer := time.NewTimer(d)
		timer.Stop()
		defer timer.Stop()

		var (
			pending T
			held    bool
			tick    <-chan time.Time // non-nil only while an interval is running
		)
		for {
			select {
			case v, ok := <-c:
				if !ok {
					if held {
						out <- pending
					}
					return
				}
				if tick == nil {
					out <- v // leading edge: first value, or first after an idle gap
					timer.Reset(d)
					tick = timer.C
				} else {
					pending, held = v, true
				}
			case <-tick:
				if !held {
					tick = nil // nothing pending: go idle until the next value
					continue
				}
				out <- pending
				var zero T
				pending, held = zero, false
				timer.Reset(d)
			}
		}
	}()
	return out
}
