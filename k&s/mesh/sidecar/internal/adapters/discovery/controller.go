package discovery

import (
	"context"
	"fmt"
	"net"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"

	"github.com/LLIEPJIOK/sidecar/internal/domain"
)

type Controller struct {
	clientset kubernetes.Interface
	namespace string
	cache     *ServiceCache
}

type serviceMeta struct {
	clusterKey   string
	serviceLabel string
}

func NewController(clientset kubernetes.Interface, namespace string, cache *ServiceCache) *Controller {
	return &Controller{
		clientset: clientset,
		namespace: namespace,
		cache:     cache,
	}
}

func (c *Controller) InitialSync(ctx context.Context) error {
	return c.relist(ctx)
}

func (c *Controller) Run(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err := c.watchLoop(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			if relistErr := c.relist(ctx); relistErr != nil {
				return relistErr
			}
			continue
		}
	}
}

func (c *Controller) watchLoop(ctx context.Context) error {
	serviceWatch, err := c.clientset.CoreV1().Services(c.namespace).Watch(ctx, metav1.ListOptions{})
	if err != nil {
		return domain.Wrap(domain.ErrorKindDiscovery, fmt.Errorf("watch services: %w", err))
	}
	defer serviceWatch.Stop()

	sliceWatch, err := c.clientset.DiscoveryV1().EndpointSlices(c.namespace).Watch(ctx, metav1.ListOptions{})
	if err != nil {
		return domain.Wrap(domain.ErrorKindDiscovery, fmt.Errorf("watch endpointslices: %w", err))
	}
	defer sliceWatch.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-serviceWatch.ResultChan():
			if !ok {
				return domain.Wrap(domain.ErrorKindDiscovery, fmt.Errorf("service watch channel closed"))
			}

			if watchEventError(event) != nil {
				return watchEventError(event)
			}

			if err := c.relist(ctx); err != nil {
				return err
			}
		case event, ok := <-sliceWatch.ResultChan():
			if !ok {
				return domain.Wrap(domain.ErrorKindDiscovery, fmt.Errorf("endpointslice watch channel closed"))
			}

			if watchEventError(event) != nil {
				return watchEventError(event)
			}

			if err := c.relist(ctx); err != nil {
				return err
			}
		}
	}
}

func watchEventError(event watch.Event) error {
	if event.Type != watch.Error {
		return nil
	}

	return domain.Wrap(domain.ErrorKindDiscovery, fmt.Errorf("watch stream returned error event"))
}

func (c *Controller) relist(ctx context.Context) error {
	services, err := c.clientset.CoreV1().Services(c.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return domain.Wrap(domain.ErrorKindDiscovery, fmt.Errorf("list services: %w", err))
	}

	serviceMap := make(map[string]serviceMeta)
	for _, service := range services.Items {
		if service.Spec.ClusterIP == "" || service.Spec.ClusterIP == corev1.ClusterIPNone {
			continue
		}

		for _, servicePort := range service.Spec.Ports {
			serviceKey := buildServiceKey(service.Name, int(servicePort.Port))
			clusterKey := net.JoinHostPort(service.Spec.ClusterIP, strconv.Itoa(int(servicePort.Port)))

			serviceMap[serviceKey] = serviceMeta{
				clusterKey:   clusterKey,
				serviceLabel: buildServiceFQDN(service.Name, service.Namespace),
			}
		}
	}

	slices, err := c.clientset.DiscoveryV1().EndpointSlices(c.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return domain.Wrap(domain.ErrorKindDiscovery, fmt.Errorf("list endpointslices: %w", err))
	}

	aggregated := make(map[string][]domain.Endpoint)
	for _, slice := range slices.Items {
		serviceName, ok := slice.Labels[discoveryv1.LabelServiceName]
		if !ok || serviceName == "" {
			continue
		}

		for _, port := range slice.Ports {
			if port.Port == nil {
				continue
			}

			if port.Protocol != nil && *port.Protocol != corev1.ProtocolTCP {
				continue
			}

			serviceKey := buildServiceKey(serviceName, int(*port.Port))
			if _, exists := serviceMap[serviceKey]; !exists {
				continue
			}

			for _, endpoint := range slice.Endpoints {
				if endpoint.Conditions.Ready != nil && !*endpoint.Conditions.Ready {
					continue
				}

				for _, address := range endpoint.Addresses {
					aggregated[serviceKey] = append(aggregated[serviceKey], domain.Endpoint{
						IP:          address,
						Port:        int(*port.Port),
						ServiceName: buildServiceFQDN(serviceName, c.namespace),
					})
				}
			}
		}
	}

	cacheEntries := make([]CachedService, 0, len(serviceMap))
	for serviceKey, meta := range serviceMap {
		cacheEntries = append(cacheEntries, CachedService{
			ServiceKey:   serviceKey,
			ClusterKey:   meta.clusterKey,
			ServiceLabel: meta.serviceLabel,
			Endpoints:    dedupeEndpoints(aggregated[serviceKey]),
		})
	}

	c.cache.Replace(cacheEntries)
	return nil
}

func buildServiceKey(serviceName string, port int) string {
	return serviceName + ":" + strconv.Itoa(port)
}

func buildServiceFQDN(serviceName string, namespace string) string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", serviceName, namespace)
}

func dedupeEndpoints(endpoints []domain.Endpoint) []domain.Endpoint {
	if len(endpoints) < 2 {
		return endpoints
	}

	unique := make(map[string]domain.Endpoint, len(endpoints))
	for _, endpoint := range endpoints {
		key := net.JoinHostPort(endpoint.IP, strconv.Itoa(endpoint.Port))
		unique[key] = endpoint
	}

	result := make([]domain.Endpoint, 0, len(unique))
	for _, endpoint := range unique {
		result = append(result, endpoint)
	}

	return result
}
