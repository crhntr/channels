package channels_test

import (
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"testing"
	"testing/synctest"

	"github.com/crhntr/channels"
)

func TestDrain(t *testing.T) {
	t.Run("closed channel", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			c := make(chan int)
			close(c)
			channels.Drain(c)
		})
	})

	t.Run("buffered with values", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			c := make(chan int, 3)
			c <- 1
			c <- 2
			c <- 3
			close(c)
			channels.Drain(c)
		})
	})

	t.Run("concurrent producer", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			c := make(chan int)
			go func() {
				c <- 1
				c <- 2
				c <- 3
				close(c)
			}()
			channels.Drain(c)
		})
	})
}

func TestPipe(t *testing.T) {
	t.Run("values in order", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ch := channels.Pipe(slices.Values([]int{10, 20, 30}))
			var got []int
			for v := range ch {
				got = append(got, v)
			}
			if exp := []int{10, 20, 30}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})

	t.Run("empty iterator", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ch := channels.Pipe(slices.Values([]int(nil)))
			got := slices.Collect(channels.PipeIter(ch))
			if len(got) != 0 {
				t.Errorf("got %v, want empty", got)
			}
		})
	})

	t.Run("closes channel when done", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ch := channels.Pipe(slices.Values([]int{42}))
			<-ch
			synctest.Wait()
			_, ok := <-ch
			if ok {
				t.Error("channel should be closed")
			}
		})
	})
}

func TestTap(t *testing.T) {
	t.Run("values in order", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ch, stop := channels.Tap(slices.Values([]int{10, 20, 30}))
			defer stop()
			var got []int
			for v := range ch {
				got = append(got, v)
			}
			if exp := []int{10, 20, 30}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})

	t.Run("empty iterator", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ch, stop := channels.Tap(slices.Values([]int(nil)))
			defer stop()
			got := slices.Collect(channels.PipeIter(ch))
			if len(got) != 0 {
				t.Error("got: ", got, " exp: empty")
			}
		})
	})

	t.Run("stop cancels early", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ch, stop := channels.Tap(slices.Values([]int{1, 2, 3, 4, 5}))
			got := <-ch
			if got != 1 {
				t.Error("got: ", got, " exp: ", 1)
			}
			stop()
			synctest.Wait()
			_, ok := <-ch
			if ok {
				t.Error("channel should be closed after stop")
			}
		})
	})
}

func TestPipeIter(t *testing.T) {
	t.Run("all values in order", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ch := make(chan int, 3)
			ch <- 10
			ch <- 20
			ch <- 30
			close(ch)
			got := slices.Collect(channels.PipeIter(ch))
			if exp := []int{10, 20, 30}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})

	t.Run("early break", func(t *testing.T) {
		ch := make(chan int, 5)
		for i := range 5 {
			ch <- i
		}
		var got []int
		for v := range channels.PipeIter(ch) {
			got = append(got, v)
			if len(got) == 3 {
				break
			}
		}
		if exp := []int{0, 1, 2}; !slices.Equal(exp, got) {
			t.Error("got: ", got, " exp: ", exp)
		}
	})

	t.Run("closed channel yields nothing", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ch := make(chan string)
			close(ch)
			got := slices.Collect(channels.PipeIter(ch))
			if len(got) != 0 {
				t.Errorf("got %v, want empty", got)
			}
		})
	})

	t.Run("concurrent sender", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ch := make(chan int)
			go func() {
				ch <- 1
				ch <- 2
				close(ch)
			}()
			got := slices.Collect(channels.PipeIter(ch))
			if exp := []int{1, 2}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})
}

func TestCount(t *testing.T) {
	for i := range 5 {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				n := channels.Count(channels.Pipe(slices.Values(make([]int, i))))
				if n != i {
					t.Errorf("got %d, want %d", n, i)
				}
			})
		})
	}

	t.Run("closed channel", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			c := make(chan int)
			close(c)
			if n := channels.Count(c); n != 0 {
				t.Errorf("got %d, want 0", n)
			}
		})
	})
}

