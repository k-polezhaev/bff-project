package resilience

import (
	"errors"
	"log/slog"
	"sync"
	"time"
)

type State int

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

type CircuitBreaker struct {
	mu            sync.Mutex
	state         State
	failureCount  int
	lastErrorTime time.Time
	threshold     int
	timeout       time.Duration
}

func NewCircuitBreaker(threshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:     StateClosed,
		threshold: threshold,
		timeout:   timeout,
	}
}

func (cb *CircuitBreaker) Execute(action func() (any, error)) (interface{}, error) {
	cb.mu.Lock()

	switch cb.state {
	case StateOpen:
		if time.Since(cb.lastErrorTime) > cb.timeout {
			cb.state = StateHalfOpen
		} else {
			cb.mu.Unlock()
			return nil, errors.New("circuit breaker is open")
		}
	case StateHalfOpen:
		cb.mu.Unlock()
		return nil, errors.New("circuit breaker is half-open (rate limited)")
	}

	cb.mu.Unlock()

	result, err := action()

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.failureCount++
		cb.lastErrorTime = time.Now()

		if cb.failureCount >= cb.threshold || cb.state == StateHalfOpen {
			cb.state = StateOpen
			slog.Warn("Circuit Breaker OPENED", "failures", cb.failureCount)
		}
		return nil, err
	}

	if cb.state == StateHalfOpen {
		slog.Info("Circuit Breaker RECOVERED")
	}
	cb.failureCount = 0
	cb.state = StateClosed

	return result, nil
}