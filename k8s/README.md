# Демонстрация service mesh

## Описание

Этот репозиторий описывает MVP-демонстрацию service mesh на Kubernetes для разработки и валидации с помощью AI-агента. Цель демо - подтвердить, что приложение доступно через mesh, трафик корректно распределяется, а метрики видны в Prometheus и Grafana.

## Scope MVP

- Развертывание инфраструктурных компонентов mesh.
- Развертывание демонстрационного приложения BookInfo.
- Автоматическая инъекция sidecar и init-контейнера в целевые pod.
- Проверка доступности сайта (smoke).
- Проверка наблюдаемости: метрики появляются в Prometheus/Grafana.

## Структура репозитория

| Путь            | Назначение                                                       |
| --------------- | ---------------------------------------------------------------- |
| `app/`          | Демонстрационные приложения для проверки mesh-сценариев.         |
| `app/bookinfo/` | BookInfo как основной workload для smoke-проверок.               |
| `manifest/`     | Индекс и правила для манифестов окружения.                       |
| `mesh/`         | Спецификации и контракты компонентов mesh.                       |
| `test/`         | Smoke-спецификация проверок доступности и мониторинга.           |
| `docs/`         | Дополнительные доменные документы (cert, role, service account). |

## Нормативные требования

### Демонстрационный контур

1. MVP MUST запускаться в Kubernetes-кластере.
2. Demo workload MUST быть доступен по HTTP после установки mesh.
3. Для целевых workload MUST применяться инъекция sidecar через webhook.

### Проверки результата

1. Демо MUST показать распределение трафика между версиями сервиса `reviews`.
2. Метрики sidecar MUST быть доступны на endpoint `/metrics` и собираться Prometheus.
3. Grafana SHOULD отображать не нулевые значения минимум по request count и latency.

## Acceptance criteria

| Сценарий               | Критерий приемки                                               |
| ---------------------- | -------------------------------------------------------------- |
| Доступность приложения | Страница BookInfo открывается и отвечает HTTP 200              |
| Распределение трафика  | Повторные запросы показывают ответы от разных версий `reviews` |
| Метрики в Prometheus   | В Prometheus есть метрики sidecar для BookInfo workload        |
| Визуализация в Grafana | На дашборде видны метрики request count/latency                |

## Failure behavior (summary)

| Ситуация                          | Поведение MVP                                          |
| --------------------------------- | ------------------------------------------------------ |
| Workload недоступен после deploy  | Smoke-проверка считается проваленной                   |
| Sidecar не инжектирован           | Сценарий mesh-валидации считается невалидным           |
| Метрики не появились в Prometheus | Наблюдаемость считается не пройденной                  |
| Grafana не показывает данные      | MVP не считается завершенным по observability-критерию |

## Non-goals (MVP)

- Полноценные нагрузочные и chaos-тесты.
- Много-кластерные сценарии.
- Production-hardening за рамками smoke-проверок.

## Быстрый старт на minikube

```bash
# 1) Собрать и загрузить mesh-образы в minikube
k\&s/manifest/scripts/build-and-load-mesh-images-minikube.sh

# 2) Сгенерировать MeshConfig с локальным root CA
k\&s/manifest/scripts/generate-mesh-config-minikube.sh

# 3) Установить mesh
cd k\&s/mesh/installer
go run ./cmd/mesh install -f ../../manifest/generated/mesh-config.minikube.yaml --wait --timeout 5m

# 4) Развернуть Bookinfo
k\&s/manifest/scripts/deploy-bookinfo-minikube.sh

# 5) Поднять мониторинг (Prometheus + Grafana)
k\&s/manifest/scripts/install-monitoring-minikube.sh
```

Проверка страницы Bookinfo:

```bash
open "http://$(minikube ip):31380/productpage"
```

## Связанные разделы

- [BookInfo приложение](app/bookinfo/README.md)
- [Манифесты](manifest/README.md)
- [Smoke-тесты](test/README.md)
- [Mesh CLI](mesh/installer/README.md)
- [Service mesh hook](mesh/hook/README.md)
- [Proxy sidecar](mesh/sidecar/README.md)
- [Наблюдаемость sidecar](mesh/sidecar/docs/observability.md)
