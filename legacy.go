package channels

import "iter"

// Send converts an iterator into a channel.
//
// Deprecated: Use [Pipe] instead.
func Send[T any](seq iter.Seq[T]) <-chan T {
	return Pipe(seq)
}

// Receive converts a channel into an iterator.
//
// Deprecated: Use [PipeIter] instead.
func Receive[T any](c <-chan T) iter.Seq[T] {
	return PipeIter(c)
}