func TestFanIn(t *testing.T) {
	t.Run("merges values from multiple channels", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			evens := channels.Pipe(slices.Values([]int{2, 4, 6}))
			odds := channels.Pipe(slices.Values([]int{1, 3, 5}))
			got := slices.Collect(channels.PipeIter(channels.FanIn(evens, odds)))
			slices.Sort(got)
			if exp := []int{1, 2, 3, 4, 5, 6}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})

	t.Run("zero channels", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			n := channels.Count(channels.FanIn[int]())
			if n != 0 {
				t.Errorf("got %d, want 0", n)
			}
		})
	})

	t.Run("single channel", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ch := channels.Pipe(slices.Values([]int{1, 2, 3}))
			got := slices.Collect(channels.PipeIter(channels.FanIn(ch)))
			if exp := []int{1, 2, 3}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})

	t.Run("one channel already closed", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			live := channels.Pipe(slices.Values([]int{1, 2, 3}))
			closed := make(chan int)
			close(closed)
			n := channels.Count(channels.FanIn(closed, live))
			if n != 3 {
				t.Errorf("got %d, want 3", n)
			}
		})
	})

	t.Run("all channels closed", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			c1 := make(chan int)
			c2 := make(chan int)
			close(c1)
			close(c2)
			n := channels.Count(channels.FanIn(c1, c2))
			if n != 0 {
				t.Errorf("got %d, want 0", n)
			}
		})
	})

	t.Run("three channels", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			a := channels.Pipe(slices.Values([]int{1}))
			b := channels.Pipe(slices.Values([]int{2}))
			c := channels.Pipe(slices.Values([]int{3}))
			got := slices.Collect(channels.PipeIter(channels.FanIn(a, b, c)))
			slices.Sort(got)
			if exp := []int{1, 2, 3}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})
}

func TestFanOut(t *testing.T) {
	t.Run("broadcasts to all channels", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := channels.Pipe(slices.Values([]int{1, 2, 3}))
			outs := channels.FanOut(3, in)
			got := slices.Collect(channels.PipeIter(channels.FanIn(outs...)))
			slices.Sort(got)
			if exp := []int{1, 1, 1, 2, 2, 2, 3, 3, 3}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})

	t.Run("single output", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := channels.Pipe(slices.Values([]int{10, 20}))
			outs := channels.FanOut(1, in)
			if len(outs) != 1 {
				t.Fatalf("got %d channels, want 1", len(outs))
			}
			got := slices.Collect(channels.PipeIter(outs[0]))
			if exp := []int{10, 20}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})

	t.Run("each value sent to every output", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make([]int, 50)
			for i := range in {
				in[i] = i
			}
			src := channels.Pipe(slices.Values(in))
			const n = 2
			out := slices.Collect(channels.PipeIter(channels.FanIn(channels.FanOut(n, src)...)))
			for _, v := range in {
				if c := countEqual(out, v); c != n {
					t.Errorf("value %d: got %d copies, want %d", v, c, n)
				}
			}
		})
	})
}

func TestFilter(t *testing.T) {
	t.Run("keeps matching values", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := channels.Pipe(slices.Values([]int{1, 2, 3, 4, 5, 6}))
			evens := channels.Filter(in, func(v int) bool { return v%2 == 0 })
			got := slices.Collect(channels.PipeIter(evens))
			if exp := []int{2, 4, 6}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})

	t.Run("keeps none", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := channels.Pipe(slices.Values([]int{1, 2, 3}))
			out := channels.Filter(in, func(int) bool { return false })
			got := slices.Collect(channels.PipeIter(out))
			if len(got) != 0 {
				t.Errorf("got %v, want empty", got)
			}
		})
	})

	t.Run("keeps all", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := channels.Pipe(slices.Values([]int{1, 2, 3}))
			out := channels.Filter(in, func(int) bool { return true })
			got := slices.Collect(channels.PipeIter(out))
			if exp := []int{1, 2, 3}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})

	t.Run("empty input", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int)
			close(in)
			out := channels.Filter(in, func(int) bool { return true })
			got := slices.Collect(channels.PipeIter(out))
			if len(got) != 0 {
				t.Errorf("got %v, want empty", got)
			}
		})
	})
}

