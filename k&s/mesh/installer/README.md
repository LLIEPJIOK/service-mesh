# Mesh CLI

## Описание

Mesh CLI - это утилита командной строки на Go (Cobra), которая управляет жизненным циклом service mesh в Kubernetes-кластере. CLI должен обеспечивать предсказуемую установку и удаление компонентов mesh по единому конфигурационному файлу и фиксированному порядку применения ресурсов.

## Scope MVP

- Установка mesh-компонентов в кластер: `mesh install -f config.yaml`
- Удаление mesh-компонентов из кластера: `mesh uninstall`
- Поддержка YAML-файла конфигурации с параметрами mesh.
- Применение манифестов в правильном порядке (CRDs, RBAC, cert-manager, webhook, etc.).
- Ожидание готовности критических компонентов (cert-manager, webhook) перед завершением установки.
- Возможность указания namespace для mesh-системы (по умолчанию `mesh-system`).

## Термины

- `MeshConfig`: YAML-объект с параметрами установки mesh.
- `mesh namespace`: namespace, где живут системные компоненты (`mesh-system` по умолчанию).
- `critical components`: `cert-manager` и webhook, readiness которых проверяется на install.

## Нормативные требования

### Команда install

1. CLI MUST требовать параметр `-f/--file` для `mesh install`.
2. CLI MUST валидировать базовую структуру `MeshConfig` (`apiVersion`, `kind`, `spec`).
3. CLI MUST применять ресурсы в фиксированном порядке (см. раздел "Порядок установки").
4. CLI MUST завершать установку ошибкой, если критичные компоненты не стали Ready в рамках timeout.
5. CLI SHOULD поддерживать `--dry-run` для вывода ресурсов без применения.

### Команда uninstall

1. CLI MUST удалять ресурсы в обратном безопасном порядке (см. раздел "Порядок удаления").
2. CLI MUST быть идемпотентным: отсутствие ресурса не должно приводить к аварийному завершению.
3. CLI MUST NOT удалять CRD автоматически в MVP.
4. CLI MAY удалять namespace при явном флаге `--delete-namespace`.

### Конфигурация и namespace

1. CLI MUST использовать `mesh-system` как namespace по умолчанию.
2. CLI MUST позволять переопределить namespace флагом `--namespace/-n`.
3. CLI SHOULD выдавать предупреждение, если root CA не задан и генерируется автоматически.
4. CLI MUST определять kubeconfig в порядке: `--kubeconfig` -> `KUBECONFIG` -> `~/.kube/config` -> in-cluster config.
5. CLI SHOULD предупреждать о потенциальной несовместимости версии CLI и `spec.version`.
6. CLI MUST разрешать переопределение образов через `spec.images.*`; при отсутствии override использовать версии, согласованные с `spec.version`.

## Команды

### `mesh install`

Устанавливает service mesh в кластер.

```bash
mesh install -f <config.yaml> [flags]
```

#### Флаги

| Флаг           | Сокращение | Описание                                   | По умолчанию     |
| -------------- | ---------- | ------------------------------------------ | ---------------- |
| `--file`       | `-f`       | Путь к YAML-файлу конфигурации mesh.       | **обязательный** |
| `--namespace`  | `-n`       | Namespace для системных компонентов mesh.  | `mesh-system`    |
| `--wait`       | -          | Ожидать готовности критичных компонентов.  | `true`           |
| `--timeout`    | -          | Таймаут ожидания readiness.                | `5m`             |
| `--dry-run`    | -          | Показать ресурсы без применения в кластер. | `false`          |
| `--kubeconfig` | -          | Путь к kubeconfig.                         | текущий контекст |

#### Пример

```bash
mesh install -f mesh-config.yaml
```

### `mesh uninstall`

Удаляет service mesh из кластера.

```bash
mesh uninstall [flags]
```

#### Пример

```bash
mesh uninstall
```

#### Флаги

