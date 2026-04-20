package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	cryptoadapter "github.com/LLIEPJIOK/service-mesh/certmanager/internal/adapters/crypto"
	kubeadapter "github.com/LLIEPJIOK/service-mesh/certmanager/internal/adapters/kube"
	appcertmanager "github.com/LLIEPJIOK/service-mesh/certmanager/internal/app/certmanager"
	"github.com/LLIEPJIOK/service-mesh/certmanager/internal/config"
	transporthttp "github.com/LLIEPJIOK/service-mesh/certmanager/internal/transport/http"
)

func main() {
	logger := log.New(os.Stdout, "certmanager ", log.LstdFlags|log.LUTC)

	cfg, err := config.LoadFromEnv()
	if err != nil {
		logger.Fatalf("load config: %v", err)
	}

	tokenReviewer, err := kubeadapter.NewTokenReviewer(cfg.KubeConfigPath)
	if err != nil {
		logger.Fatalf("build token reviewer: %v", err)
	}

	signer, err := cryptoadapter.NewSignerFromFiles(cfg.RootCACertFile, cfg.RootCAKeyFile, cfg.LeafTTL)
	if err != nil {
		logger.Fatalf("build signer: %v", err)
	}

	service := appcertmanager.NewService(tokenReviewer, signer)
	handler := transporthttp.NewHandler(service, cfg.MaxRequestBytes, cfg.RateLimitRPS, cfg.RateLimitBurst, logger)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Printf("starting certmanager on %s", cfg.HTTPAddr)
		if listenErr := server.ListenAndServe(); listenErr != nil && !errors.Is(listenErr, http.ErrServerClosed) {
			logger.Fatalf("serve: %v", listenErr)
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