func TestWorkers(t *testing.T) {
	t.Run("transforms values", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := channels.Pipe(slices.Values([]int{1, 2, 3}))
			out := channels.Workers(2, in, func(v int) (string, bool) {
				return strconv.Itoa(v * 10), true
			})
			got := slices.Collect(channels.PipeIter(out))
			slices.Sort(got)
			if exp := []string{"10", "20", "30"}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})

	t.Run("filters values", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := channels.Pipe(slices.Values([]int{1, 2, 3, 4, 5}))
			out := channels.Workers(2, in, func(v int) (int, bool) {
				if v%2 == 0 {
					return v, true
				}
				return 0, false
			})
			got := slices.Collect(channels.PipeIter(out))
			slices.Sort(got)
			if exp := []int{2, 4}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})

	t.Run("single worker", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := channels.Pipe(slices.Values([]int{5, 10, 15}))
			out := channels.Workers(1, in, func(v int) (int, bool) {
				return v * 2, true
			})
			got := slices.Collect(channels.PipeIter(out))
			slices.Sort(got)
			if exp := []int{10, 20, 30}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})

	t.Run("empty input", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int)
			close(in)
			out := channels.Workers(2, in, func(v int) (int, bool) {
				return v, true
			})
			got := slices.Collect(channels.PipeIter(out))
			if len(got) != 0 {
				t.Errorf("got %v, want empty", got)
			}
		})
	})
}

func TestApply(t *testing.T) {
	t.Run("preserves order", func(t *testing.T) {
		in := []int{http.StatusOK, http.StatusNotFound, http.StatusTeapot, http.StatusSeeOther, http.StatusInternalServerError}
		out := channels.Apply(2, in, http.StatusText)
		exp := []string{
			http.StatusText(http.StatusOK),
			http.StatusText(http.StatusNotFound),
			http.StatusText(http.StatusTeapot),
			http.StatusText(http.StatusSeeOther),
			http.StatusText(http.StatusInternalServerError),
		}
		if !slices.Equal(exp, out) {
			t.Error("got: ", out, " exp: ", exp)
		}
	})

	t.Run("zero workers uses NumCPU", func(t *testing.T) {
		in := []float64{25}
		got := channels.Apply(0, in, math.Sqrt)
		if exp := []float64{5}; !slices.Equal(exp, got) {
			t.Error("got: ", got, " exp: ", exp)
		}
	})

	t.Run("more workers than inputs", func(t *testing.T) {
		in := []float64{25}
		got := channels.Apply(10000, in, math.Sqrt)
		if exp := []float64{5}; !slices.Equal(exp, got) {
			t.Error("got: ", got, " exp: ", exp)
		}
	})

	t.Run("empty input", func(t *testing.T) {
		out := channels.Apply(2, []int{}, strconv.Itoa)
		if len(out) != 0 {
			t.Errorf("got %v, want empty", out)
		}
	})

	t.Run("large input preserves order", func(t *testing.T) {
		in := make([]int, 100)
		for i := range in {
			in[i] = i
		}
		out := channels.Apply(4, in, func(v int) int { return v * v })
		for i, v := range out {
			if exp := i * i; v != exp {
				t.Errorf("index %d: got %d, want %d", i, v, exp)
			}
		}
	})
}

func TestMap(t *testing.T) {
	t.Run("transforms type", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := channels.Pipe(slices.Values([]int{1, 2, 3}))
			got := slices.Collect(channels.PipeIter(channels.Map(in, strconv.Itoa)))
			if exp := []string{"1", "2", "3"}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})

	t.Run("empty input", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int)
			close(in)
			got := slices.Collect(channels.PipeIter(channels.Map(in, strconv.Itoa)))
			if len(got) != 0 {
				t.Errorf("got %v, want empty", got)
			}
		})
	})
}

