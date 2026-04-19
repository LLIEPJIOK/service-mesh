# Наблюдаемость

## Мониторинг

Сайдкар может быть использован для мониторинга трафика, проходящего через приложение. Он может собирать метрики, такие как количество запросов, время ответа и количество ошибок, и отправлять их в систему Prometheus. Это позволяет строить красивые графики в Grafana, отслеживать производительность приложения и выявлять потенциальные проблемы.

> [!IMPORTANT]
> Prometheus должен автоматически обнаруживать сайдкары и контейнеры с специальными аннотациями приложений и собирать метрики с endpoint’а `/metrics`

Мониторинг настраивается с помощью переменной `monitoringEnabled`, которая включает или отключает сбор метрик (по умолчанию включён). Например:

```yaml
sidecar:
  monitoringEnabled: true # Включить сбор метрик для мониторинга
  metricsPort: 9090
  excludeInboundPorts: "9090"
```

> [!Note]
> Сбор метрик с самого приложения не входит в функциональность сайдкара. Вместо этого система мониторинга сама собирает метрики с endpoint’ов приложений, используя стандартные механизмы, такие как Prometheus scrape.

> [!IMPORTANT]
> `metricsPort` MUST быть исключен из inbound REDIRECT через `excludeInboundPorts`, иначе `/metrics` может зациклиться через proxy.

## Цель

Наблюдаемость должна позволять быстро ответить:

1. проходит ли трафик через sidecar;
2. есть ли ошибки в сети/сертификатах;
3. деградирует ли latency.

## Endpoint метрик

- Sidecar экспортирует метрики на `/metrics`.
- Рекомендуемый порт sidecar метрик: `9090` (`metricsPort`).

## Контракт метрик (минимум)

| Метрика                         | Тип       | Labels                          | Назначение                      |
| ------------------------------- | --------- | ------------------------------- | ------------------------------- |
| `mesh_requests_total`           | Counter   | `service,status_code,direction` | Количество запросов             |
| `mesh_request_duration_seconds` | Histogram | `service,direction`             | Латентность                     |
| `mesh_request_errors_total`     | Counter   | `service,error_type`            | Сетевые/прокси ошибки           |
| `mesh_retry_attempts_total`     | Counter   | `service`                       | Повторные попытки               |
| `mesh_circuit_breaker_state`    | Gauge     | `service`                       | 0 closed / 1 open / 2 half-open |
| `mesh_endpoints_ready`          | Gauge     | `service`                       | Количество ready endpoints      |

### Семантика labels

- `direction`: `inbound` или `outbound`.
- `service`: целевой service identity (или `external` для внешних адресов).
- `error_type`: нормализованные категории (`dial_error`, `tls_error`, `timeout`, `proxy_error`).

## Prometheus scrape

Используйте pod annotations:

```yaml
prometheus.io/scrape: "true"
prometheus.io/path: "/metrics"
prometheus.io/port: "9090"
```

## Реализация

```go
// Пример middleware для сбора Prometheus метрик
type MetricsMiddleware struct {
	connections prometheus.Counter
	bytesIn     prometheus.Counter
	bytesOut    prometheus.Counter
}

func (m *MetricsMiddleware) Handle(ctx *proxy.ConnContext, next func(*proxy.ConnContext) error) error {
	m.connections.Inc()

	// Оборачиваем соединение для подсчёта байт
	ctx.ClientConn = &meteredConn{
		Conn: ctx.ClientConn,
		onRead: func(n int) { m.bytesIn.Add(float64(n)) },
		onWrite: func(n int) { m.bytesOut.Add(float64(n)) },
	}

	return next(ctx)
}
```

## Связь с reliability

- `mesh_retry_attempts_total` должен увеличиваться на каждую retry-попытку.
- `mesh_circuit_breaker_state` должен отражать текущее состояние breaker для endpoint/service.

## См. также

- [MVP Spec](mvp-spec.md)
- [Proxy](proxy.md)
- [Отказоустойчивость](reliability.md)
