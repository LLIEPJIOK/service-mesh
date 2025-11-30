package prober

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/LLIEPJIOK/sidecar/internal/config"
)

type ProbeStatus string

const (
	ProbeStatusHealthy   ProbeStatus = "healthy"
	ProbeStatusUnhealthy ProbeStatus = "unhealthy"
	ProbeStatusUnknown   ProbeStatus = "unknown"
)

type ProbeReport struct {
	ContainerName string      `json:"container_name"`
	ProbeName     string      `json:"probe_name"`
	Status        ProbeStatus `json:"status"`
}

type Prober struct {
	config    *config.Probes
	client    *http.Client
	mu        sync.RWMutex
	isRunning bool
	stopCh    chan struct{}
}

func New(cfg *config.Probes) *Prober {
	return &Prober{
		config: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		stopCh: make(chan struct{}),
	}
}

func (p *Prober) Start(ctx context.Context) {
	p.mu.Lock()
	if p.isRunning {
		p.mu.Unlock()
		return
	}

	p.isRunning = true
	p.mu.Unlock()

	readinessTicker := time.NewTicker(p.config.ReadinessPeriod)
	defer readinessTicker.Stop()

	livenessTicker := time.NewTicker(p.config.LivenessPeriod)
	defer livenessTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.stopCh:
			return
		case <-readinessTicker.C:
			if !p.config.ReadinessEnabled {
				readinessTicker.Stop()
				continue
			}

			status, err := p.executeProbes(ctx, p.config.ReadinessURL)
			if err != nil {
				slog.Error("Failed to execute readiness probe", slog.Any("error", err))
			}

			report := ProbeReport{
				ContainerName: p.config.ContainerName,
				ProbeName:     "readiness",
				Status:        status,
			}

			if err := p.sendReport(ctx, report); err != nil {
				slog.Error("Failed to send readiness probe report", slog.Any("error", err))
			}

		case <-livenessTicker.C:
			if !p.config.LivenessEnabled {
				livenessTicker.Stop()
				continue
			}

			status, err := p.executeProbes(ctx, p.config.LivenessURL)
			if err != nil {
				slog.Error("Failed to execute liveness probe", slog.Any("error", err))
			}

			report := ProbeReport{
				ContainerName: p.config.ContainerName,
				ProbeName:     "liveness",
				Status:        status,
			}

			if err := p.sendReport(ctx, report); err != nil {
				slog.Error("Failed to send liveness probe report", slog.Any("error", err))
			}
		}
	}
}

func (p *Prober) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.isRunning {
		return
	}

	close(p.stopCh)
	p.isRunning = false
}

func (p *Prober) executeProbes(
	ctx context.Context,
	url string,
) (ProbeStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ProbeStatusUnknown, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return ProbeStatusUnhealthy, fmt.Errorf("probe request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return ProbeStatusHealthy, nil
	}

	return ProbeStatusUnhealthy, fmt.Errorf("unhealthy status code: %d", resp.StatusCode)
}

func (p *Prober) sendReport(ctx context.Context, report ProbeReport) error {
	if p.config.CDockerURL == "" {
		slog.Debug("CDocker URL not configured, skipping report")
		return nil
	}

	data, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}

	url := fmt.Sprintf("%s/probe-report", p.config.CDockerURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send report: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}
