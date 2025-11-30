package cdocker

import (
	"context"
	"log/slog"
	"maps"
	"sync"
	"time"

	"github.com/LLIEPJIOK/service-mesh/cdocker/internal/domain"
	"github.com/LLIEPJIOK/service-mesh/cdocker/internal/infra/docker"
)

const (
	livenessThreshold  = 3
	readinessThreshold = 3
	probeTimeout       = 3 * time.Minute
	checkInterval      = 30 * time.Second
)

type HealthMonitor struct {
	containers map[string]*domain.ContainerInfo
	states     map[string]*domain.HealthState
	mu         sync.RWMutex
	docker     *docker.Client
	stopCh     chan struct{}
}

func NewHealthMonitor(
	containers map[string]*domain.ContainerInfo,
	dockerClient *docker.Client,
) *HealthMonitor {
	return &HealthMonitor{
		containers: containers,
		states:     make(map[string]*domain.HealthState),
		docker:     dockerClient,
		stopCh:     make(chan struct{}),
	}
}

func (m *HealthMonitor) Start(ctx context.Context) {
	slog.Info("Starting health monitor")

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.checkStates(ctx)
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		}
	}
}

func (m *HealthMonitor) Stop() {
	close(m.stopCh)
}

func (m *HealthMonitor) HandleProbeReport(report domain.ProbeReport) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, exists := m.states[report.ContainerName]
	if !exists {
		state = &domain.HealthState{ContainerName: report.ContainerName}
		m.states[report.ContainerName] = state
	}

	now := time.Now().Unix()

	if report.ProbeName == "liveness" {
		state.LastLivenessTime = now
		if report.Status == domain.ProbeStatusHealthy {
			m.containers[report.ContainerName].Status = "running"
			state.LivenessFails = 0
		} else {
			m.containers[report.ContainerName].Status = "failed"
			state.LivenessFails++
		}
	}

	if report.ProbeName == "readiness" {
		state.LastReadinessTime = now
		if report.Status == domain.ProbeStatusHealthy {
			m.containers[report.ContainerName].Status = "running"
			state.ReadinessFails = 0
		} else {
			if m.containers[report.ContainerName].Status != "failed" {
				m.containers[report.ContainerName].Status = "not ready"
			}

			state.ReadinessFails++
		}
	}
}

func (m *HealthMonitor) checkStates(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().Unix()
	timeout := int64(probeTimeout.Seconds())

	for containerName, state := range m.states {
		if state.LastLivenessTime > 0 && (now-state.LastLivenessTime) > timeout {
			slog.Warn("Liveness probe timeout", slog.String("container", containerName))
			state.LivenessFails = livenessThreshold
		}
		if state.LastReadinessTime > 0 && (now-state.LastReadinessTime) > timeout {
			slog.Warn("Readiness probe timeout", slog.String("container", containerName))
			state.ReadinessFails = readinessThreshold
		}

		if state.LivenessFails >= livenessThreshold {
			m.restartContainer(ctx, containerName)
			state.LivenessFails = 0
		} else if state.ReadinessFails >= readinessThreshold {
			m.restartContainer(ctx, containerName)
			state.ReadinessFails = 0
		}
	}
}

func (m *HealthMonitor) restartContainer(ctx context.Context, containerName string) {
	m.containers[containerName].Restarts++

	if err := m.docker.RestartContainer(ctx, containerName); err != nil {
		slog.Error("Failed to restart container",
			slog.String("container", containerName),
			slog.Any("error", err),
		)
	}
}

func (m *HealthMonitor) GetAllHealthStates() map[string]*domain.HealthState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*domain.HealthState, len(m.states))
	maps.Copy(result, m.states)

	return result
}
