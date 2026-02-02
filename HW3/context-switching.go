package main

import (
	"fmt"
	"runtime"
	"time"
)

// Total channel handoffs = 2*N.
// We measure total time and compute avg per handoff.
func pingPong(N int) (total time.Duration, avgPerHandoff time.Duration) {
	ch := make(chan struct{}) // unbuffered
	done := make(chan struct{})

	// Goroutine A: initiator (ping)
	go func() {
		for i := 0; i < N; i++ {
			ch <- struct{}{} // send to pong
			<-ch             // receive back from pong
		}
		close(done)
	}()

	// Goroutine B: responder (pong)
	go func() {
		for i := 0; i < N; i++ {
			<-ch             // receive from ping
			ch <- struct{}{} // send back to ping
		}
	}()

	start := time.Now()
	<-done
	total = time.Since(start)

	// 2 handoffs per round-trip
	avgPerHandoff = total / time.Duration(2*N)
	return total, avgPerHandoff
}

func runCase(label string, procs int, N int) {
	// Restrict Go scheduler to "procs" logical CPUs (P's).
	// procs=1 tends to keep both goroutines on the same OS thread most of the time.
	prev := runtime.GOMAXPROCS(procs)
	defer runtime.GOMAXPROCS(prev)

	// Warm-up (helps avoid one-time effects like first-time scheduling overhead)
	_, _ = pingPong(10_000)

	total, avg := pingPong(N)
	fmt.Printf("%s | GOMAXPROCS=%d | round-trips=%d | total=%v | avg/hand-off=%v\n",
		label, procs, N, total, avg)
}

func main() {
	N := 1_000_000

	fmt.Printf("Go version: %s\n", runtime.Version())
	fmt.Printf("NumCPU: %d\n\n", runtime.NumCPU())

	runCase("Single-thread", 1, N)

	// Use all CPUs available (you can also pick 2 explicitly)
	runCase("Multi-thread", runtime.NumCPU(), N)
}
