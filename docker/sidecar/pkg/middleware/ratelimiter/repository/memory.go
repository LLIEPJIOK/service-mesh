package repository

import (
	"context"
	"sort"
	"sync"
	"time"
)

type InMemory struct {
	mu      *sync.RWMutex
	data    map[string][]int64
	expires map[string]time.Time
}

func NewInMemory() Repository {
	return &InMemory{
		mu:      &sync.RWMutex{},
		data:    make(map[string][]int64),
		expires: make(map[string]time.Time),
	}
}

func (m *InMemory) AddRecord(ctx context.Context, key string, tm time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isExpired(key) {
		delete(m.data, key)
		delete(m.expires, key)
	}

	unix := tm.UnixNano()
	slice := m.data[key]

	idx := sort.Search(len(slice), func(i int) bool { return slice[i] >= unix })
	slice = append(slice, 0)
	copy(slice[idx+1:], slice[idx:])

	slice[idx] = unix
	m.data[key] = slice

	return nil
}

func (m *InMemory) CountRecords(ctx context.Context, key string) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.isExpired(key) {
		return 0, nil
	}

	slice := m.data[key]

	return int64(len(slice)), nil
}

func (m *InMemory) RemoveOldRecords(ctx context.Context, key string, from, to time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isExpired(key) {
		delete(m.data, key)
		delete(m.expires, key)

		return nil
	}

	slice := m.data[key]
	min := from.UnixNano()
	max := to.UnixNano()

	start := sort.Search(len(slice), func(i int) bool { return slice[i] >= min })
	end := sort.Search(len(slice), func(i int) bool { return slice[i] > max })

	slice = append(slice[:start], slice[end:]...)
	m.data[key] = slice

	return nil
}

func (m *InMemory) ExpireKey(ctx context.Context, key string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.expires[key] = time.Now().Add(ttl)

	return nil
}

func (m *InMemory) isExpired(key string) bool {
	if exp, ok := m.expires[key]; ok {
		if time.Now().After(exp) {
			delete(m.data, key)
			delete(m.expires, key)

			return true
		}
	}

	return false
}
