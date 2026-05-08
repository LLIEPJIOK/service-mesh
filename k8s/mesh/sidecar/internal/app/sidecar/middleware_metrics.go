package sidecar

import (
	"time"

	"github.com/LLIEPJIOK/sidecar/internal/adapters/metrics"
	"github.com/LLIEPJIOK/sidecar/internal/domain"
)

type metricsMiddleware struct {
	recorder *metrics.Recorder
}

func newMetricsMiddleware(recorder *metrics.Recorder) *metricsMiddleware {
	return &metricsMiddleware{recorder: recorder}
}

func (m *metricsMiddleware) Handle(ctx *domain.ConnContext, next domain.NextFunc) error {
	started := time.Now()
	err := next(ctx)

	service := ctx.GetString(domain.MetadataService)
	direction := ctx.GetString(domain.MetadataDirection)
	statusCode := "200"
	if err != nil {
		statusCode = "500"
	}

	m.recorder.ObserveRequest(service, statusCode, direction, time.Since(started))
	if err != nil {
		errorType := domain.NormalizeErrorType(err)
		ctx.Set(domain.MetadataErrorType, errorType)
		m.recorder.ObserveError(service, errorType)
	}

	ctx.Set(domain.MetadataStatusCode, statusCode)
	return err
}
