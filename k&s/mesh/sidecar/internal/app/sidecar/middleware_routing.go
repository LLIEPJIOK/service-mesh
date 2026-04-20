package sidecar

import (
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/LLIEPJIOK/sidecar/internal/adapters/discovery"
	"github.com/LLIEPJIOK/sidecar/internal/adapters/proxy"
	"github.com/LLIEPJIOK/sidecar/internal/domain"
)

type routingMiddleware struct {
	cache              *discovery.ServiceCache
	appTargetAddr      string
	inboundMTLSPort    int
	loadBalancerPolicy string

	mu              sync.Mutex
	roundRobinState map[string]int
	rnd             *rand.Rand
}

func newRoutingMiddleware(
	cache *discovery.ServiceCache,
	appTargetAddr string,
	inboundMTLSPort int,
	loadBalancerPolicy string,
) *routingMiddleware {
	return &routingMiddleware{
		cache:              cache,
		appTargetAddr:      appTargetAddr,
		inboundMTLSPort:    inboundMTLSPort,
		loadBalancerPolicy: loadBalancerPolicy,
		roundRobinState:    make(map[string]int),
		rnd:                rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (m *routingMiddleware) Handle(ctx *domain.ConnContext, next domain.NextFunc) error {
	listener := ctx.GetString(domain.MetadataListener)
	switch listener {
	case string(proxy.ProfileInboundPlain), string(proxy.ProfileInboundMTLS):
		ctx.Set(domain.MetadataTargetAddr, m.appTargetAddr)
		ctx.Set(domain.MetadataService, "local-app")
		ctx.Set(domain.MetadataInMesh, false)
		ctx.Set(domain.MetadataServerName, "")
		ctx.Set(domain.MetadataBreakerKey, "")
		return next(ctx)
	default:
		return m.routeOutbound(ctx, next)
	}
}

func (m *routingMiddleware) routeOutbound(ctx *domain.ConnContext, next domain.NextFunc) error {
	endpoints := m.cache.GetEndpoints(ctx.OriginalDst)
	if len(endpoints) == 0 {
		ctx.Set(domain.MetadataTargetAddr, ctx.OriginalDst)
		ctx.Set(domain.MetadataService, "external")
		ctx.Set(domain.MetadataInMesh, false)
		ctx.Set(domain.MetadataServerName, "")
		ctx.Set(domain.MetadataBreakerKey, "")
		return next(ctx)
	}

	selected := m.selectEndpoint(ctx.OriginalDst, endpoints)
	targetAddr := net.JoinHostPort(selected.IP, strconv.Itoa(m.inboundMTLSPort))

	ctx.Set(domain.MetadataTargetAddr, targetAddr)
	ctx.Set(domain.MetadataService, selected.ServiceName)
	ctx.Set(domain.MetadataInMesh, true)
	ctx.Set(domain.MetadataServerName, selected.ServiceName)
	ctx.Set(domain.MetadataBreakerKey, targetAddr)

	return next(ctx)
}

func (m *routingMiddleware) selectEndpoint(key string, endpoints []domain.Endpoint) domain.Endpoint {
	if m.loadBalancerPolicy == "random" {
		m.mu.Lock()
		defer m.mu.Unlock()
		return endpoints[m.rnd.Intn(len(endpoints))]
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	idx := m.roundRobinState[key]
	selected := endpoints[idx%len(endpoints)]
	m.roundRobinState[key] = (idx + 1) % len(endpoints)

	return selected
}
