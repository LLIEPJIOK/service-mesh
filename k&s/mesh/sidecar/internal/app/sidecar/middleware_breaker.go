package sidecar

import (
	"fmt"
	"sync"
	"time"

	"github.com/LLIEPJIOK/sidecar/internal/adapters/metrics"
	"github.com/LLIEPJIOK/sidecar/internal/domain"
)

const (
	breakerStateClosed = 0
	breakerStateOpen   = 1
	breakerStateHalf   = 2
)

type breakerEntry struct {
	state         int
	failures      uint32
	openedAt      time.Time
	trialInFlight bool
}

type breakerMiddleware struct {
	failureThreshold uint32
	recoveryTime     time.Duration
	recorder         *metrics.Recorder

	mu      sync.Mutex
	entries map[string]*breakerEntry
}

func newBreakerMiddleware(failureThreshold uint32, recoveryTime time.Duration, recorder *metrics.Recorder) *breakerMiddleware {
	return &breakerMiddleware{
		failureThreshold: failureThreshold,
		recoveryTime:     recoveryTime,
		recorder:         recorder,
		entries:          make(map[string]*breakerEntry),
	}
}

func (m *breakerMiddleware) Handle(ctx *domain.ConnContext, next domain.NextFunc) error {
	key := ctx.GetString(domain.MetadataBreakerKey)
	if key == "" {
		return next(ctx)
	}

	service := ctx.GetString(domain.MetadataService)
	if err := m.allow(key, service); err != nil {
		return err
	}

	err := next(ctx)
	if err == nil {
		m.recordSuccess(key, service)
		return nil
	}

	if domain.IsEstablishError(err) {
		m.recordFailure(key, service)
	}

	return err
}

func (m *breakerMiddleware) allow(key string, service string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry := m.getOrCreateEntry(key, service)
	now := time.Now()

	switch entry.state {
	case breakerStateOpen:
		if now.Sub(entry.openedAt) < m.recoveryTime {
			return domain.Wrap(domain.ErrorKindBreakerOpen, fmt.Errorf("circuit breaker is open for %s", key))
		}

		entry.state = breakerStateHalf
		entry.trialInFlight = false
		m.recorder.SetCircuitBreakerState(service, breakerStateHalf)
		fallthrough
	case breakerStateHalf:
		if entry.trialInFlight {
			return domain.Wrap(domain.ErrorKindBreakerOpen, fmt.Errorf("circuit breaker is half-open for %s", key))
		}

		entry.trialInFlight = true
	}

	return nil
}

func (m *breakerMiddleware) recordSuccess(key string, service string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry := m.getOrCreateEntry(key, service)
	entry.state = breakerStateClosed
	entry.failures = 0
	entry.trialInFlight = false
	m.recorder.SetCircuitBreakerState(service, breakerStateClosed)
}

func (m *breakerMiddleware) recordFailure(key string, service string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry := m.getOrCreateEntry(key, service)
	now := time.Now()

	switch entry.state {
	case breakerStateHalf:
		entry.state = breakerStateOpen
		entry.openedAt = now
		entry.failures = m.failureThreshold
		entry.trialInFlight = false
		m.recorder.SetCircuitBreakerState(service, breakerStateOpen)
		return
	case breakerStateOpen:
		entry.openedAt = now
		m.recorder.SetCircuitBreakerState(service, breakerStateOpen)
		return
	}

	entry.failures++
	if entry.failures >= m.failureThreshold {
		entry.state = breakerStateOpen
		entry.openedAt = now
		entry.trialInFlight = false
		m.recorder.SetCircuitBreakerState(service, breakerStateOpen)
		return
	}

	m.recorder.SetCircuitBreakerState(service, breakerStateClosed)
}

func (m *breakerMiddleware) getOrCreateEntry(key string, service string) *breakerEntry {
	entry, exists := m.entries[key]
	if exists {
		return entry
	}

	entry = &breakerEntry{state: breakerStateClosed}
	m.entries[key] = entry
	m.recorder.SetCircuitBreakerState(service, breakerStateClosed)
	return entry
}
