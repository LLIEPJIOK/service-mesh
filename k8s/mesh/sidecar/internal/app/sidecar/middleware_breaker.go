package sidecar

import (
	"fmt"
	"log/slog"
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
			slog.Warn("circuit breaker rejected request", slog.String("key", key), slog.Int("state", entry.state))
			return domain.Wrap(domain.ErrorKindBreakerOpen, fmt.Errorf("circuit breaker is open for %s", key))
		}

		slog.Info("circuit breaker transition", slog.String("key", key), slog.Int("from", breakerStateOpen), slog.Int("to", breakerStateHalf))
		entry.state = breakerStateHalf
		entry.trialInFlight = false
		m.recorder.SetCircuitBreakerState(service, breakerStateHalf)
		fallthrough
	case breakerStateHalf:
		if entry.trialInFlight {
			slog.Warn("circuit breaker rejected parallel half-open request", slog.String("key", key))
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
	previousState := entry.state
	entry.state = breakerStateClosed
	entry.failures = 0
	entry.trialInFlight = false
	m.recorder.SetCircuitBreakerState(service, breakerStateClosed)
	if previousState != breakerStateClosed {
		slog.Info("circuit breaker transition", slog.String("key", key), slog.Int("from", previousState), slog.Int("to", breakerStateClosed))
	}
}

func (m *breakerMiddleware) recordFailure(key string, service string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry := m.getOrCreateEntry(key, service)
	previousState := entry.state
	now := time.Now()

	switch entry.state {
	case breakerStateHalf:
		entry.state = breakerStateOpen
		entry.openedAt = now
		entry.failures = m.failureThreshold
		entry.trialInFlight = false
		m.recorder.SetCircuitBreakerState(service, breakerStateOpen)
		slog.Info("circuit breaker transition", slog.String("key", key), slog.Int("from", previousState), slog.Int("to", breakerStateOpen), slog.Uint64("failures", uint64(entry.failures)))
		return
	case breakerStateOpen:
		entry.openedAt = now
		m.recorder.SetCircuitBreakerState(service, breakerStateOpen)
		slog.Debug("circuit breaker remains open", slog.String("key", key), slog.Uint64("failures", uint64(entry.failures)))
		return
	}

	entry.failures++
	if entry.failures >= m.failureThreshold {
		entry.state = breakerStateOpen
		entry.openedAt = now
		entry.trialInFlight = false
		m.recorder.SetCircuitBreakerState(service, breakerStateOpen)
		slog.Info("circuit breaker transition", slog.String("key", key), slog.Int("from", previousState), slog.Int("to", breakerStateOpen), slog.Uint64("failures", uint64(entry.failures)))
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
