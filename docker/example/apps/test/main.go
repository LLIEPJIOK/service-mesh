package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/LLIEPJIOK/ws-mesh/pkg/ws"
	"github.com/LLIEPJIOK/ws-mesh/pkg/ws/mesh/client"
)

// HTML шаблон
const pageTemplate = `<!DOCTYPE html>
<html lang="ru">
<head>
    <meta charset="UTF-8">
    <title>Сообщение</title>
    <style>
        body {
            display: flex;
            justify-content: center;
            align-items: center;
            height: 100vh;
            margin: 0;
            font-family: sans-serif;
            background: #f0f0f0;
        }
        .message {
            text-align: center;
            padding: 20px;
            background: white;
            border-radius: 8px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.1);
        }
        .from {
            font-weight: bold;
            margin-bottom: 10px;
        }
        .text {
            font-size: 1.2em;
            margin-bottom: 10px;
        }
        .counter {
            font-size: 1em;
            color: #666;
        }
    </style>
</head>
<body>
    <div class="message">
        <div class="from">От: %s</div>
        <div class="text">Текст: %s</div>
    </div>
</body>
</html>`

func handleHome(w http.ResponseWriter, r *http.Request) {
	// Выводим всё в HTML
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, pageTemplate, os.Getenv("SERVICE_NAME"), "hello world!")
}

func counterHandler(w http.ResponseWriter, r *http.Request) {
	// Делаем запрос к внешнему счетчику
	client := http.Client{Timeout: 3 * time.Second}

	resp, err := client.Get("http://counter.localhost/")
	if err != nil {
		slog.Error("failed to get counter", slog.Any("error", err))

		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}

	defer func() {
		if clErr := resp.Body.Close(); clErr != nil {
			slog.Error("failed to close response body", slog.Any("error", clErr))
		}
	}()

	// Читаем полностью тело ответа
	counterBody, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("failed to get counter resp", slog.Any("error", err))
		http.Error(
			w,
			http.StatusText(http.StatusInternalServerError),
			http.StatusInternalServerError,
		)

		return
	}

	// Выводим всё в HTML
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, pageTemplate, "counter", string(counterBody))
}

// counterWSHandler получает счётчик через WebSocket
func counterWSHandler(meshClient *client.MeshClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		// Инкрементируем счётчик
		var resp struct {
			Value int64 `json:"value"`
		}
		if err := meshClient.RequestTyped(ctx, "counter.increment", nil, &resp); err != nil {
			slog.Error("failed to increment counter", slog.Any("error", err))
			http.Error(w, "Failed to increment", http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, pageTemplate, "counter (WebSocket)", fmt.Sprintf("Value: %d", resp.Value))
	}
}

func initCounterClient(tlsCfg *ws.TLSConfig) (*client.MeshClient, error) {
	serviceName := os.Getenv("SERVICE_NAME")
	if serviceName == "" {
		serviceName = "test-1"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	meshClient, err := client.New(ctx, client.Config{
		ServiceName: serviceName,
		TargetName:  "counter",
		TLS:         tlsCfg,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create mesh client: %w", err)
	}

	if err := meshClient.Connect(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to counter: %w", err)
	}

	return meshClient, nil
}

func main() {
	// Пробуем загрузить TLS конфигурацию
	tlsCfg, err := client.TLSConfigFromEnv()
	if err != nil {
		slog.Warn("TLS not configured", slog.Any("error", err))
	} else {
		slog.Info("TLS configured", slog.String("service", os.Getenv("SERVICE_NAME")))
	}

	// запуск сервера
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/counter", counterHandler)

	// Инициализируем WebSocket клиент к counter
	if tlsCfg != nil {
		meshClient, err := initCounterClient(tlsCfg)
		if err != nil {
			slog.Error("failed to init counter client", slog.Any("error", err))
		} else {
			defer meshClient.Close()
			http.HandleFunc("/counter-ws", counterWSHandler(meshClient))
			slog.Info("Counter WebSocket client initialized")
		}
	}

	fmt.Println("Server is running on http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
