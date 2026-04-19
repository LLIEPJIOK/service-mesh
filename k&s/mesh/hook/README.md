# Service mesh hook

## Описание

Hook выполняет мутацию pod-объектов admission webhook-ом, чтобы pod автоматически получал все компоненты service mesh. На практике это применяется к pod, создаваемым workload-контроллерами (Deployment, StatefulSet, DaemonSet). В pod template добавляются:

- `serviceAccountName` для workload (без создания отдельного ресурса `ServiceAccount` в webhook);
- init‑контейнер для настройки правил iptables (прозрачный перехват трафика);
- sidecar‑контейнер, реализующий mTLS, балансировку и метрики;
- volume с корневым сертификатом CA;
- аннотации для интеграции с Prometheus.

Благодаря этому сервисы начинают работать через mesh без ручной донастройки каждого Deployment.

## Scope MVP

- Инжекция компонентов только при наличии разрешающей namespace-метки и отсутствии disable-аннотации.
- Добавление строго определённого набора полей в pod template (см. [Контракт мутации](#контракт-мутации)).
- Отсутствие сложной конфигурации на уровне отдельного workload (все параметры берутся из единого конфига mesh).
- Поддержка admission для ресурса `pods` через `MutatingWebhookConfiguration`.

## Нормативные требования

### Принятие решения об инъекции

1. Webhook MUST инжектировать sidecar только для namespace с меткой `mesh-injection: enabled`.
2. Webhook MUST пропускать pod с аннотацией `sidecar.mesh.io/inject: "false"`.
3. Webhook MUST пропускать pod из `kube-system` и `mesh-system`.

### Мутация pod template

1. Webhook MUST добавлять `iptables-init`, `sidecar`, `mesh-ca` volume и служебные аннотации согласно контракту ниже.
2. Webhook MUST NOT изменять поля pod, не перечисленные в контракте мутации.
3. Webhook MUST быть идемпотентным: повторная мутация не должна дублировать контейнеры, volume и аннотации.
4. Webhook SHOULD уважать существующий `spec.serviceAccountName` и не перезаписывать его.
5. Webhook MUST вычислять `serviceAccountName` по детерминированному алгоритму, если поле пустое.
6. Если `spec.serviceAccountName` пустое, webhook MUST проставить вычисленное значение через JSON Patch.
7. Webhook MUST NOT создавать или изменять ресурс `ServiceAccount` через Kubernetes API.
8. Webhook MUST синхронизировать `UID` для `iptables-init` с `securityContext.runAsUser` sidecar.

### Надежность

1. При `failurePolicy: Fail` недоступность webhook MUST блокировать создание/обновление pod в целевых namespace.
2. Webhook MUST отвечать в пределах `timeoutSeconds`.
3. Webhook SHOULD логировать причину отказа/пропуска мутации.

## Правила инъекции (когда хук срабатывает)

Хук обрабатывает pod **только** при одновременном выполнении условий:

1. Namespace, в котором создаётся pod, имеет метку `mesh-injection: enabled`.
2. Pod **не** имеет аннотации `sidecar.mesh.io/inject: "false"`.
3. Namespace pod не входит в список исключений (`kube-system`, `mesh-system`).

> [!IMPORTANT]
> Проверка на уровне namespace label выполняется **на стороне Kubernetes** с помощью `namespaceSelector` в `MutatingWebhookConfiguration`. Проверка аннотации и имени namespace реализуется в коде самого webhook-сервера.

## Контракт мутации

Хук модифицирует `podTemplate` строго по описанным ниже правилам. Любое поле, не упомянутое здесь, остаётся без изменений.

### 1. ServiceAccount

- Если `spec.serviceAccountName` уже заполнено, webhook **не** перезаписывает его.
- Если `spec.serviceAccountName` пустое, webhook вычисляет имя в порядке приоритета:
  1. `metadata.labels["app"]`
  2. `metadata.labels["app.kubernetes.io/name"]`
  3. имя контроллера из `metadata.ownerReferences` (для controller=true)
  4. fallback: `default`
- Webhook только патчит `spec.serviceAccountName`; создание соответствующего `ServiceAccount` должно выполняться манифестами приложения/оператором.

### 2. Init‑контейнер `iptables-init`

Добавляется первым в списке `spec.initContainers`.

| Поле                               | Значение                                                                                      |
| ---------------------------------- | --------------------------------------------------------------------------------------------- |
| `name`                             | `iptables-init`                                                                               |
| `image`                            | `mesh/iptables-init:latest` (версия синхронизируется с релизом mesh)                          |
| `imagePullPolicy`                  | `IfNotPresent`                                                                                |
| `securityContext.capabilities.add` | `["NET_ADMIN"]`                                                                               |
| `env`                              | Переменные окружения для настройки портов (см. [Переменные окружения](#переменные-окружения)) |

Init‑контейнер выполняет скрипт, который:

- создаёт цепочки `MESH_INBOUND` и `MESH_OUTPUT` в таблице `nat`;
- настраивает правила `REDIRECT` согласно конфигурации портов;
- добавляет исключения для `excludeInboundPorts` и `excludeOutboundIPs`.

После завершения работы init‑контейнер завершается с кодом `0`.

### 3. Sidecar‑контейнер

Добавляется в `spec.containers` после всех существующих контейнеров.

| Поле              | Значение                                                                                                                   |
| ----------------- | -------------------------------------------------------------------------------------------------------------------------- |
| `name`            | `sidecar`                                                                                                                  |
| `image`           | `mesh/sidecar:latest`                                                                                                      |
| `imagePullPolicy` | `IfNotPresent`                                                                                                             |
| `securityContext` | `runAsNonRoot: true`, `runAsUser: 1337`                                                                                    |
| `ports`           | `- containerPort: 15001` (inbound mTLS)<br>`- containerPort: 15002` (outbound)<br>`- containerPort: 15006` (inbound plain) |
| `env`             | Переменные окружения для конфигурации sidecar (см. [Переменные окружения](#переменные-окружения))                          |
| `volumeMounts`    | `- name: mesh-ca` mountPath: `/etc/mesh/ca` readOnly: true                                                                 |

> [!NOTE]
> В MVP порт `15000` используется только для health‑проб; метрики экспортируются на отдельном порту `metricsPort` (обычно `9090`), который **не** перехватывается iptables.

### 4. Volumes

Добавляется volume `mesh-ca` типа `configMap`.

| Поле             | Значение                                               |
| ---------------- | ------------------------------------------------------ |
| `name`           | `mesh-ca`                                              |
| `configMap.name` | `mesh-root-ca` (имя ConfigMap с корневым сертификатом) |

### 5. Аннотации

| Аннотация                  | Значение                             |
| -------------------------- | ------------------------------------ |
| `prometheus.io/scrape`     | `"true"`                             |
| `prometheus.io/port`       | `"9090"`                             |
| `prometheus.io/path`       | `"/metrics"`                         |
| `sidecar.mesh.io/injected` | `"true"`                             |
| `sidecar.mesh.io/version`  | Версия релиза mesh из `spec.version` |

Аннотации `prometheus.io/*` добавляются **только** если в конфигурации mesh включён мониторинг (`monitoringEnabled: true`).

## Переменные окружения

### Init‑контейнер `iptables-init`

| Имя                     | Описание                                                                               | Пример               |
| ----------------------- | -------------------------------------------------------------------------------------- | -------------------- |
| `INBOUND_PORTS`         | Список портов приложения, на которые нужно перенаправлять трафик (через запятую)       | `8080,8443`          |
| `INBOUND_PLAIN_PORT`    | Порт sidecar для входящего plain‑трафика                                               | `15006`              |
| `OUTBOUND_PORT`         | Порт sidecar для исходящего трафика                                                    | `15002`              |
| `EXCLUDE_INBOUND_PORTS` | Порты, исключаемые из inbound‑редиректа (обычно `metricsPort`)                         | `9090`               |
| `EXCLUDE_OUTBOUND_IPS`  | IP‑адреса (или подсети), исключаемые из outbound‑редиректа (например, metadata server) | `169.254.169.254/32` |
| `UID`                   | UID sidecar; MUST совпадать с `sidecar.securityContext.runAsUser`                      | `1337`               |

### Sidecar‑контейнер

| Имя                       | Описание                                                   | Источник значения               |
| ------------------------- | ---------------------------------------------------------- | ------------------------------- |
| `POD_NAME`                | Имя пода                                                   | `metadata.name` (fieldRef)      |
| `POD_NAMESPACE`           | Namespace пода                                             | `metadata.namespace` (fieldRef) |
| `SERVICE_ACCOUNT`         | Имя сервисного аккаунта                                    | `spec.serviceAccountName`       |
| `INBOUND_PLAIN_PORT`      | Порт для входящего plain‑трафика                           | `15006`                         |
| `OUTBOUND_PORT`           | Порт для исходящего трафика                                | `15002`                         |
| `INBOUND_MTLS_PORT`       | Порт для входящего mTLS‑трафика                            | `15001`                         |
| `METRICS_PORT`            | Порт для экспорта метрик Prometheus                        | `9090`                          |
| `CERT_FILE`               | Путь к файлу сертификата sidecar                           | `/etc/mesh/certs/tls.crt`       |
| `KEY_FILE`                | Путь к файлу приватного ключа                              | `/etc/mesh/certs/tls.key`       |
| `CA_FILE`                 | Путь к файлу корневого CA                                  | `/etc/mesh/ca/ca.crt`           |
| `LOAD_BALANCER_ALGORITHM` | Алгоритм балансировки (`roundRobin` или `random`)          | из конфигурации mesh            |
| `RETRY_ATTEMPTS`          | Количество попыток при dial‑ошибках                        | из `retryPolicy.attempts`       |
| `TIMEOUT`                 | Таймаут установления соединения                            | из `timeout`                    |
| `CIRCUIT_BREAKER_*`       | Параметры circuit breaker (failureThreshold, recoveryTime) | из `circuitBreakerPolicy`       |

## Пример мутации (YAML)

Исходный pod spec:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-app
  namespace: default
  labels:
    app: my-app
spec:
  containers:
    - name: app
      image: my-app:v1
      ports:
        - containerPort: 8080
```

После обработки хук‑ом (сокращённо, только ключевые изменения):

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-app
  namespace: default
  labels:
    app: my-app
  annotations:
    prometheus.io/scrape: "true"
    prometheus.io/port: "9090"
    prometheus.io/path: "/metrics"
    sidecar.mesh.io/injected: "true"
    sidecar.mesh.io/version: "v0.1.0"
spec:
  serviceAccountName: my-app
  initContainers:
    - name: iptables-init
      image: mesh/iptables-init:latest
      securityContext:
        capabilities:
          add: ["NET_ADMIN"]
      env:
        - name: INBOUND_PORTS
          value: "8080"
        - name: INBOUND_PLAIN_PORT
          value: "15006"
        - name: OUTBOUND_PORT
          value: "15002"
        - name: EXCLUDE_INBOUND_PORTS
          value: "9090"
        - name: EXCLUDE_OUTBOUND_IPS
          value: "169.254.169.254/32"
        - name: UID
          value: "1337"
  containers:
    - name: app
      image: my-app:v1
      ports:
        - containerPort: 8080
    - name: sidecar
      image: mesh/sidecar:latest
      securityContext:
        runAsNonRoot: true
        runAsUser: 1337
      ports:
        - containerPort: 15001
          name: mesh-mtls
        - containerPort: 15002
          name: mesh-outbound
        - containerPort: 15006
          name: mesh-inbound
      env:
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: SERVICE_ACCOUNT
          value: my-app
        - name: INBOUND_PLAIN_PORT
          value: "15006"
        - name: OUTBOUND_PORT
          value: "15002"
        - name: INBOUND_MTLS_PORT
          value: "15001"
        - name: METRICS_PORT
          value: "9090"
        - name: CERT_FILE
          value: /etc/mesh/certs/tls.crt
        - name: KEY_FILE
          value: /etc/mesh/certs/tls.key
        - name: CA_FILE
          value: /etc/mesh/ca/ca.crt
        - name: LOAD_BALANCER_ALGORITHM
          value: roundRobin
        - name: RETRY_ATTEMPTS
          value: "3"
        - name: TIMEOUT
          value: "5s"
        - name: CIRCUIT_BREAKER_FAILURE_THRESHOLD
          value: "5"
        - name: CIRCUIT_BREAKER_RECOVERY_TIME
          value: "30s"
      volumeMounts:
        - name: mesh-ca
          mountPath: /etc/mesh/ca
          readOnly: true
  volumes:
    - name: mesh-ca
      configMap:
        name: mesh-root-ca
```

## Конфигурация MutatingWebhookConfiguration

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: mesh-sidecar-injector
webhooks:
  - name: sidecar-injector.mesh.io
    clientConfig:
      service:
        name: mesh-webhook
        namespace: mesh-system
        path: "/mutate"
      caBundle: <base64-encoded-ca-cert>
    rules:
      - operations: ["CREATE", "UPDATE"]
        apiGroups: [""]
        apiVersions: ["v1"]
        resources: ["pods"]
    namespaceSelector:
      matchLabels:
        mesh-injection: enabled
    objectSelector: {}
    sideEffects: None
    failurePolicy: Fail
    timeoutSeconds: 10
    admissionReviewVersions: ["v1"]
```

> [!IMPORTANT]
> `failurePolicy: Fail` означает, что при недоступности webhook-сервера создание/обновление подов в помеченных namespace будет блокироваться. Для production‑окружения это рекомендуемый режим. В средах разработки можно временно установить `Ignore`.

## Failure behavior (summary)

| Ситуация                                                         | Поведение MVP                                        |
| ---------------------------------------------------------------- | ---------------------------------------------------- |
| Webhook недоступен и `failurePolicy: Fail`                       | Создание/обновление pod блокируется                  |
| Namespace не имеет `mesh-injection: enabled`                     | Мутация пропускается                                 |
| Pod содержит `sidecar.mesh.io/inject: "false"`                   | Мутация пропускается                                 |
| `serviceAccountName` указывает на отсутствующий `ServiceAccount` | Kubernetes отклоняет создание pod на этапе валидации |
| Ошибка сборки patch-ответа                                       | Admission возвращает ошибку, pod не создается        |

## Acceptance criteria

| Функция                   | Критерий приемки                                                                                    |
| ------------------------- | --------------------------------------------------------------------------------------------------- |
| Namespace-gated injection | В namespace с `mesh-injection: enabled` pod получает init-container, sidecar и volume               |
| Opt-out                   | Pod с аннотацией `sidecar.mesh.io/inject: "false"` остается без инъекции                            |
| Idempotency               | Повторная обработка pod не дублирует injected-поля                                                  |
| Metrics annotations       | При `monitoringEnabled: true` в pod добавляются `prometheus.io/*` аннотации                         |
| SA behavior               | При пустом `serviceAccountName` webhook вычисляет имя (или `default`) и патчит pod без side effects |

## Non-goals (MVP)

- Глубокая per-workload настройка sidecar параметров через аннотации.
- Инъекция в namespace системного уровня (`kube-system`, `mesh-system`).
- Поддержка сложных стратегий partial-injection для отдельных контейнеров pod.

## Связанные разделы

- [Менеджер сертификатов](../certmanager/README.md)
- [Iptables-init](../iptables/README.md)
- [Жизненный цикл sidecar](../sidecar/docs/lifecycle.md)
- [Сертификаты](../../docs/cert/README.md)
- [Манифесты](../../manifest/README.md)
