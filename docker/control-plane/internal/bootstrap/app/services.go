package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"

	mesh "github.com/LLIEPJIOK/control-plane/internal/app/http/plane"
)

type runService = func(ctx context.Context, stop context.CancelFunc, wg *sync.WaitGroup)

func (a *App) services() []runService {
	return []runService{
		a.runMesh,
	}
}

func (a *App) runMesh(ctx context.Context, stop context.CancelFunc, wg *sync.WaitGroup) {
	defer wg.Done()
	defer stop()
	defer slog.Info("proxy stopped")

	m, err := mesh.New(&a.cfg.Plane)
	if err != nil {
		slog.Error("failed to create proxy", slog.Any("error", err))

		return
	}

	mux := http.NewServeMux()
	m.RegisterRoutes(mux)

	httpServer := &http.Server{
		Addr:              a.cfg.Plane.URL,
		Handler:           mux,
		ReadTimeout:       a.cfg.Plane.ReadTimeout,
		ReadHeaderTimeout: a.cfg.Plane.ReadHeaderTimeout,
	}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("failed to start proxy server", slog.Any("error", err))

			stop()
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.App.ShutdownTimeout)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("failed to shutdown scrapper server", slog.Any("error", err))
	}
}
