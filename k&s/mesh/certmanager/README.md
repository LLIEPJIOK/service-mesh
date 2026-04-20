# Менеджер сертификатов

## Описание

Менеджер сертификатов (cert-manager) выпускает рабочие сертификаты для sidecar-компонентов внутри mesh. Основная задача сервиса - безопасно связать Kubernetes-идентичность (`ServiceAccount`) с X.509 сертификатом, чтобы mTLS строился на проверяемой identity, а не на данных из CSR.

## Scope MVP

В рамках MVP cert-manager обеспечивает:

- выпуск leaf-сертификатов для sidecar по запросу `POST /sign`;
- валидацию JWT service account токена через Kubernetes API `TokenReview`;
- проверку подписи CSR и выпуск сертификата на основе identity из токена;
- подписание сертификата корневым CA, смонтированным в cert-manager.

## Trust model

- Источник identity: Kubernetes `ServiceAccount` + JWT токен пода.
- Источник доверия: корневой CA, которым cert-manager подписывает leaf-сертификаты.
- Источник валидации токена: Kubernetes API `TokenReview`.
- Данные identity в CSR не являются источником истины и не должны определять выдаваемый сертификат.

## Нормативные требования

### Идентичность и токены

1. cert-manager MUST принимать запрос на выпуск сертификата только через `POST /sign`.
2. cert-manager MUST валидировать JWT токен через Kubernetes API `TokenReview`.
3. cert-manager MUST отклонять запрос, если токен невалиден, истек или не аутентифицирован.
4. cert-manager MUST формировать identity сертификата из claims service account токена.
5. cert-manager MUST NOT использовать `CommonName`/`DNSNames` из CSR как источник identity.

### Валидация CSR

1. cert-manager MUST декодировать и парсить CSR.
2. cert-manager MUST проверять подпись CSR через `csr.CheckSignature()`.
3. cert-manager MUST отклонять malformed CSR с ошибкой клиентского уровня.

### Выпуск сертификата

1. cert-manager MUST подписывать сертификат только корневым ключом CA.
2. cert-manager MUST задавать Subject/SAN на основе identity из токена, а не из CSR.
3. cert-manager MUST ограничивать срок leaf-сертификата и не превышать срок действия корневого CA.
4. cert-manager SHOULD возвращать leaf-сертификат и CA-сертификат в одном ответе.

### Безопасность и эксплуатация

1. cert-manager MUST хранить root CA key только в Kubernetes Secret, смонтированном в pod cert-manager.
2. cert-manager MUST NOT требовать монтирования root CA key в application pod.
3. cert-manager SHOULD вести аудит логов по запросам на выпуск и отказам.
4. cert-manager SHOULD ограничивать rate выдачи сертификатов на identity.

## Подготовка к работе

Для работы cert-manager нужны:

1. корневой CA (`Secret` с ключом и сертификатом);
2. конфигурация для распространения корневого CA в sidecar (`ConfigMap`);
3. service account для приложений, которые будут запрашивать сертификаты;
4. RBAC-права для cert-manager на использование `TokenReview`.

> [!IMPORTANT]
> Service account приложения создается hook-ом при инъекции sidecar (или задается вручную в Deployment).

Минимальный пример RBAC для cert-manager (валидация токенов через TokenReview):

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: cert-manager-tokenreviewer
rules:
  - apiGroups: ["authentication.k8s.io"]
    resources: ["tokenreviews"]
    verbs: ["create"]
```

## Принцип работы

1. Sidecar читает JWT токен из `/var/run/secrets/kubernetes.io/serviceaccount/token`.
2. Sidecar генерирует приватный ключ и CSR.
3. Sidecar отправляет `POST /sign` с `csr` и `token`.
4. cert-manager выполняет `TokenReview` и проверяет, что токен аутентифицирован и не просрочен.
5. cert-manager проверяет подпись CSR (`csr.CheckSignature()`).
6. cert-manager формирует certificate identity из claims токена и подписывает leaf-сертификат корневым CA.
7. cert-manager возвращает сертификат sidecar, после чего sidecar завершает TLS bootstrap.

> [!Note]
> Подробности по root CA и trust model см. в [Сертификаты](../../docs/cert/README.md).

## API

### Endpoint

`POST /sign`

### Request

```
POST /sign
Content-Type: application/json

