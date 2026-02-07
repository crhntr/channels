package channels_test

import (
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

func TestSend(t *testing.T) {
	t.Run("values in order", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ch := channels.Send(slices.Values([]int{10, 20, 30}))
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
			ch := channels.Send(slices.Values([]int(nil)))
			got := slices.Collect(channels.Receive(ch))
			if len(got) != 0 {
				t.Errorf("got %v, want empty", got)
			}
		})
	})

	t.Run("closes channel when done", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ch := channels.Send(slices.Values([]int{42}))
			<-ch
			synctest.Wait()
			_, ok := <-ch
			if ok {
				t.Error("channel should be closed")
			}
		})
	})
}

func TestReceive(t *testing.T) {
	t.Run("all values in order", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ch := make(chan int, 3)
			ch <- 10
			ch <- 20
			ch <- 30
			close(ch)
			got := slices.Collect(channels.Receive(ch))
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
		for v := range channels.Receive(ch) {
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
			got := slices.Collect(channels.Receive(ch))
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
			got := slices.Collect(channels.Receive(ch))
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
				n := channels.Count(channels.Send(slices.Values(make([]int, i))))
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
			evens := channels.Send(slices.Values([]int{2, 4, 6}))
			odds := channels.Send(slices.Values([]int{1, 3, 5}))
			got := slices.Collect(channels.Receive(channels.FanIn(evens, odds)))
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
			ch := channels.Send(slices.Values([]int{1, 2, 3}))
			got := slices.Collect(channels.Receive(channels.FanIn(ch)))
			if exp := []int{1, 2, 3}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})

	t.Run("one channel already closed", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			live := channels.Send(slices.Values([]int{1, 2, 3}))
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
			a := channels.Send(slices.Values([]int{1}))
			b := channels.Send(slices.Values([]int{2}))
			c := channels.Send(slices.Values([]int{3}))
			got := slices.Collect(channels.Receive(channels.FanIn(a, b, c)))
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
			in := channels.Send(slices.Values([]int{1, 2, 3}))
			outs := channels.FanOut(3, in)
			got := slices.Collect(channels.Receive(channels.FanIn(outs...)))
			slices.Sort(got)
			if exp := []int{1, 1, 1, 2, 2, 2, 3, 3, 3}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})

	t.Run("single output", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := channels.Send(slices.Values([]int{10, 20}))
			outs := channels.FanOut(1, in)
			if len(outs) != 1 {
				t.Fatalf("got %d channels, want 1", len(outs))
			}
			got := slices.Collect(channels.Receive(outs[0]))
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
			src := channels.Send(slices.Values(in))
			const n = 2
			out := slices.Collect(channels.Receive(channels.FanIn(channels.FanOut(n, src)...)))
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
			in := channels.Send(slices.Values([]int{1, 2, 3, 4, 5, 6}))
			evens := channels.Filter(in, func(v int) bool { return v%2 == 0 })
			got := slices.Collect(channels.Receive(evens))
			if exp := []int{2, 4, 6}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})

	t.Run("keeps none", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := channels.Send(slices.Values([]int{1, 2, 3}))
			out := channels.Filter(in, func(int) bool { return false })
			got := slices.Collect(channels.Receive(out))
			if len(got) != 0 {
				t.Errorf("got %v, want empty", got)
			}
		})
	})

	t.Run("keeps all", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := channels.Send(slices.Values([]int{1, 2, 3}))
			out := channels.Filter(in, func(int) bool { return true })
			got := slices.Collect(channels.Receive(out))
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
			got := slices.Collect(channels.Receive(out))
			if len(got) != 0 {
				t.Errorf("got %v, want empty", got)
			}
		})
	})
}

func TestWorkers(t *testing.T) {
	t.Run("transforms values", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := channels.Send(slices.Values([]int{1, 2, 3}))
			out := channels.Workers(2, in, func(v int) (string, bool) {
				return strconv.Itoa(v * 10), true
			})
			got := slices.Collect(channels.Receive(out))
			slices.Sort(got)
			if exp := []string{"10", "20", "30"}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})

	t.Run("filters values", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := channels.Send(slices.Values([]int{1, 2, 3, 4, 5}))
			out := channels.Workers(2, in, func(v int) (int, bool) {
				if v%2 == 0 {
					return v, true
				}
				return 0, false
			})
			got := slices.Collect(channels.Receive(out))
			slices.Sort(got)
			if exp := []int{2, 4}; !slices.Equal(exp, got) {
				t.Error("got: ", got, " exp: ", exp)
			}
		})
	})

	t.Run("single worker", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			in := channels.Send(slices.Values([]int{5, 10, 15}))
			out := channels.Workers(1, in, func(v int) (int, bool) {
				return v * 2, true
			})
			got := slices.Collect(channels.Receive(out))
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
			got := slices.Collect(channels.Receive(out))
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

func countEqual[T comparable](slice []T, val T) int {
	n := 0
	for _, v := range slice {
		if v == val {
			n++
		}
	}
	return n
}
