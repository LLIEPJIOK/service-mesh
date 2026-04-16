package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/LLIEPJIOK/sidecar/internal/bootstrap/app"
	"github.com/LLIEPJIOK/sidecar/internal/config"
)

const (
	OkCode = iota
	ErrorConfigLoad
	ErrorCreateApp
	ErrorRunApp
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("Error loading config", slog.Any("error", err))
		os.Exit(ErrorConfigLoad)
	}

	application := app.New(cfg)

	if runerr := application.Run(ctx); runerr != nil {
		slog.Error("Error running application", slog.Any("error", runerr))

		os.Exit(ErrorRunApp)
	}

	os.Exit(OkCode)
}
