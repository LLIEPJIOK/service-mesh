# Манифесты

## Описание

Этот раздел является минимальным индексом манифестов для запуска демонстрации service mesh. На текущем этапе документ задает структуру и правила, а детальная оркестрация выполняется через Mesh CLI.

## Scope MVP

- Навигация по манифестам и связанным спецификациям.
- Фиксация базовых требований к добавлению новых манифестов.
- Ссылки на компоненты, которые формируют итоговое окружение.

## Нормативные требования

1. Каталог `manifest/` MUST использоваться как точка входа для навигации по манифестам демо-окружения.
2. Каждый новый набор манифестов MUST быть связан ссылкой с соответствующей компонентной спецификацией (`mesh/*`).
3. Порядок применения манифестов MUST соответствовать контракту установки из Mesh CLI.
4. Манифесты SHOULD использовать единые namespace/label-конвенции проекта.

## Ожидаемые группы манифестов

| Группа        | Назначение                                                    | Источник контракта                                                                                           |
| ------------- | ------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------ |
| Платформенные | Базовые ресурсы mesh (namespace, RBAC, cert-manager, webhook) | [Mesh CLI](../mesh/installer/README.md)                                                                      |
| Data plane    | Sidecar-инъекция и трафик-перехват                            | [Hook](../mesh/hook/README.md), [Iptables](../mesh/iptables/README.md), [Sidecar](../mesh/sidecar/README.md) |
| Наблюдаемость | Метрики и scrape-контракт                                     | [Observability](../mesh/sidecar/docs/observability.md)                                                       |

> [!NOTE]
> В MVP этот файл остается минимальным индексом. По мере появления реальных YAML-пакетов сюда добавляются ссылки на конкретные каталоги/файлы.

## Текущий набор MVP манифестов

Платформенные ресурсы webhook-инжектора размещены отдельными файлами (один ресурс на файл):

- `mesh-system-namespace.yaml` - namespace `mesh-system`.
- `mesh-webhook-serviceaccount.yaml` - service account webhook-сервера.
- `mesh-webhook-deployment.yaml` - deployment webhook-сервера.
- `mesh-webhook-service.yaml` - service для admission webhook.
- `mesh-sidecar-injector.yaml` - `MutatingWebhookConfiguration` для мутации pod.

Порядок применения этого набора:

1. `mesh-system-namespace.yaml`
2. `mesh-webhook-serviceaccount.yaml`
3. `mesh-webhook-deployment.yaml`
4. `mesh-webhook-service.yaml`
5. `mesh-sidecar-injector.yaml`

> [!IMPORTANT]
> Перед применением `mesh-sidecar-injector.yaml` должен быть доступен TLS-секрет `mesh-webhook-tls` и заполнен `caBundle` в `MutatingWebhookConfiguration`.

## Платформенные ресурсы, применяемые installer из MeshConfig

Помимо файлового набора webhook, `mesh install` в MVP дополнительно применяет платформенные ресурсы, которые формируются из `MeshConfig`:

- Secret `mesh-root-ca` (корневой CA, источник из `spec.certificates.rootCA`).
- Secret `mesh-webhook-tls` (TLS для webhook-сервера, подписывается корневым CA).
- ServiceAccount `cert-manager`.
- ClusterRole `cert-manager-tokenreviewer`.
- ClusterRoleBinding `cert-manager-tokenreviewer-binding`.
- Deployment `mesh-cert-manager`.
- Service `mesh-cert-manager`.
- ConfigMap `mesh-sidecar-config` (канонические sidecar defaults).

Эти ресурсы являются частью платформенной группы и должны оставаться в install-order, описанном в [Mesh CLI](../mesh/installer/README.md).

## Acceptance criteria

| Критерий          | Условие                                                          |
| ----------------- | ---------------------------------------------------------------- |
| Полнота навигации | Из этого README можно перейти к ключевым mesh-спецификациям      |
| Консистентность   | Порядок применения не противоречит `mesh/installer` контракту    |
| Расширяемость     | Добавление новых манифестов не требует смены структуры документа |

## Failure behavior (summary)

| Ситуация                                             | Поведение MVP                              |
| ---------------------------------------------------- | ------------------------------------------ |
| Добавлены манифесты без ссылок на контракт           | Документация считается неполной            |
| Порядок применения расходится с installer-контрактом | Набор манифестов считается неконсистентным |

## Non-goals (MVP)

- Пошаговый deployment-runbook на все среды.
- Детальный reference каждого YAML в этом документе.

## Связанные разделы

- [Демонстрация service mesh](../README.md)
- [Mesh CLI](../mesh/installer/README.md)
- [Service mesh hook](../mesh/hook/README.md)
- [Iptables-init](../mesh/iptables/README.md)
- [Sidecar](../mesh/sidecar/README.md)