func TestTake(t *testing.T) {
	t.Run("takes first n", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int, 5)
			for _, v := range []int{10, 20, 30, 40, 50} {
				in <- v
			}
			close(in)
			got := slices.Collect(channels.PipeIter(channels.Take(in, 3)))
			if exp := []int{10, 20, 30}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})

	t.Run("n greater than available", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := channels.Pipe(slices.Values([]int{1, 2}))
			got := slices.Collect(channels.PipeIter(channels.Take(in, 10)))
			if exp := []int{1, 2}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})

	t.Run("n is zero", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int)
			close(in)
			got := slices.Collect(channels.PipeIter(channels.Take(in, 0)))
			if len(got) != 0 {
				t.Errorf("got %v, want empty", got)
			}
		})
	})
}

func TestBatch(t *testing.T) {
	t.Run("even split", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := channels.Pipe(slices.Values([]int{1, 2, 3, 4}))
			var got [][]int
			for b := range channels.Batch(in, 2) {
				got = append(got, b)
			}
			if len(got) != 2 {
				t.Fatalf("got %d batches, want 2", len(got))
			}
			if exp := []int{1, 2}; !slices.Equal(exp, got[0]) {
				t.Error("batch 0 got: ", got[0], " exp: ", exp)
			}
			if exp := []int{3, 4}; !slices.Equal(exp, got[1]) {
				t.Error("batch 1 got: ", got[1], " exp: ", exp)
			}
		})
	})

	t.Run("remainder batch", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := channels.Pipe(slices.Values([]int{1, 2, 3, 4, 5}))
			var got [][]int
			for b := range channels.Batch(in, 3) {
				got = append(got, b)
			}
			if len(got) != 2 {
				t.Fatalf("got %d batches, want 2", len(got))
			}
			if exp := []int{1, 2, 3}; !slices.Equal(exp, got[0]) {
				t.Error("batch 0 got: ", got[0], " exp: ", exp)
			}
			if exp := []int{4, 5}; !slices.Equal(exp, got[1]) {
				t.Error("batch 1 got: ", got[1], " exp: ", exp)
			}
		})
	})

	t.Run("single element batches", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := channels.Pipe(slices.Values([]int{1, 2, 3}))
			got := channels.Count(channels.Batch(in, 1))
			if got != 3 {
				t.Error("got: ", got, " exp: ", 3)
			}
		})
	})

	t.Run("empty input", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int)
			close(in)
			got := channels.Count(channels.Batch(in, 5))
			if got != 0 {
				t.Error("got: ", got, " exp: ", 0)
			}
		})
	})
}

func ExampleDrain() {
	in := channels.Pipe(slices.Values([]int{1, 2, 3}))
	channels.Drain(in)
	fmt.Println("done")
	// Output: done
}

func ExamplePipe() {
	ch := channels.Pipe(slices.Values([]int{1, 2, 3}))
	for v := range ch {
		fmt.Println(v)
	}
	// Output:
	// 1
	// 2
	// 3
}

func ExamplePipeIter() {
	ch := make(chan string, 3)
	ch <- "a"
	ch <- "b"
	ch <- "c"
	close(ch)
	for v := range channels.PipeIter(ch) {
		fmt.Println(v)
	}
	// Output:
	// a
	// b
	// c
}

func ExampleCount() {
	in := channels.Pipe(slices.Values([]int{1, 2, 3, 4, 5}))
	fmt.Println(channels.Count(in))
	// Output: 5
}

func ExampleFanIn() {
	a := channels.Pipe(slices.Values([]int{1, 2, 3}))
	b := channels.Pipe(slices.Values([]int{4, 5, 6}))
	got := slices.Collect(channels.PipeIter(channels.FanIn(a, b)))
	slices.Sort(got)
	fmt.Println(got)
	// Output: [1 2 3 4 5 6]
}