| Флаг                 | Сокращение | Описание                                      | По умолчанию     |
| -------------------- | ---------- | --------------------------------------------- | ---------------- |
| `--namespace`        | `-n`       | Namespace, где удаляются компоненты mesh.     | `mesh-system`    |
| `--delete-namespace` | -          | Удалить namespace после удаления компонентов. | `false`          |
| `--kubeconfig`       | -          | Путь к kubeconfig.                            | текущий контекст |

## Конфигурационный файл

Файл конфигурации в формате YAML определяет параметры устанавливаемой service mesh. Пример:

```yaml
# mesh-config.yaml
apiVersion: mesh.io/v1alpha1
kind: MeshConfig
metadata:
  name: default
spec:
  # Общие настройки
  namespace: mesh-system
  version: "0.1.0"

  # Явное закрепление образов (опционально)
  images:
    sidecar: mesh/sidecar:v0.1.0
    iptablesInit: mesh/iptables-init:v0.1.0
    certManager: mesh/cert-manager:v0.1.0

  # Сертификаты и безопасность
  certificates:
    rootCA:
      cert: |
        -----BEGIN CERTIFICATE-----
        ...
        -----END CERTIFICATE-----
      key: |
        -----BEGIN RSA PRIVATE KEY-----
        ...
        -----END RSA PRIVATE KEY-----
    validity: 8760h # 1 год

  # Конфигурация sidecar по умолчанию (может быть переопределена на уровне workload)
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
    excludeOutboundIPs: "169.254.169.254/32"

  # Настройки webhook-инжектора
  injection:
    namespaceSelector:
      matchLabels:
        mesh-injection: enabled

  # Настройки cert-manager (если используется встроенный)
  certManager:
    enabled: true
    resources: {}
```

Минимальные обязательные поля для MVP:

- `apiVersion: mesh.io/v1alpha1`
- `kind: MeshConfig`
- `spec.namespace`
- `spec.sidecar.inboundPlainPort`
- `spec.sidecar.outboundPort`
- `spec.sidecar.inboundMTLSPort`

> [!IMPORTANT]
> Если корневой CA не указан, CLI может сгенерировать самоподписанный сертификат автоматически (с выводом предупреждения). Однако для production окружения **настоятельно** рекомендуется предоставить собственный CA.

## Порядок установки

При выполнении `mesh install` CLI выполняет следующие шаги:

1. Создаёт namespace `mesh-system` (если не существует).
2. Применяет CRDs (если есть).
3. Создаёт Secret с корневым CA (`mesh-root-ca`).
4. Устанавливает cert-manager (Deployment + Service + RBAC).
5. Применяет ConfigMap с настройками sidecar по умолчанию.
6. Устанавливает webhook-сервер (MutatingWebhookConfiguration + Deployment + Service).
7. Применяет дополнительные компоненты (например, Prometheus Operator, если задано).

> [!IMPORTANT]
> Шаги установки должны быть детерминированными. CLI не должен переходить к шагам, зависящим от cert-manager/webhook, пока их readiness не подтверждена. ConfigMap с настройками sidecar должна быть применена до webhook.

## Порядок удаления

При выполнении `mesh uninstall`:

1. Удаляет MutatingWebhookConfiguration.
2. Удаляет Deployment и Service webhook-сервера.
3. Удаляет Deployment и Service cert-manager.
4. Удаляет Secret `mesh-root-ca`.
5. Удаляет ConfigMap с конфигурацией.
6. Удаляет RBAC ресурсы.
7. Опционально удаляет namespace `mesh-system` (`--delete-namespace`).

> [!WARNING]
> Удаление CRDs **не выполняется автоматически**, чтобы избежать потери пользовательских ресурсов. При необходимости удалить CRDs следует вручную.

## Failure behavior (summary)

