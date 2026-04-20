# Sidecar

## Описание

Sidecar - это L7 прокси-компонент, который работает рядом с приложением и обеспечивает сетевые функции service mesh на уровне pod. Он перехватывает входящий и исходящий трафик приложения и добавляет mTLS, сервис-дискавери, балансировку, метрики и базовые механизмы отказоустойчивости.

## Что входит в MVP

- Прозрачный перехват входящего и исходящего TCP-трафика через iptables (см. [Proxy](docs/proxy.md)).
- mTLS между sidecar-компонентами в mesh (см. [Proxy](docs/proxy.md)).
- Обнаружение endpoint'ов через Kubernetes EndpointSlice (см. [Обнаружение сервисов](docs/service-discovery.md)).
- Балансировка исходящих соединений (`roundRobin`, `random`) (см. [Балансировка нагрузки](docs/balancing.md)).
- Retry/timeout/circuit breaker на этапе установления исходящего соединения (см. [Отказоустойчивость](docs/reliability.md)).
- Экспорт метрик sidecar на `/metrics` (см. [Наблюдаемость](docs/observability.md)).

## Ограничения MVP

- Headless-сервисы (`clusterIP: None`) не поддерживаются.
- Retry по HTTP-статусам (например, `5xx`) не выполняется: в MVP поддерживаются только ошибки установления соединения.
- Автоматическая ротация сертификатов отсутствует.

## Навигация

- [MVP Spec](docs/mvp-spec.md) - основной spec-first документ для генерации кода.
- [Реализация sidecar](docs/implementation.md) - архитектурное ядро и карта компонентов.
- [Жизненный цикл](docs/lifecycle.md) - запуск/остановка и listener-профили.
- [Proxy](docs/proxy.md) - перехват трафика, SO_ORIGINAL_DST, mTLS-маршрутизация.
- [Обнаружение сервисов](docs/service-discovery.md) - LIST/WATCH и кэш endpoint'ов.
- [Балансировка нагрузки](docs/balancing.md) - выбор endpoint и интеграция с discovery.
- [Отказоустойчивость](docs/reliability.md) - retry/timeout/circuit breaker.
- [Наблюдаемость](docs/observability.md) - метрики и scrape-контракт.
- [Appendix: Code Snippets](docs/appendix-code-snippets.md) - длинные reference-примеры.

## Конфигурации

Конфигурация sidecar сохраняется при инициализации service mesh и добавляется в workload с помощью [hook-а](../hook/README.md#service-mesh-hook). Ниже указан канонический набор полей для MVP.

> [!IMPORTANT]
> Ключи в этом разделе являются источником истины для остальных документов sidecar.

Пример итоговой конфигурации:

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

  excludeInboundPorts: "9090" # metricsPort должен быть исключен
  excludeOutboundIPs: "169.254.169.254"
```

## Реализация

Подробную архитектурную реализацию sidecar см. в [Реализация sidecar](docs/implementation.md).

## Практические команды (MVP)

После добавления реализации доступны базовые команды для проверки, сборки и публикации образа.

Запускайте команды из директории `k&s/mesh/sidecar`.

```bash
cd k\&s/mesh/sidecar
```

### Локальная проверка и сборка

```bash
make fmt
make vet
make test
make build
```

Бинарник sidecar будет создан в `bin/sidecar`.

### Сборка Docker-образа

```bash
make docker-build VERSION=v0.1.0 DOCKERHUB_NAMESPACE=lliepjiok IMAGE_NAME=mesh-sidecar
```

Команда собирает образ и выставляет 2 тега:

- `lliepjiok/mesh-sidecar:v0.1.0`
- `lliepjiok/mesh-sidecar:latest`

### Push в Docker Hub

```bash
docker login
make docker-push VERSION=v0.1.0 DOCKERHUB_NAMESPACE=lliepjiok IMAGE_NAME=mesh-sidecar
```

Для полного цикла (build + push) можно использовать:

```bash
make docker-build-push VERSION=v0.1.0 DOCKERHUB_NAMESPACE=lliepjiok IMAGE_NAME=mesh-sidecar
```

### Минимальный запуск sidecar (ручной)

```bash
INBOUND_PLAIN_PORT=15006 \
OUTBOUND_PORT=15002 \
INBOUND_MTLS_PORT=15001 \
METRICS_PORT=9090 \
APP_TARGET_ADDR=127.0.0.1:8080 \
BOOTSTRAP_CERTIFICATES=false \
CERT_FILE=/etc/mesh/certs/tls.crt \
KEY_FILE=/etc/mesh/certs/tls.key \
CA_FILE=/etc/mesh/ca/ca.crt \
./bin/sidecar
```

> [!IMPORTANT]
> Перехват `SO_ORIGINAL_DST` реализован для Linux runtime. Для Kubernetes deployment sidecar должен запускаться в Linux pod.
