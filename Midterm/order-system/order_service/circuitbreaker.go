package main

import (
	"errors"
	"sync"
	"time"
)

type State int

const (
	StateClosed   State = iota // normal operation
	StateOpen                  // failing fast
	StateHalfOpen              // testing if service recovered
)

var ErrCircuitOpen = errors.New("circuit breaker is open")

type CircuitBreaker struct {
	mu sync.Mutex

	state State

	failureThreshold int
	failureCount     int

	successThreshold int // successes needed in half-open to close
	successCount     int

	timeout   time.Duration // how long to stay open before trying half-open
	openedAt  time.Time
}

func NewCircuitBreaker(failureThreshold, successThreshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:            StateClosed,
		failureThreshold: failureThreshold,
		successThreshold: successThreshold,
		timeout:          timeout,
	}
}

func (cb *CircuitBreaker) Call(fn func() error) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateOpen:
		if time.Since(cb.openedAt) >= cb.timeout {
			cb.state = StateHalfOpen
			cb.successCount = 0
		} else {
			return ErrCircuitOpen
		}
	}

	err := fn()

	if err != nil {
		cb.onFailure()
	} else {
		cb.onSuccess()
	}
	return err
}

func (cb *CircuitBreaker) onFailure() {
	cb.successCount = 0
	switch cb.state {
	case StateClosed:
		cb.failureCount++
		if cb.failureCount >= cb.failureThreshold {
			cb.trip()
		}
	case StateHalfOpen:
		cb.trip()
	}
}

func (cb *CircuitBreaker) onSuccess() {
	cb.failureCount = 0
	if cb.state == StateHalfOpen {
		cb.successCount++
		if cb.successCount >= cb.successThreshold {
			cb.state = StateClosed
		}
	}
}

func (cb *CircuitBreaker) trip() {
	cb.state = StateOpen
	cb.openedAt = time.Now()
}

func (cb *CircuitBreaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}
