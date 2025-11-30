package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"counter/pkg/ws"
)

type MeshClient struct {
	*ws.SecureClient
}

type Config struct {
	ServiceName string        // Имя текущего сервиса
	TargetName  string        // Имя целевого сервиса
	TLS         *ws.TLSConfig // TLS конфигурация
}

func New(ctx context.Context, cfg Config) (*MeshClient, error) {
	address, container, err := getAddress(ctx, cfg.ServiceName, cfg.TargetName)
	if err != nil {
		return nil, fmt.Errorf("failed to get address: %w", err)
	}

	clientCfg := ws.DefaultSecureClientConfig(address)
	clientCfg.TLS = cfg.TLS

	// Устанавливаем ожидаемый ID партнёра, если не задан
	if clientCfg.TLS != nil && clientCfg.TLS.ExpectedPeerID == "" {
		clientCfg.TLS.ExpectedPeerID = container
	}

	secureClient := ws.NewSecureClient(clientCfg)

	return &MeshClient{
		SecureClient: secureClient,
	}, nil
}

func getAddress(
	ctx context.Context,
	serviceName, targetName string,
) (string, string, error) {
	resp, err := http.Get(
		fmt.Sprintf("http://%s-sidecar:8080/address?service=%s", serviceName, targetName),
	)
	if err != nil {
		return "", "", fmt.Errorf("failed to get address: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var address string

	if err := json.NewDecoder(resp.Body).Decode(&address); err != nil {
		return "", "", fmt.Errorf("failed to decode address response: %w", err)
	}

	container := address[: strings.Index(address, "-sidecar")]
	address = "ws://" + container + ":9090"

	return address, container, nil
}
