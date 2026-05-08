package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/LLIEPJIOK/sidecar/internal/app/sidecar"
	"github.com/LLIEPJIOK/sidecar/internal/config"
)

const exitCodeError = 1

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.LoadFromEnv()
	if err != nil {
		slog.Error("failed to load config", slog.Any("error", err))
		os.Exit(exitCodeError)
	}

	service, err := sidecar.New(cfg)
	if err != nil {
		slog.Error("failed to create sidecar service", slog.Any("error", err))
		os.Exit(exitCodeError)
	}

	if err := service.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("sidecar stopped with error", slog.Any("error", err))
		os.Exit(exitCodeError)
	}
}