func ExampleFanOut() {
	in := channels.Pipe(slices.Values([]int{1, 2, 3}))
	outs := channels.FanOut(2, in)
	got := slices.Collect(channels.PipeIter(channels.FanIn(outs...)))
	slices.Sort(got)
	fmt.Println(got)
	// Output: [1 1 2 2 3 3]
}

func ExampleFanOut_tee() {
	in := channels.Pipe(slices.Values([]int{1, 2, 3}))
	outs := channels.FanOut(2, in)
	doubled := channels.Map(outs[0], func(v int) int { return v * 2 })
	got := slices.Collect(channels.PipeIter(channels.FanIn(doubled, outs[1])))
	slices.Sort(got)
	fmt.Println(got)
	// Output: [1 2 2 3 4 6]
}

func ExampleFilter() {
	in := channels.Pipe(slices.Values([]int{1, 2, 3, 4, 5, 6}))
	for v := range channels.Filter(in, func(v int) bool { return v%2 == 0 }) {
		fmt.Println(v)
	}
	// Output:
	// 2
	// 4
	// 6
}

func ExampleWorkers() {
	in := channels.Pipe(slices.Values([]int{1, 2, 3, 4}))
	out := channels.Workers(2, in, func(v int) (int, bool) {
		return v * v, true
	})
	got := slices.Collect(channels.PipeIter(out))
	slices.Sort(got)
	fmt.Println(got)
	// Output: [1 4 9 16]
}

func ExampleApply() {
	out := channels.Apply(2, []int{1, 2, 3, 4}, func(v int) int { return v * v })
	fmt.Println(out)
	// Output: [1 4 9 16]
}

func ExampleMap() {
	in := channels.Pipe(slices.Values([]int{1, 2, 3}))
	for v := range channels.Map(in, strconv.Itoa) {
		fmt.Println(v)
	}
	// Output:
	// 1
	// 2
	// 3
}

func ExampleTake() {
	in := channels.Pipe(slices.Values([]int{10, 20, 30, 40, 50}))
	for v := range channels.Take(in, 3) {
		fmt.Println(v)
	}
	channels.Drain(in)
	// Output:
	// 10
	// 20
	// 30
}

func ExampleBatch() {
	in := channels.Pipe(slices.Values([]int{1, 2, 3, 4, 5}))
	for batch := range channels.Batch(in, 2) {
		fmt.Println(batch)
	}
	// Output:
	// [1 2]
	// [3 4]
	// [5]
}

func ExamplePipeIter_reduce() {
	in := channels.Pipe(slices.Values([]int{1, 2, 3, 4, 5}))
	sum := 0
	for v := range channels.PipeIter(in) {
		sum += v
	}
	fmt.Println(sum)
	// Output: 15
}

func TestRecent(t *testing.T) {
	t.Run("keeps last n values when reader is slow", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int)
			out := channels.Recent(2, in)

			// send all values without reading
			for i := 1; i <= 5; i++ {
				in <- i
				synctest.Wait()
			}
			close(in)
			synctest.Wait()

			got := slices.Collect(channels.PipeIter(out))
			if exp := []int{4, 5}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})

	t.Run("drops oldest when reader is slow", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int)
			out := channels.Recent(2, in)

			// send 5 values without reading — only last 2 should survive
			for i := 1; i <= 5; i++ {
				in <- i
				synctest.Wait()
			}
			close(in)
			synctest.Wait()

			got := slices.Collect(channels.PipeIter(out))
			if exp := []int{4, 5}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})

	t.Run("forwards values continuously before input closes", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int)
			out := channels.Recent(3, in)

			in <- 1
			synctest.Wait()

			// should be readable now, not waiting for in to close
			got := <-out

			if got != 1 {
				t.Error("got: ", got, " exp: ", 1)
			}

			close(in)
		})
	})
}

