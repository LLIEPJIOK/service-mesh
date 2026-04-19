# Mesh

## Описание

`mesh/` содержит инфраструктурные компоненты service mesh, которые обеспечивают инъекцию sidecar, перехват трафика, выпуск сертификатов, запуск компонентов и observability-контракт.

## Scope MVP

- Автоматическая инъекция `sidecar` и `iptables-init` в целевые pod через webhook.
- Прозрачный inbound/outbound перехват трафика на уровне pod.
- Выпуск сертификатов для mTLS внутри mesh.
- Установка и удаление компонентов через единый CLI-поток.
- Экспорт и сбор метрик для проверки работоспособности mesh.

## Нормативные требования

### Оркестрация компонентов

1. Установка mesh MUST выполняться в детерминированном порядке через installer-контракт.
2. Компоненты mesh MUST запускаться в системном namespace mesh.
3. Конфигурация sidecar MUST быть доступна до начала мутации workload.

### Инъекция и data plane

1. Hook MUST инжектировать `sidecar` и `iptables-init` в pod, удовлетворяющие политике инъекции.
2. `iptables-init` MUST применить правила перехвата трафика до старта приложения.
3. Sidecar MUST обрабатывать входящий и исходящий трафик согласно proxy/lifecycle контрактам.

### Идентичность и безопасность

1. Cert-manager MUST выдавать сертификаты на основе identity из `ServiceAccount` токена.
2. Sidecar MUST использовать выданные сертификаты для mTLS-соединений в mesh.
3. Корневой CA MUST храниться только в доверенных системных ресурсах mesh.

## Карта компонентов

| Компонент     | Назначение                                                    | Документ                                       |
| ------------- | ------------------------------------------------------------- | ---------------------------------------------- |
| `installer`   | Установка и удаление mesh-компонентов                         | [Mesh CLI](installer/README.md)                |
| `hook`        | Мутация pod и инъекция sidecar/init-контейнера                | [Service mesh hook](hook/README.md)            |
| `iptables`    | Прозрачный перехват трафика                                   | [Iptables-init](iptables/README.md)            |
| `certmanager` | Выпуск сертификатов и trust contract                          | [Менеджер сертификатов](certmanager/README.md) |
| `sidecar`     | Data plane: proxy, discovery, balancing, reliability, metrics | [Sidecar](sidecar/README.md)                   |

## Acceptance criteria

| Функция          | Критерий приемки                                             |
| ---------------- | ------------------------------------------------------------ |
| Инъекция         | Целевой pod содержит `iptables-init` и `sidecar`             |
| Перехват трафика | Входящий/исходящий трафик проходит через sidecar             |
| Идентичность     | Sidecar получает валидный сертификат через cert-manager      |
| Установка        | `mesh install` выполняет развертывание без нарушения порядка |
| Observability    | Sidecar-метрики доступны для сбора и отображения             |

## Failure behavior (summary)

| Ситуация                        | Поведение MVP                                                      |
| ------------------------------- | ------------------------------------------------------------------ |
| Webhook недоступен              | Инъекция не выполняется, mesh-функции для workload недоступны      |
| Ошибка iptables-init            | Pod не проходит инициализацию data plane                           |
| Cert-manager недоступен         | Sidecar не получает сертификат, mTLS-соединения не устанавливаются |
| Невалидная конфигурация install | Installer завершает выполнение с ошибкой                           |
| Нет sidecar-метрик              | Проверка observability считается не пройденной                     |

## Связанные разделы

- [Демонстрация service mesh](../README.md)
- [Манифесты](../manifest/README.md)
- [Smoke-тесты](../test/README.md)
- [MVP Spec sidecar](sidecar/docs/mvp-spec.md)
- [Sidecar](sidecar/README.md)
- [Service mesh hook](hook/README.md)
- [Iptables-init](iptables/README.md)
- [Менеджер сертификатов](certmanager/README.md)
- [Mesh CLI](installer/README.md)
