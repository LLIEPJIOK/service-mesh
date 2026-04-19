# MVP Spec (Hybrid)

Этот документ задает требования для MVP sidecar в формате spec-first: критичные части описаны нормативно (`MUST`/`SHOULD`), остальные - в описательном виде для быстрой реализации.

## Scope

MVP sidecar обеспечивает:

- Прозрачный перехват TCP-трафика приложения.
- mTLS между sidecar внутри mesh.
- Service discovery через EndpointSlice и Service.
- Балансировку исходящих соединений.
- Базовую отказоустойчивость на этапе установления соединения.
- Экспорт метрик в Prometheus.

## Термины

- `OriginalDst`: исходный адрес назначения соединения до REDIRECT.
- `Mesh endpoint`: endpoint, найденный через service discovery кэш.
- `External endpoint`: адрес, отсутствующий в service discovery кэше.

## Нормативные требования

### Перехват и порты

1. Sidecar MUST использовать три listener-порта: `inboundPlainPort`, `outboundPort`, `inboundMTLSPort`.
2. Inbound plain-трафик MUST быть перенаправлен на `inboundPlainPort` через `PREROUTING`.
3. Outbound-трафик приложения MUST быть перенаправлен на `outboundPort` через `OUTPUT`.
4. Порт `inboundMTLSPort` MUST слушаться напрямую, без REDIRECT на тот же порт.

### mTLS

1. Входящий трафик на `inboundMTLSPort` MUST проходить mutual TLS с проверкой клиентского сертификата.
2. Для mesh endpoint'ов исходящий dial SHOULD использовать mTLS.
3. Для external endpoint'ов исходящий dial MAY выполняться без mTLS.
4. Sidecar MUST валидировать peer-сертификаты через доверенный CA.
5. Для mesh endpoint'ов sidecar MUST передавать TLS `ServerName` из `Endpoint.ServiceName` (service FQDN), а не из IP-адреса.

### Service Discovery

1. Sidecar MUST выполнять первичную загрузку endpoint'ов через LIST EndpointSlice.
2. Sidecar MUST выполнять первичную загрузку service mapping через LIST Service.
3. Sidecar MUST поддерживать актуальность кэша через WATCH EndpointSlice.
4. Sidecar MUST поддерживать актуальность `serviceName <-> ClusterIP` mapping через WATCH Service.
5. Sidecar MUST учитывать только endpoint'ы с `Ready == true`.
6. Sidecar MUST хранить endpoint'ы под ключами `serviceName:port` и `clusterIP:port`.
7. Запись endpoint в кэше SHOULD содержать `IP`, `Port` и `ServiceName` (FQDN для mTLS `ServerName`).
8. При потере любого watch-соединения sidecar MUST выполнить повторный LIST и возобновить WATCH.

### Балансировка

1. Sidecar MUST поддерживать `roundRobin` и `random`.
2. Алгоритм балансировки MUST применяться при установлении нового соединения.
3. Уже установленные соединения MUST NOT перераспределяться.
4. Если endpoint не найден в кэше, sidecar MUST использовать fallback на `OriginalDst`.

### Отказоустойчивость

1. Retry MUST применяться только к ошибкам этапа установления соединения.
2. Retry MUST NOT срабатывать по HTTP-кодам в рамках MVP.
3. Timeout MUST ограничивать длительность dial/establish шага.
4. Circuit breaker MUST считать failure только для dial/tls/timeout ошибок.

### Наблюдаемость

1. Sidecar MUST экспортировать метрики на `/metrics`.
2. Sidecar MUST публиковать минимум набор метрик из observability-документа.
3. `metricsPort` MUST быть исключен из inbound REDIRECT (`excludeInboundPorts`).
4. Метрика retry SHOULD увеличиваться на каждую дополнительную попытку.

### Жизненный цикл

1. Sidecar MUST получить рабочий сертификат до запуска listener'ов.
2. При остановке sidecar MUST прекратить прием новых соединений и дождаться активных обработчиков в пределах timeout.

## Non-goals (MVP)

- Поддержка headless-сервисов (`clusterIP: None`).
- Retry по HTTP-ответам (`5xx`/`4xx`).
- Автоматическая ротация сертификатов.
- L7-aware маршрутизация и анализ payload.

## Acceptance criteria

| Функция              | Критерий приемки                                                                            |
| -------------------- | ------------------------------------------------------------------------------------------- |
| Перехват inbound     | Запрос к приложению снаружи pod достигает sidecar через `inboundPlainPort`                  |
| Перехват outbound    | Исходящее соединение приложения проходит через `outboundPort`                               |
| Incoming mTLS        | Соединение на `inboundMTLSPort` без валидного client cert отклоняется                       |
| Discovery LIST/WATCH | Добавление/удаление `Service` и pod endpoint отражается в кэше без рестарта sidecar         |
| Балансировка         | Для серии новых соединений по одному service наблюдается распределение по алгоритму         |
| Retry                | При искусственной dial-ошибке выполняются повторные попытки согласно `retryPolicy.attempts` |
| Timeout              | Соединение, не установленное за `timeout`, завершается с timeout ошибкой                    |
| Circuit breaker      | После `failureThreshold` ошибок endpoint блокируется до `recoveryTime`                      |
| Метрики              | Endpoint `/metrics` доступен и содержит минимум обязательные метрики                        |
| Graceful shutdown    | При SIGTERM новые соединения не принимаются, активные завершаются или обрываются по timeout |

## Канонический конфиг

```yaml
sidecar:
  inboundPlainPort: 15006
  outboundPort: 15002
  inboundMTLSPort: 15001
  metricsPort: 9090

  monitoringEnabled: true
  loadBalancerAlgorithm: roundRobin # roundRobin | random

  retryPolicy:
    attempts: 3
    backoff:
      type: exponential # exponential | linear
      baseInterval: 100ms

  timeout: 5s

  circuitBreakerPolicy:
    failureThreshold: 5
    recoveryTime: 30s

  excludeInboundPorts: "9090"
  excludeOutboundIPs: "169.254.169.254"
```

## Failure behavior (summary)

| Ситуация                                | Поведение MVP                     |
| --------------------------------------- | --------------------------------- |
| Service discovery недоступен при старте | Ошибка запуска sidecar            |
| WATCH обрывается во время работы        | Повторный LIST + перезапуск WATCH |
| Нет endpoint'ов в кэше                  | Fallback dial в `OriginalDst`     |
| Ошибка mTLS handshake inbound           | Соединение отклоняется            |
| Ошибка dial outbound                    | Retry/circuit breaker по политике |

## См. также

- [README Sidecar](../README.md)
- [Реализация sidecar](implementation.md)
- [Appendix: Code Snippets](appendix-code-snippets.md)
