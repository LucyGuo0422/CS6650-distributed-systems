package main

import (
	"fmt"
	"sync"
	"time"
)

type SafeMap struct {
	mu sync.RWMutex
	m  map[int]int
}

func NewSafeMap() *SafeMap {
	return &SafeMap{
		m: make(map[int]int),
	}
}

// Write: lock before touching the map, unlock right after.
func (s *SafeMap) Set(key, val int) {
	s.mu.Lock()
    s.m[key] = val
    s.mu.Unlock()
}

// Length: also a read, so lock it too.
func (s *SafeMap) Len() int {
    s.mu.RLock()
    n := len(s.m)
    s.mu.RUnlock()
    return n
}

func main() {
	sm := NewSafeMap()

	start := time.Now()

	var wg sync.WaitGroup
	wg.Add(50)

	for g := 0; g < 50; g++ {
		g := g // capture loop variable safely
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				key := g*1000 + i
				sm.Set(key, i)
			}
		}()
	}

	wg.Wait()

	elapsed := time.Since(start)

	fmt.Printf("len(map) = %d\n", sm.Len())
	fmt.Printf("total time = %s\n", elapsed)
}
