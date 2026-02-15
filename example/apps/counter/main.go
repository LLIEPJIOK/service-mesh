package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"sync/atomic"

	"github.com/LLIEPJIOK/ws-mesh/pkg/ws"
	"github.com/LLIEPJIOK/ws-mesh/pkg/ws/mesh/client"
)

var cnt int32

func counterHandler(w http.ResponseWriter, r *http.Request) {
	cnt++
	// 2 из 3 запросов возвращают ошибку
	// таким образом идёт проверка на повторы в sidecar
	// if cnt%3 != 0 {
	// 	http.Error(
	// 		w,
	// 		http.StatusText(http.StatusTooManyRequests),
	// 		http.StatusTooManyRequests,
	// 	)

	// 	return
	// }

	fmt.Fprint(w, cnt)
}

// WebSocket сервер для счётчика
func runWebSocketServer(tlsCfg *ws.TLSConfig) {
	cfg := ws.DefaultSecureServerConfig()
	cfg.TLS = tlsCfg

	server := ws.NewSecureServer(cfg)

	// Счётчик с атомарным инкрементом
	var wsCounter int64

	// Обработчик для получения текущего значения
	server.Handle("counter.get", func(ctx context.Context, payload json.RawMessage) (any, error) {
		return map[string]int64{"value": atomic.LoadInt64(&wsCounter)}, nil
	})

	// Обработчик для инкремента
	server.Handle(
		"counter.increment",
		func(ctx context.Context, payload json.RawMessage) (any, error) {
			newVal := atomic.AddInt64(&wsCounter, 1)
			return map[string]int64{"value": newVal}, nil
		},
	)

	// Обработчик для декремента
	server.Handle(
		"counter.decrement",
		func(ctx context.Context, payload json.RawMessage) (any, error) {
			newVal := atomic.AddInt64(&wsCounter, -1)
			return map[string]int64{"value": newVal}, nil
		},
	)

	// Обработчик для добавления произвольного значения
	server.Handle("counter.add", func(ctx context.Context, payload json.RawMessage) (any, error) {
		var req struct {
			Delta int64 `json:"delta"`
		}
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		newVal := atomic.AddInt64(&wsCounter, req.Delta)
		return map[string]int64{"value": newVal}, nil
	})

	mux := http.NewServeMux()
	mux.Handle("/", server)

	srv := http.Server{
		Addr:    ":9090",
		Handler: mux,
	}

	slog.Info("WebSocket server starting on :9090")
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal("WebSocket server error:", err)
	}
}

func main() {
	// Пробуем загрузить TLS конфигурацию
	tlsCfg, err := client.TLSConfigFromEnv()
	if err != nil {
		slog.Warn("TLS not configured, WebSocket server disabled", slog.Any("error", err))
	} else {
		slog.Info("TLS configured", slog.String("service", os.Getenv("SERVICE_NAME")))
		// Запускаем WebSocket сервер в горутине
		go runWebSocketServer(tlsCfg)
	}

	mux := http.NewServeMux()
	// запуск HTTP сервера
	mux.HandleFunc("/", counterHandler)

	srv := http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	fmt.Println("Server is running on http://localhost:8080")
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