func TestBroadcast(t *testing.T) {
	t.Run("broadcasts to multiple listeners", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int)
			b := channels.NewBroadcast(in)

			ch1, cancel1 := b.Subscribe(10)
			ch2, cancel2 := b.Subscribe(10)
			defer cancel1()
			defer cancel2()

			in <- 1
			in <- 2
			in <- 3
			close(in)

			synctest.Wait()

			got1 := slices.Collect(channels.PipeIter(ch1))
			got2 := slices.Collect(channels.PipeIter(ch2))
			slices.Sort(got1)
			slices.Sort(got2)
			if exp := []int{1, 2, 3}; !slices.Equal(exp, got1) {
				t.Error("listener 1 got: ", got1, " exp: ", exp)
			}
			if exp := []int{1, 2, 3}; !slices.Equal(exp, got2) {
				t.Error("listener 2 got: ", got2, " exp: ", exp)
			}
		})
	})

	t.Run("cancel stops delivery", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int)
			b := channels.NewBroadcast(in)

			ch1, cancel1 := b.Subscribe(10)
			ch2, cancel2 := b.Subscribe(10)
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
				t.Error("cancelled listener got: ", got1, " exp: ", exp)
			}
			if exp := []int{1, 2}; !slices.Equal(exp, got2) {
				t.Error("active listener got: ", got2, " exp: ", exp)
			}
		})
	})

	t.Run("close stops all listeners", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int)
			b := channels.NewBroadcast(in)

			ch1, _ := b.Subscribe(10)
			ch2, _ := b.Subscribe(10)

			in <- 1
			synctest.Wait()

			b.Close()
			synctest.Wait()

			got1 := slices.Collect(channels.PipeIter(ch1))
			got2 := slices.Collect(channels.PipeIter(ch2))
			if exp := []int{1}; !slices.Equal(exp, got1) {
				t.Error("listener 1 got: ", got1, " exp: ", exp)
			}
			if exp := []int{1}; !slices.Equal(exp, got2) {
				t.Error("listener 2 got: ", got2, " exp: ", exp)
			}
		})
	})

	t.Run("recent after close returns closed channel", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int)
			b := channels.NewBroadcast(in)
			b.Close()

			ch, cancel := b.Subscribe(10)
			defer cancel()

			_, ok := <-ch
			if ok {
				t.Error("expected closed channel")
			}
		})
	})

	t.Run("recent filter skips non-matching values", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int)
			b := channels.NewBroadcast(in)

			ch, cancel := b.SubscribeFilter(10, func(v int) bool { return v%2 == 0 })
			defer cancel()

			for _, v := range []int{1, 2, 3, 4} {
				in <- v
			}
			close(in)
			synctest.Wait()

			got := slices.Collect(channels.PipeIter(ch))
			if exp := []int{2, 4}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})

	t.Run("multiple listeners with different filters", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int)
			b := channels.NewBroadcast(in)

			evens, cancelEvens := b.SubscribeFilter(10, func(v int) bool { return v%2 == 0 })
			odds, cancelOdds := b.SubscribeFilter(10, func(v int) bool { return v%2 != 0 })
			defer cancelEvens()
			defer cancelOdds()

			for i := 1; i <= 6; i++ {
				in <- i
			}
			close(in)
			synctest.Wait()

			gotEvens := slices.Collect(channels.PipeIter(evens))
			gotOdds := slices.Collect(channels.PipeIter(odds))
			if exp := []int{2, 4, 6}; !slices.Equal(exp, gotEvens) {
				t.Error("evens got: ", gotEvens, " exp: ", exp)
			}
			if exp := []int{1, 3, 5}; !slices.Equal(exp, gotOdds) {
				t.Error("odds got: ", gotOdds, " exp: ", exp)
			}
		})
	})

	t.Run("sliding window keeps last n values", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int)
			b := channels.NewBroadcast(in)

			ch, cancel := b.Subscribe(2)
			defer cancel()

			for i := 1; i <= 5; i++ {
				in <- i
				synctest.Wait()
			}
			close(in)
			synctest.Wait()

			got := slices.Collect(channels.PipeIter(ch))
			if exp := []int{4, 5}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})

	t.Run("buffer stats tracks drops", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int)
			b := channels.NewBroadcast(in)

			if got := b.DropCount(); got != 0 {
				t.Error("initial drops got: ", got, " exp: ", 0)
			}

			_, cancel := b.Subscribe(2)
			defer cancel()

			for i := 1; i <= 5; i++ {
				in <- i
				synctest.Wait()
			}

			if got := b.DropCount(); got != 3 {
				t.Error("drops got: ", got, " exp: ", 3)
			}

			close(in)
		})
	})

	t.Run("input close propagates to listeners", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int)
			b := channels.NewBroadcast(in)

			ch1, _ := b.Subscribe(10)
			ch2, _ := b.Subscribe(10)

			in <- 42
			close(in)
			synctest.Wait()

			got1 := slices.Collect(channels.PipeIter(ch1))
			got2 := slices.Collect(channels.PipeIter(ch2))
			if exp := []int{42}; !slices.Equal(exp, got1) {
				t.Error("listener 1 got: ", got1, " exp: ", exp)
			}
			if exp := []int{42}; !slices.Equal(exp, got2) {
				t.Error("listener 2 got: ", got2, " exp: ", exp)
			}
		})
	})

	t.Run("double cancel is no-op", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int)
			b := channels.NewBroadcast(in)

			_, cancel := b.Subscribe(10)
			cancel()
			cancel() // should not panic

			close(in)
		})
	})

	t.Run("double close is no-op", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int)
			b := channels.NewBroadcast(in)

			b.Close()
			b.Close() // should not panic
		})
	})

	t.Run("recent and cancel during active broadcasting", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int)
			b := channels.NewBroadcast(in)

			ch1, cancel1 := b.Subscribe(10)

			in <- 1
			synctest.Wait()
			cancel1()

			ch2, cancel2 := b.Subscribe(10)
			defer cancel2()

			in <- 2
			close(in)
			synctest.Wait()

			got1 := slices.Collect(channels.PipeIter(ch1))
			got2 := slices.Collect(channels.PipeIter(ch2))
			if exp := []int{1}; !slices.Equal(exp, got1) {
				t.Error("old listener got: ", got1, " exp: ", exp)
			}
			if exp := []int{2}; !slices.Equal(exp, got2) {
				t.Error("new listener got: ", got2, " exp: ", exp)
			}
		})
	})

	t.Run("iterate yields values and stops on break", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int)
			b := channels.NewBroadcast(in)

			go func() {
				for i := 1; i <= 5; i++ {
					in <- i
				}
				close(in)
			}()

			var got []int
			for v := range channels.TapIter(b.Subscribe(10)) {
				got = append(got, v)
				if len(got) == 3 {
					break
				}
			}

			if exp := []int{1, 2, 3}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})

	t.Run("iterate stops when broadcast closes", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int, 2)
			b := channels.NewBroadcast(in)

			in <- 1
			in <- 2
			close(in)

			var got []int
			for v := range channels.TapIter(b.Subscribe(10)) {
				got = append(got, v)
			}
			if exp := []int{1, 2}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})

	t.Run("iterate filter skips non-matching values", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int)
			b := channels.NewBroadcast(in)

			go func() {
				for i := 1; i <= 6; i++ {
					in <- i
				}
				close(in)
			}()

			var got []int
			for v := range channels.TapIter(b.SubscribeFilter(10, func(v int) bool { return v%2 == 0 })) {
				got = append(got, v)
			}

			if exp := []int{2, 4, 6}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})

	t.Run("iterate after close yields nothing", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int)
			b := channels.NewBroadcast(in)
			b.Close()

			if got := slices.Collect(channels.TapIter(b.Subscribe(10))); len(got) != 0 {
				t.Error("got: ", got, " exp: empty")
			}
		})
	})

	t.Run("latest returns most recent value", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := make(chan int)
			b := channels.NewBroadcast(in)

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