| Ситуация                                          | Поведение MVP                                                          |
| ------------------------------------------------- | ---------------------------------------------------------------------- |
| Конфигурационный файл отсутствует или невалиден   | `mesh install` завершается ошибкой до применения ресурсов              |
| Namespace не может быть создан                    | установка останавливается с ошибкой                                    |
| cert-manager/webhook не Ready до timeout          | установка завершается ошибкой                                          |
| Недостаточно прав на создание/обновление ресурсов | установка завершается ошибкой с контекстом RBAC запрета                |
| Ошибка частичного удаления в `mesh uninstall`     | CLI продолжает удаление остальных ресурсов, возвращает итоговую ошибку |
| CRD отсутствует при удалении                      | операция пропускается без аварийного завершения                        |

## Acceptance criteria

| Функция            | Критерий приемки                                                         |
| ------------------ | ------------------------------------------------------------------------ |
| Install by config  | `mesh install -f` устанавливает компоненты в правильном порядке          |
| Readiness wait     | Команда завершается успешно только после readiness критичных компонентов |
| Dry-run            | `mesh install --dry-run` не изменяет кластер                             |
| Uninstall          | `mesh uninstall` удаляет mesh-компоненты в безопасном порядке            |
| Namespace handling | При `--delete-namespace` namespace удаляется в конце процесса            |
| Idempotency        | Повторный uninstall не падает из-за отсутствующих ресурсов               |

## Структура кода (Cobra)

```go
// cmd/mesh/main.go
package main

import (
    "os"

    "github.com/spf13/cobra"
    "mesh/cli/cmd"
)

func main() {
    rootCmd := &cobra.Command{
        Use:   "mesh",
        Short: "Mesh CLI manages the service mesh lifecycle",
    }

    rootCmd.AddCommand(cmd.NewInstallCmd())
    rootCmd.AddCommand(cmd.NewUninstallCmd())
    rootCmd.AddCommand(cmd.NewVersionCmd())

    if err := rootCmd.Execute(); err != nil {
        os.Exit(1)
    }
}
```

```go
// cmd/install.go
package cmd

import (
    "github.com/spf13/cobra"
)

func NewInstallCmd() *cobra.Command {
    var configFile string
    var namespace string
    var wait bool
    var timeout string
    var dryRun bool
    var kubeconfig string

    cmd := &cobra.Command{
        Use:   "install -f CONFIG",
        Short: "Install service mesh into the cluster",
        RunE: func(cmd *cobra.Command, args []string) error {
            // реализация установки
            return runInstall(configFile, namespace, wait, timeout, dryRun, kubeconfig)
        },
    }

    cmd.Flags().StringVarP(&configFile, "file", "f", "", "Path to mesh config YAML (required)")
    _ = cmd.MarkFlagRequired("file")
    cmd.Flags().StringVarP(&namespace, "namespace", "n", "mesh-system", "Namespace for mesh system components")
    cmd.Flags().BoolVar(&wait, "wait", true, "Wait for components to be ready")
    cmd.Flags().StringVar(&timeout, "timeout", "5m", "Timeout when waiting")
    cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Dry run (print resources, do not apply)")
    cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file")

    return cmd
}
```

## Ограничения MVP

- Нет поддержки обновления (upgrade) существующей mesh; для изменения конфигурации требуется `uninstall` и повторный `install`.
- Нет встроенной валидации конфигурации (только базовая проверка наличия обязательных полей).
- Нет управления несколькими кластерами (работает только с текущим контекстом kubeconfig).
- Удаление CRDs не автоматизировано.
- Строгая матрица совместимости версий CLI и компонентов пока не enforced (только предупреждения).

## Non-goals (MVP)

- Оркестрация rolling-upgrade mesh без переустановки.
- Полноценная схема валидации конфигурации с CEL/JSON Schema.
- Управление одновременно несколькими kube-context в одном запуске.

## Связанные разделы

- [Менеджер сертификатов](../certmanager/README.md)
- [Service mesh hook](../hook/README.md)
- [Конфигурация sidecar](../sidecar/README.md#конфигурации)
- [Iptables-init контейнер](../iptables/README.md)
