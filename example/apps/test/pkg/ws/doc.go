// Package ws предоставляет WebSocket клиент и сервер с поддержкой:
//   - Конкурентных запросов (клиент может отправлять много запросов одновременно)
//   - Маршрутизации на стороне сервера (обработчики регистрируются по имени маршрута)
//   - Request-Response паттерна с автоматической корреляцией по ID
//   - Кастомного mTLS шифрования (ECDH key exchange + AES-256-GCM)
//
// # Обычный сервер (без шифрования)
//
//	server := ws.NewServer(ws.DefaultServerConfig())
//	server.Handle("echo", func(ctx context.Context, payload json.RawMessage) (any, error) {
//	    var req Request
//	    json.Unmarshal(payload, &req)
//	    return Response{...}, nil
//	})
//	http.Handle("/ws", server)
//
// # Обычный клиент (без шифрования)
//
//	client := ws.NewClient(ws.DefaultClientConfig("ws://localhost:8080/ws"))
//	client.Connect(ctx)
//	var resp Response
//	client.RequestTyped(ctx, "echo", Request{...}, &resp)
//
// # Защищенный сервер (с шифрованием)
//
//	cfg := ws.DefaultSecureServerConfig()
//	cfg.TLS = &ws.TLSConfig{
//	    Certificate:    cert,      // Сертификат сервера
//	    CertificatePEM: certPEM,   // Сертификат в PEM для отправки клиенту
//	    RootCAs:        rootPool,  // Для верификации клиентских сертификатов
//	    VerifyPeerCertificate: true,
//	}
//	server := ws.NewSecureServer(cfg)
//
// # Защищенный клиент (с шифрованием)
//
//	cfg := ws.DefaultSecureClientConfig("ws://localhost:8080/ws")
//	cfg.TLS = &ws.TLSConfig{
//	    Certificate:    cert,
//	    CertificatePEM: certPEM,
//	    RootCAs:        rootPool,
//	    VerifyPeerCertificate: true,
//	}
//	client := ws.NewSecureClient(cfg)
//
// # Протокол шифрования
//
// При установке соединения происходит обмен ключами:
//  1. Клиент генерирует ECDH ключевую пару (P-256)
//  2. Клиент отправляет публичный ключ и сертификат (handshake_init)
//  3. Сервер генерирует свою ECDH пару
//  4. Сервер вычисляет shared secret и отправляет свой публичный ключ (handshake_response)
//  5. Клиент вычисляет shared secret
//  6. Обе стороны используют HKDF для получения AES-256 ключа
//  7. Все последующие сообщения шифруются AES-256-GCM
//
// # Протокол сообщений
//
// Незашифрованные сообщения:
//
//	{"id": 123, "route": "user.get", "payload": {...}, "error": "..."}
//
// Зашифрованные сообщения:
//
//	{"nonce": "base64...", "ciphertext": "base64..."}
package ws
