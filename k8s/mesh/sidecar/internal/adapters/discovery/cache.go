package discovery

import (
	"sync"

	"github.com/LLIEPJIOK/sidecar/internal/domain"
)

type EndpointsObserver interface {
	SetEndpointsReady(service string, ready int)
}

type CachedService struct {
	ServiceKey   string
	ClusterKey   string
	ServiceLabel string
	Endpoints    []domain.Endpoint
}

type ServiceCache struct {
	mu       sync.RWMutex
	byKey    map[string][]domain.Endpoint
	observer EndpointsObserver
}

func NewServiceCache(observer EndpointsObserver) *ServiceCache {
	return &ServiceCache{
		byKey:    make(map[string][]domain.Endpoint),
		observer: observer,
	}
}

func (c *ServiceCache) GetEndpoints(key string) []domain.Endpoint {
	c.mu.RLock()
	defer c.mu.RUnlock()

	endpoints, ok := c.byKey[key]
	if !ok {
		return nil
	}

	cloned := make([]domain.Endpoint, len(endpoints))
	copy(cloned, endpoints)
	return cloned
}

func (c *ServiceCache) Replace(services []CachedService) {
	next := make(map[string][]domain.Endpoint)

	for _, service := range services {
		cloned := make([]domain.Endpoint, len(service.Endpoints))
		copy(cloned, service.Endpoints)

		next[service.ServiceKey] = cloned
		if service.ClusterKey != "" {
			next[service.ClusterKey] = cloned
		}

		if c.observer != nil {
			c.observer.SetEndpointsReady(service.ServiceLabel, len(cloned))
		}
	}

	c.mu.Lock()
	c.byKey = next
	c.mu.Unlock()
}
