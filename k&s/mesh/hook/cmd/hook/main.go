package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/LLIEPJIOK/service-mesh/hook/internal/app/injector"
	"github.com/LLIEPJIOK/service-mesh/hook/internal/config"
	transporthttp "github.com/LLIEPJIOK/service-mesh/hook/internal/transport/http"
)

func main() {
	logger := log.New(os.Stdout, "hook ", log.LstdFlags|log.LUTC)

	cfg, err := config.LoadFromEnv()
	if err != nil {
		logger.Fatalf("load config: %v", err)
	}

	service := injector.NewService(cfg, logger)
	handler := transporthttp.NewHandler(service, cfg.MaxRequestBytes, logger)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Printf("starting hook on %s", cfg.HTTPAddr)

		if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
			if serveErr := server.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
				logger.Fatalf("serve tls: %v", serveErr)
			}
			return
		}

		logger.Printf("running without TLS; this mode is intended only for local development")
		if serveErr := server.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			logger.Fatalf("serve: %v", serveErr)
		}
	}()

	<-ctx.Done()
	logger.Printf("shutdown requested")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Fatalf("shutdown: %v", err)
	}

	logger.Printf("server stopped")
}
