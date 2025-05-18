package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/LLIEPJIOK/sidecar/internal/app/http/sidecar"
	"github.com/LLIEPJIOK/sidecar/pkg/middleware"
	"github.com/LLIEPJIOK/sidecar/pkg/middleware/ratelimiter"
	raterepository "github.com/LLIEPJIOK/sidecar/pkg/middleware/ratelimiter/repository"
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

	sc, err := sidecar.New(a.cfg)
	if err != nil {
		slog.Error("failed to create proxy", slog.Any("error", err))

		return
	}

	mux := http.NewServeMux()
	sc.RegisterRoutes(mux)

	repo := raterepository.NewInMemory()
	rateLimiter := ratelimiter.NewSlidingWindow(repo, &a.cfg.RateLimiter)

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", a.cfg.SideCar.Port),
		Handler:           middleware.Wrap(mux, rateLimiter),
		ReadTimeout:       a.cfg.SideCar.ReadTimeout,
		ReadHeaderTimeout: a.cfg.SideCar.ReadHeaderTimeout,
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
