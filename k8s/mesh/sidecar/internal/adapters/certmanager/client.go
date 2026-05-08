package certmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	signURL    string
	httpClient *http.Client
}

type signRequest struct {
	CSR   string `json:"csr"`
	Token string `json:"token"`
}

type signResponse struct {
	Certificate string `json:"certificate"`
	CA          string `json:"ca"`
}

func NewClient(signURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}

	return &Client{
		signURL:    signURL,
		httpClient: httpClient,
	}
}

func (c *Client) Sign(ctx context.Context, csrPEM []byte, token string) ([]byte, []byte, error) {
	payload, err := json.Marshal(signRequest{
		CSR:   string(csrPEM),
		Token: token,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("marshal sign request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.signURL, bytes.NewReader(payload))
	if err != nil {
		return nil, nil, fmt.Errorf("create sign request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("call cert-manager sign endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, nil, fmt.Errorf("read cert-manager response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}

		return nil, nil, fmt.Errorf("cert-manager sign failed with status %d: %s", resp.StatusCode, message)
	}

	var parsed signResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, nil, fmt.Errorf("decode cert-manager response: %w", err)
	}

	if strings.TrimSpace(parsed.Certificate) == "" || strings.TrimSpace(parsed.CA) == "" {
		return nil, nil, fmt.Errorf("cert-manager response does not contain certificate and ca")
	}

	return []byte(parsed.Certificate), []byte(parsed.CA), nil
}
