package channels

func Drain[T any](c <-chan T) {
	for range c {
	}
}
