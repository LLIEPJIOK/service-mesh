package main

import (
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"time"
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
	fmt.Fprintf(w, pageTemplate, "себя", "hello world!")
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
	fmt.Fprintf(w, pageTemplate, "счётчик", string(counterBody))
}

func main() {
	// запуск сервера
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/counter", counterHandler)

	fmt.Println("Server is running on http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
