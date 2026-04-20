package sidecar

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/LLIEPJIOK/sidecar/internal/adapters/discovery"
	"github.com/LLIEPJIOK/sidecar/internal/adapters/metrics"
	"github.com/LLIEPJIOK/sidecar/internal/adapters/proxy"
	"github.com/LLIEPJIOK/sidecar/internal/config"
	"github.com/LLIEPJIOK/sidecar/internal/domain"
)

type Service struct {
	cfg             config.Config
	discovery       *discovery.Controller
	cache           *discovery.ServiceCache
	metricsRecorder *metrics.Recorder
}

func New(cfg config.Config) (*Service, error) {
	metricsRecorder := metrics.NewRecorder()
	cache := discovery.NewServiceCache(metricsRecorder)

	clientset, err := discovery.NewClientset(cfg.KubeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("initialize discovery client: %w", err)
	}

	controller := discovery.NewController(clientset, cfg.Namespace, cache)

	return &Service{
		cfg:             cfg,
		discovery:       controller,
		cache:           cache,
		metricsRecorder: metricsRecorder,
	}, nil
}

func (s *Service) Run(ctx context.Context) error {
	tlsConfig, err := bootstrapTLSConfig(ctx, s.cfg)
	if err != nil {
		return fmt.Errorf("bootstrap tls config: %w", err)
	}

	if err := s.discovery.InitialSync(ctx); err != nil {
		return fmt.Errorf("initial discovery sync failed: %w", err)
	}

	listeners, err := s.buildListeners(tlsConfig)
	if err != nil {
		return err
	}
	defer closeListeners(listeners)

	forwarder := proxy.NewForwarder(tlsConfig, s.cfg.DialTimeout)
	chain := domain.Chain(
		newMetricsMiddleware(s.metricsRecorder),
		newTimeoutMiddleware(s.cfg.Timeout),
		newRetryMiddleware(
			s.cfg.RetryPolicy.Attempts,
			s.cfg.RetryPolicy.BackoffType,
			s.cfg.RetryPolicy.BaseInterval,
			s.metricsRecorder,
		),
		newRoutingMiddleware(
			s.cache,
			s.cfg.AppTargetAddr,
			s.cfg.InboundMTLSPort,
			s.cfg.LoadBalancerConfig.Algorithm,
		),
		newBreakerMiddleware(
			s.cfg.CircuitBreakerPolicy.FailureThreshold,
			s.cfg.CircuitBreakerPolicy.RecoveryTime,
			s.metricsRecorder,
		),
	)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, len(listeners)+2)
	go func() {
		if runErr := s.discovery.Run(runCtx); runErr != nil && !errors.Is(runErr, context.Canceled) {
			nonBlockingSend(errCh, fmt.Errorf("discovery watch loop failed: %w", runErr))
		}
	}()

	var metricsServer *http.Server
	if s.cfg.MonitoringEnabled {
		metricsServer = &http.Server{
			Addr:    fmt.Sprintf(":%d", s.cfg.MetricsPort),
			Handler: s.metricsRecorder.Handler(),
		}

		go func() {
			if runErr := metricsServer.ListenAndServe(); runErr != nil && !errors.Is(runErr, http.ErrServerClosed) {
				nonBlockingSend(errCh, fmt.Errorf("metrics server failed: %w", runErr))
			}
		}()
	}

	var acceptWG sync.WaitGroup
	var connectionWG sync.WaitGroup
	for _, listener := range listeners {
		acceptWG.Add(1)
		go s.runListener(runCtx, &acceptWG, &connectionWG, listener, chain, forwarder.Handle)
	}

	var runErr error
	select {
	case <-ctx.Done():
		cancel()
		runErr = ctx.Err()
	case runErr = <-errCh:
		cancel()
	}

	closeListeners(listeners)

	if metricsServer != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
		if err := metricsServer.Shutdown(shutdownCtx); err != nil && runErr == nil {
			runErr = fmt.Errorf("shutdown metrics server: %w", err)
		}
		shutdownCancel()
	}

	waitForAcceptLoops(&acceptWG, s.cfg.ShutdownTimeout)
	if err := waitForConnections(&connectionWG, s.cfg.ShutdownTimeout); err != nil && runErr == nil {
		runErr = err
	}

	return runErr
}

func (s *Service) buildListeners(tlsConfig *tls.Config) ([]*proxy.TransparentListener, error) {
	inboundPlain, err := proxy.NewTCPListener(
		fmt.Sprintf(":%d", s.cfg.InboundPlainPort),
		proxy.ProfileInboundPlain,
	)
	if err != nil {
		return nil, fmt.Errorf("create inbound plain listener: %w", err)
	}

	outbound, err := proxy.NewTCPListener(
		fmt.Sprintf(":%d", s.cfg.OutboundPort),
		proxy.ProfileOutbound,
	)
	if err != nil {
		_ = inboundPlain.Close()
		return nil, fmt.Errorf("create outbound listener: %w", err)
	}

	inboundMTLSNetListener, err := tls.Listen(
		"tcp",
		fmt.Sprintf(":%d", s.cfg.InboundMTLSPort),
		tlsConfig,
	)
	if err != nil {
		_ = outbound.Close()
		_ = inboundPlain.Close()
		return nil, fmt.Errorf("create inbound mtls listener: %w", err)
	}

	inboundMTLS := proxy.NewFromListener(proxy.ProfileInboundMTLS, inboundMTLSNetListener)

	return []*proxy.TransparentListener{inboundPlain, outbound, inboundMTLS}, nil
}

func (s *Service) runListener(
	ctx context.Context,
	acceptWG *sync.WaitGroup,
	connectionWG *sync.WaitGroup,
	listener *proxy.TransparentListener,
	chain domain.Handler,
	terminal domain.NextFunc,
) {
	defer acceptWG.Done()

	for {
		connCtx, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}

			slog.Warn("listener accept failed", slog.String("listener", string(listener.Profile())), slog.Any("error", err))
			continue
		}

		connectionWG.Add(1)
		go func(connectionCtx *domain.ConnContext) {
			defer connectionWG.Done()
			defer connectionCtx.ClientConn.Close()

			if handleErr := chain.Handle(connectionCtx, terminal); handleErr != nil && !errors.Is(handleErr, context.Canceled) {
				slog.Debug("connection handling finished with error", slog.Any("error", handleErr))
			}
		}(connCtx)
	}
}

func waitForAcceptLoops(acceptWG *sync.WaitGroup, timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		acceptWG.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(timeout):
	}
}

func waitForConnections(connectionWG *sync.WaitGroup, timeout time.Duration) error {
	done := make(chan struct{})
	go func() {
		connectionWG.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("graceful shutdown timed out while draining active connections")
	}
}

func closeListeners(listeners []*proxy.TransparentListener) {
	for _, listener := range listeners {
		_ = listener.Close()
	}
}

func nonBlockingSend(errCh chan<- error, err error) {
	select {
	case errCh <- err:
	default:
	}
}
