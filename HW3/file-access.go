package main

import (
	"bufio"
	"fmt"
	"os"
	"time"
)

const (
	N        = 100_000
	filename = "out.txt"
)

func unbufferedWrite() time.Duration {
	f, err := os.Create(filename) // truncate/create
	if err != nil {
		panic(err)
	}
	defer f.Close()

	start := time.Now()

	for i := 0; i < N; i++ {
		// One write per line (small write, many syscalls)
		line := fmt.Sprintf("line %d\n", i)
		if _, err := f.Write([]byte(line)); err != nil {
			panic(err)
		}
	}

	// After the loop, close the file and compute the elapsed time.
	return time.Since(start)
}

func bufferedWrite() time.Duration {
	f, err := os.Create(filename) // truncate/create
	if err != nil {
		panic(err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)

	start := time.Now()

	for i := 0; i < N; i++ {
		// Writes go into a user-space buffer (fewer syscalls)
		if _, err := w.WriteString(fmt.Sprintf("line %d\n", i)); err != nil {
			panic(err)
		}
	}

	// Actually push buffered bytes down to the OS
	if err := w.Flush(); err != nil {
		panic(err)
	}

	return time.Since(start)
}

func main() {
	t1 := unbufferedWrite()
	t2 := bufferedWrite()

	fmt.Printf("Unbuffered: %v\n", t1)
	fmt.Printf("Buffered:   %v\n", t2)
}
