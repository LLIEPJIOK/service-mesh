# Smoke-тесты MVP

## Описание

Этот раздел определяет минимальные smoke-проверки демонстрации: сайт должен запускаться и быть доступен, а метрики должны появляться в Prometheus и Grafana.

## Scope MVP

- Проверка доступности BookInfo после развёртывания.
- Проверка прохождения трафика через sidecar (косвенно через метрики/поведение).
- Проверка появления метрик в Prometheus.
- Проверка визуализации метрик в Grafana.

## Нормативные требования

### Preconditions

1. Перед запуском smoke-проверок MUST быть установлен mesh-контур.
2. BookInfo MUST быть развернут в namespace с включенной инъекцией sidecar.
3. Prometheus и Grafana MUST быть доступны оператору теста.

### Проверки

1. Тест доступности MUST подтвердить, что пользовательская страница BookInfo отдает успешный ответ.
2. Тест метрик MUST подтвердить наличие sidecar-метрик в Prometheus.
3. Тест дашборда SHOULD подтвердить, что Grafana отображает ненулевые точки по request count/latency.

## Smoke-сценарии

| ID    | Сценарий                                    | Ожидаемый результат                                                 |
| ----- | ------------------------------------------- | ------------------------------------------------------------------- |
| SMK-1 | Открыть страницу BookInfo                   | Страница доступна, HTTP 200                                         |
| SMK-2 | Сгенерировать несколько запросов к BookInfo | В ответах видна работа `reviews` сервиса                            |
| SMK-3 | Проверить Prometheus                        | Есть sidecar-метрики (`mesh_requests_total`, latency/error метрики) |
| SMK-4 | Проверить Grafana                           | На дашборде есть данные по запросам и латентности                   |

## Acceptance criteria

| Функция              | Критерий приемки                                              |
| -------------------- | ------------------------------------------------------------- |
| Доступность сайта    | BookInfo стабильно отвечает на запросы                        |
| Метрики в Prometheus | Метрики для workload присутствуют и обновляются               |
| Метрики в Grafana    | Графики request/latency отображают данные за текущий интервал |

## Failure behavior (summary)

| Ситуация                        | Поведение MVP                     |
| ------------------------------- | --------------------------------- |
| BookInfo не открывается         | Smoke-пакет считается проваленным |
| В Prometheus нет sidecar-метрик | Наблюдаемость не пройдена         |
| В Grafana пустые графики        | Наблюдаемость не пройдена         |

## Non-goals (MVP)

- Нагрузочные/перфоманс-тесты с KPI.
- Chaos и fault-injection сценарии.
- Полная матрица регрессионного тестирования.

## Практический smoke-runbook (minikube)

1. Установить mesh-контур:

```bash
k\&s/manifest/scripts/build-and-load-mesh-images-minikube.sh
k\&s/manifest/scripts/generate-mesh-config-minikube.sh
cd k\&s/mesh/installer
go run ./cmd/mesh install -f ../../manifest/generated/mesh-config.minikube.yaml --wait --timeout 10s
```

2. Развернуть Bookinfo:

```bash
k\&s/manifest/scripts/deploy-bookinfo-minikube.sh
```

3. Установить мониторинг:

```bash
k\&s/manifest/scripts/install-monitoring-minikube.sh
```

4. Проверить SMK-1 (доступность страницы):

```bash
open "http://$(minikube ip):31380/productpage"
curl -i "http://$(minikube ip):31380/productpage" | head -n 1
```

5. Проверить SMK-2 (распределение по reviews):

```bash
for i in $(seq 1 20); do
	curl -s "http://$(minikube ip):31380/productpage" | grep -E "(glyphicon-star|color=black|Reviews served by)" | head -n 1
done
```

6. Проверить SMK-3 (Prometheus):

```bash
curl -s "http://$(minikube ip):32001/api/v1/query?query=mesh_requests_total" | jq '.status'
```

7. Проверить SMK-4 (Grafana):

```bash
open "http://$(minikube ip):32000"
```

> [!NOTE]
> Для подтверждения mTLS и reliability дополнительно проверяйте логи `sidecar`, `mesh-cert-manager`, `mesh-webhook` и корреляцию с метриками `mesh_request_errors_total`, `mesh_retry_attempts_total`, `mesh_circuit_breaker_state`.

## Связанные разделы

- [Демонстрация service mesh](../README.md)
- [BookInfo приложение](../app/bookinfo/README.md)
- [Манифесты](../manifest/README.md)
- [Наблюдаемость sidecar](../mesh/sidecar/docs/observability.md)