{
	"csr": "pem-csr",
	"token": "jwt-token"
}
```

Поля запроса:

- `csr` (required): PEM-кодированный CSR.
- `token` (required): JWT service account токен пода.

### Response 200

```json
{
	"certificate": "-----BEGIN CERTIFICATE-----...",
	"ca": "-----BEGIN CERTIFICATE-----...",
	"identity": "default/reviews",
	"expiresAt": "2027-04-19T12:00:00Z"
}
```

### Error responses

| HTTP code | Причина                                   | Поведение                                             |
| --------- | ----------------------------------------- | ----------------------------------------------------- |
| `400`     | Некорректный JSON/CSR                     | Запрос отклоняется без повторной попытки на сервере   |
| `401`     | Невалидный или просроченный токен         | Запрос отклоняется                                    |
| `403`     | TokenReview не подтвердил identity/доступ | Запрос отклоняется                                    |
| `500`     | Внутренняя ошибка подписи/доступа к CA    | Запрос отклоняется, требуется вмешательство оператора |

## Failure behavior (summary)

| Ситуация                   | Поведение MVP                                                  |
| -------------------------- | -------------------------------------------------------------- |
| TokenReview API недоступен | cert-manager возвращает ошибку (`500`), сертификат не выдается |
| JWT токен просрочен        | cert-manager возвращает `401`                                  |
| CSR поврежден/невалиден    | cert-manager возвращает `400`                                  |
| Root CA key недоступен     | cert-manager возвращает `500`, выпуск останавливается          |

## Acceptance criteria

| Функция          | Критерий приемки                                               |
| ---------------- | -------------------------------------------------------------- |
| Token validation | Запрос с валидным JWT проходит TokenReview и обрабатывается    |
| Auth rejection   | Запрос с невалидным/просроченным JWT получает `401`            |
| CSR validation   | Запрос с некорректной подписью CSR получает `400`              |
| Identity binding | В выданном сертификате identity берется из токена, а не из CSR |
| Signing          | Сертификат подписан корневым CA и верифицируется sidecar       |

## Non-goals (MVP)

- Автоматическая ротация уже выданных сертификатов на стороне cert-manager.
- Поддержка нескольких root CA.
- CRL/OCSP и расширенные механизмы отзыва сертификатов.

> [!Note]
> Sidecar MUST отслеживать срок действия сертификата и запрашивать новый сертификат до истечения текущего.

## Практические команды (MVP)

Запускайте команды из директории `k&s/mesh/certmanager`.

```bash
cd k\&s/mesh/certmanager
```

### Локальная проверка и сборка

```bash
make fmt
make vet
make test
make build
```

Бинарник cert-manager будет создан в `bin/certmanager`.

### Сборка Docker-образа

```bash
make docker-build VERSION=v0.1.0 DOCKERHUB_NAMESPACE=lliepjiok IMAGE_NAME=cert-manager
```

Команда собирает образ и выставляет 2 тега:

- `lliepjiok/cert-manager:v0.1.0`
- `lliepjiok/cert-manager:latest`

### Push в Docker Hub

```bash
docker login
make docker-push VERSION=v0.1.0 DOCKERHUB_NAMESPACE=lliepjiok IMAGE_NAME=cert-manager
```

Для полного цикла (build + push):

```bash
make docker-build-push VERSION=v0.1.0 DOCKERHUB_NAMESPACE=lliepjiok IMAGE_NAME=cert-manager
```

## Конфигурация окружения

| Переменная            | Назначение                                               | Значение по умолчанию     |
| --------------------- | -------------------------------------------------------- | ------------------------- |
| `HTTP_ADDR`           | Адрес HTTP-сервера cert-manager                          | `:8080`                   |
| `PORT`                | Порт HTTP-сервера (используется, если `HTTP_ADDR` пуст)  | `8080`                    |
| `ROOT_CA_CERT_FILE`   | Путь к PEM-файлу корневого CA сертификата                | `/etc/mesh/ca/tls.crt`    |
| `ROOT_CA_KEY_FILE`    | Путь к PEM-файлу корневого CA приватного ключа           | `/etc/mesh/ca/tls.key`    |
| `LEAF_TTL`            | Срок действия выдаваемого leaf-сертификата               | `8760h`                   |
| `MAX_REQUEST_BYTES`   | Максимальный размер HTTP тела запроса                    | `1048576`                 |
| `RATE_LIMIT_RPS`      | Ограничение частоты запросов (`0` отключает ограничение) | `0`                       |
| `RATE_LIMIT_BURST`    | Размер burst для rate limit                              | `0`                       |
| `READ_HEADER_TIMEOUT` | Таймаут чтения HTTP заголовков                           | `10s`                     |
| `IDLE_TIMEOUT`        | Таймаут idle соединений                                  | `60s`                     |
| `SHUTDOWN_TIMEOUT`    | Таймаут graceful shutdown                                | `10s`                     |
| `KUBECONFIG`          | Путь к kubeconfig для локального запуска (вне кластера)  | `~/.kube/config` fallback |

> [!IMPORTANT]
> Для MVP cert-manager выставляет DNS SAN в формате `${serviceAccount}.${namespace}.svc.cluster.local` на основе identity из TokenReview. Это предполагает, что для демонстрационного сценария service name совпадает с service account name.

## См. также

- [Сертификаты](../../docs/cert/README.md)
- [Сервисный аккаунт](../../docs/service/account/README.md)
- [Роли](../../docs/role/README.md)
- [Service mesh hook](../hook/README.md)
- [Жизненный цикл sidecar](../sidecar/docs/lifecycle.md)
