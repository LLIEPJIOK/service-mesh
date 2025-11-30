package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"

	"github.com/LLIEPJIOK/service-mesh/cdocker/internal/app/http/cdocker"
	"github.com/LLIEPJIOK/service-mesh/cdocker/internal/infra/docker"
)

type runService = func(ctx context.Context, stop context.CancelFunc, wg *sync.WaitGroup)

func (a *App) services() []runService {
	return []runService{
		a.runCDocker,
	}
}

func (a *App) runCDocker(ctx context.Context, stop context.CancelFunc, wg *sync.WaitGroup) {
	defer wg.Done()
	defer stop()
	defer slog.Info("cdocker stopped")

	// Create Docker client
	dockerClient, err := docker.NewClient()
	if err != nil {
		slog.Error("failed to create docker client", slog.Any("error", err))
		return
	}
	defer dockerClient.Close()

	m, err := cdocker.New(ctx, &a.cfg.CDocker, dockerClient)
	if err != nil {
		slog.Error("failed to create cdocker", slog.Any("error", err))
		return
	}

	mux := http.NewServeMux()
	m.RegisterRoutes(mux)

	httpServer := &http.Server{
		Addr:              a.cfg.CDocker.URL,
		Handler:           mux,
		ReadTimeout:       a.cfg.CDocker.ReadTimeout,
		ReadHeaderTimeout: a.cfg.CDocker.ReadHeaderTimeout,
	}

	go func() {
		slog.Info("Starting CDocker server", slog.String("addr", a.cfg.CDocker.URL))
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("failed to start cdocker server", slog.Any("error", err))

			stop()
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.App.ShutdownTimeout)
	defer cancel()

	if err := m.Stop(shutdownCtx); err != nil {
		slog.Error("failed to stop cdocker", slog.Any("error", err))
	}

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("failed to shutdown cdocker server", slog.Any("error", err))
	}
}
