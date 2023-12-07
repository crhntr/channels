package channels

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
