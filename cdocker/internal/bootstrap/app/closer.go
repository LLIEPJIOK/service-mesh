package app

import (
	"context"
	"log/slog"

	"github.com/LLIEPJIOK/service-mesh/cdocker/internal/config"
)

func (a *App) closer(
	ctx context.Context,
	cfg *config.Config,
	stoppedChan <-chan struct{},
) error {
	<-ctx.Done()

	slog.Info("Stopping application")

	timeoutCtx, cancel := context.WithTimeout(context.Background(), cfg.App.TerminateTimeout)
	defer cancel()

	select {
	case <-timeoutCtx.Done():
		slog.Error("Timed out waiting for application to shut down")

		return NewErrStopApp("shutdown timeout")

	case <-stoppedChan:
		slog.Info("Application stopped successfully")

		return nil
	}
}
