package ws_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LLIEPJIOK/service-mesh/ws/pkg/ws"
)

type EchoRequest struct {
	Message string `json:"message"`
}

type EchoResponse struct {
	Message string `json:"message"`
	Echo    bool   `json:"echo"`
}

func setupTestServer(t *testing.T) (*ws.Server, *httptest.Server) {
	t.Helper()

	server := ws.NewServer(ws.DefaultServerConfig())

	server.Handle("echo", func(ctx context.Context, payload json.RawMessage) (any, error) {
		var req EchoRequest

		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}

		return EchoResponse{Message: req.Message, Echo: true}, nil
	})

	server.Handle("slow", func(ctx context.Context, payload json.RawMessage) (any, error) {
		time.Sleep(100 * time.Millisecond)
		return map[string]string{"status": "done"}, nil
	})

	server.Handle("error", func(ctx context.Context, payload json.RawMessage) (any, error) {
		return nil, ws.ErrServerError
	})

	ts := httptest.NewServer(server)

	return server, ts
}

func TestClientServer_BasicEcho(t *testing.T) {
	_, ts := setupTestServer(t)
	defer ts.Close()

	// Преобразуем HTTP URL в WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")

	cfg := ws.DefaultClientConfig(wsURL)
	client := ws.NewClient(cfg)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Close()

	var resp EchoResponse

	err := client.RequestTyped(ctx, "echo", EchoRequest{Message: "hello"}, &resp)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.Message != "hello" {
		t.Errorf("expected message 'hello', got '%s'", resp.Message)
	}

	if !resp.Echo {
		t.Error("expected echo to be true")
	}
}

func TestClientServer_ConcurrentRequests(t *testing.T) {
	_, ts := setupTestServer(t)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")

	cfg := ws.DefaultClientConfig(wsURL)
	client := ws.NewClient(cfg)

	ctx := context.Background()
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Close()

	const numRequests = 100
	var wg sync.WaitGroup
	errors := make(chan error, numRequests)

	for i := range numRequests {
		wg.Go(func() {
			var resp EchoResponse

			msg := EchoRequest{Message: strings.Repeat("a", i+1)}

			err := client.RequestTyped(ctx, "echo", msg, &resp)
			if err != nil {
				errors <- err
				return
			}

			if resp.Message != msg.Message {
				errors <- ws.ErrServerError
			}
		})
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent request error: %v", err)
	}
}

func TestClientServer_RouteNotFound(t *testing.T) {
	_, ts := setupTestServer(t)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")

	cfg := ws.DefaultClientConfig(wsURL)
	client := ws.NewClient(cfg)

	ctx := context.Background()
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Close()

	_, err := client.Request(ctx, "nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent route")
	}

	if !strings.Contains(err.Error(), "route not found") {
		t.Errorf("expected 'route not found' error, got: %v", err)
	}
}

func TestClientServer_HandlerError(t *testing.T) {
	_, ts := setupTestServer(t)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")

	cfg := ws.DefaultClientConfig(wsURL)
	client := ws.NewClient(cfg)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Close()

	_, err := client.Request(ctx, "error", nil)
	if err == nil {
		t.Fatal("expected error from handler")
	}
}

func TestClientServer_Timeout(t *testing.T) {
	server := ws.NewServer(ws.DefaultServerConfig())

	server.Handle("veryslow", func(ctx context.Context, payload json.RawMessage) (any, error) {
		time.Sleep(2 * time.Second)
		return nil, nil
	})

	ts := httptest.NewServer(server)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")

	cfg := ws.DefaultClientConfig(wsURL)
	cfg.RequestTimeout = 100 * time.Millisecond
	client := ws.NewClient(cfg)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Close()

	_, err := client.Request(ctx, "veryslow", nil)
	if err != ws.ErrRequestTimeout {
		t.Errorf("expected timeout error, got: %v", err)
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	server := ws.NewServer(ws.DefaultServerConfig())

	server.Handle("slow", func(ctx context.Context, payload json.RawMessage) (any, error) {
		time.Sleep(time.Second)
		return nil, nil
	})

	ts := httptest.NewServer(server)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")

	cfg := ws.DefaultClientConfig(wsURL)
	client := ws.NewClient(cfg)

	ctx := context.Background()
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Close()

	// Отменяем контекст
	cancelCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	_, err := client.Request(cancelCtx, "slow", nil)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestClient_Close(t *testing.T) {
	_, ts := setupTestServer(t)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")

	cfg := ws.DefaultClientConfig(wsURL)
	client := ws.NewClient(cfg)

	ctx := context.Background()
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	if err := client.Close(); err != nil {
		t.Errorf("close error: %v", err)
	}

	if !client.IsClosed() {
		t.Error("expected client to be closed")
	}

	_, err := client.Request(ctx, "echo", nil)
	if err != ws.ErrConnectionClosed {
		t.Errorf("expected connection closed error, got: %v", err)
	}
}

func TestServer_MultipleHandlers(t *testing.T) {
	server := ws.NewServer(ws.DefaultServerConfig())

	server.Handle("add", func(ctx context.Context, payload json.RawMessage) (any, error) {
		var nums []int
		if err := json.Unmarshal(payload, &nums); err != nil {
			return nil, err
		}
		sum := 0
		for _, n := range nums {
			sum += n
		}
		return map[string]int{"sum": sum}, nil
	})

	server.Handle("multiply", func(ctx context.Context, payload json.RawMessage) (any, error) {
		var nums []int
		if err := json.Unmarshal(payload, &nums); err != nil {
			return nil, err
		}
		product := 1
		for _, n := range nums {
			product *= n
		}
		return map[string]int{"product": product}, nil
	})

	ts := httptest.NewServer(server)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")
	cfg := ws.DefaultClientConfig(wsURL)
	client := ws.NewClient(cfg)
	ctx := context.Background()

	if err := client.Connect(ctx); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Close()

	var addResp map[string]int
	if err := client.RequestTyped(ctx, "add", []int{1, 2, 3, 4, 5}, &addResp); err != nil {
		t.Fatalf("add request failed: %v", err)
	}

	if addResp["sum"] != 15 {
		t.Errorf("expected sum 15, got %d", addResp["sum"])
	}
	var mulResp map[string]int
	if err := client.RequestTyped(ctx, "multiply", []int{2, 3, 4}, &mulResp); err != nil {
		t.Fatalf("multiply request failed: %v", err)
	}

	if mulResp["product"] != 24 {
		t.Errorf("expected product 24, got %d", mulResp["product"])
	}
}
