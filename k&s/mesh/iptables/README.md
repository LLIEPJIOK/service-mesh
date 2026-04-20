# Iptables-init

## Описание

Init-контейнер `iptables-init` настраивает правила `iptables` в namespace сети pod для прозрачного перехвата TCP-трафика приложения. Контейнер запускается до основных контейнеров, применяет правила и завершается.

Перехваченный трафик перенаправляется на listener-порты sidecar, где затем применяются mTLS, балансировка и отказоустойчивость.

## Scope MVP

- Настройка правил `REDIRECT` для входящего (inbound) и исходящего (outbound) трафика.
- Исключение из редиректа заданных портов (например, порта метрик).
- Исключение из редиректа заданных IP-адресов (например, metadata server).
- Поддержка только IPv4.
- Работа в окружении Kubernetes с установленным `iptables` (legacy или nft).

## Нормативные требования

### Безопасность и запуск

1. Init-контейнер MUST запускаться с `NET_ADMIN` capability.
2. Init-контейнер MUST выполняться до запуска app и sidecar контейнеров.
3. Init-контейнер MUST завершаться кодом `0`, если правила применены успешно.

### Идемпотентность

1. Скрипт MUST безопасно обрабатывать повторный запуск.
2. Перед созданием цепочек `MESH_INBOUND` и `MESH_OUTPUT` скрипт MUST удалить старые привязки и цепочки, если они существуют.

### Перехват трафика

1. Inbound-перехват MUST выполняться через `PREROUTING -> MESH_INBOUND -> REDIRECT`.
2. Outbound-перехват MUST выполняться через `OUTPUT -> MESH_OUTPUT -> REDIRECT`.
3. Трафик sidecar пользователя (`UID`) MUST исключаться из outbound-перехвата.
4. DNS-трафик (`53/tcp`, `53/udp`) MUST исключаться из outbound-перехвата.
5. Порты из `EXCLUDE_INBOUND_PORTS` и IP/CIDR из `EXCLUDE_OUTBOUND_IPS` MUST исключаться из редиректа.

## Термины

- `inbound`: входящий трафик к приложению в pod.
- `outbound`: исходящий трафик приложения из pod.
- `redirect`: перенаправление трафика на sidecar listener-порты.

## Принцип работы

1. Контейнер запускается с `securityContext.capabilities.add: ["NET_ADMIN"]`.
2. Считывает конфигурацию из переменных окружения.
3. Создаёт новые цепочки `MESH_INBOUND` и `MESH_OUTPUT` в таблице `nat`.
4. Добавляет правила перехвата, исключений и перенаправления.
5. Завершается с кодом `0`.

## Переменные окружения

| Имя                     | Описание                                                                                      | Пример               | Обязательность |
| ----------------------- | --------------------------------------------------------------------------------------------- | -------------------- | -------------- |
| `INBOUND_PORTS`         | Список портов приложения, трафик на которые нужно перехватывать (через запятую)               | `8080,8443`          | Да             |
| `INBOUND_PLAIN_PORT`    | Порт sidecar, на который перенаправляется входящий plain‑трафик                               | `15006`              | Да             |
| `OUTBOUND_PORT`         | Порт sidecar, на который перенаправляется исходящий трафик                                    | `15002`              | Да             |
| `EXCLUDE_INBOUND_PORTS` | Порты, исключаемые из inbound‑редиректа (обычно порт метрик и health‑check)                   | `9090,8081`          | Нет            |
| `EXCLUDE_OUTBOUND_IPS`  | IP‑адреса или подсети (CIDR), исключаемые из outbound‑редиректа (например, `169.254.169.254`) | `169.254.169.254/32` | Нет            |
| `UID`                   | UID пользователя sidecar; MUST совпадать с `sidecar.securityContext.runAsUser`                | `1337`               | Да             |

## Правила iptables

### Инициализация цепочек

```bash
iptables -t nat -N MESH_INBOUND
iptables -t nat -N MESH_OUTPUT
iptables -t nat -A OUTPUT -j MESH_OUTPUT
```

### Входящий трафик (inbound)

1. Трафик, поступающий в pod, попадает в цепочку `PREROUTING`.
2. Для каждого порта из `INBOUND_PORTS` создаётся правило перехода в `MESH_INBOUND`.
3. В цепочке `MESH_INBOUND` исключаются порты из `EXCLUDE_INBOUND_PORTS` (`-j RETURN`).
4. Оставшийся трафик перенаправляется на `INBOUND_PLAIN_PORT`.

```bash
# Пример для порта 8080
iptables -t nat -A PREROUTING -p tcp --dport 8080 -j MESH_INBOUND

# Исключения
iptables -t nat -A MESH_INBOUND -p tcp --dport 9090 -j RETURN

# Redirect
iptables -t nat -A MESH_INBOUND -p tcp -j REDIRECT --to-port 15006
```

### Исходящий трафик (outbound)

1. Трафик, генерируемый приложением, проходит через цепочку `OUTPUT` и передаётся в `MESH_OUTPUT`.
2. Трафик от пользователя с `UID` sidecar исключается (`-m owner --uid-owner 1337 -j RETURN`), чтобы избежать петель.
3. Исключается трафик к IP из `EXCLUDE_OUTBOUND_IPS` (`-j RETURN`).
4. Исключается трафик к локальным адресам (`127.0.0.0/8`).
5. Исключается DNS‑трафик (порт `53` UDP/TCP).
6. Весь оставшийся TCP‑трафик перенаправляется на `OUTBOUND_PORT`.

```bash
# Исключения
iptables -t nat -A MESH_OUTPUT -m owner --uid-owner 1337 -j RETURN
iptables -t nat -A MESH_OUTPUT -d 169.254.169.254/32 -j RETURN
iptables -t nat -A MESH_OUTPUT -d 127.0.0.0/8 -j RETURN
iptables -t nat -A MESH_OUTPUT -p udp --dport 53 -j RETURN
iptables -t nat -A MESH_OUTPUT -p tcp --dport 53 -j RETURN

# Перенаправление
iptables -t nat -A MESH_OUTPUT -p tcp -j REDIRECT --to-port 15002
```

> [!IMPORTANT]
> Правила должны быть идемпотентными: перед созданием цепочек необходимо удалить существующие с теми же именами.

## Failure behavior (summary)

| Ситуация                                                                                  | Поведение MVP                                         |
| ----------------------------------------------------------------------------------------- | ----------------------------------------------------- |
| Отсутствует `NET_ADMIN` capability                                                        | init-контейнер завершается с ошибкой, pod не стартует |
| Не задан обязательный env (`INBOUND_PORTS`, `INBOUND_PLAIN_PORT`, `OUTBOUND_PORT`, `UID`) | init-контейнер завершается с ошибкой                  |
| Ошибка применения iptables команды                                                        | init-контейнер завершается с ошибкой                  |
| Повторный запуск init-контейнера                                                          | правила переинициализируются без дублирования         |

## Acceptance criteria

| Функция               | Критерий приемки                                                    |
| --------------------- | ------------------------------------------------------------------- |
| Inbound redirect      | Трафик на app port перенаправляется на `INBOUND_PLAIN_PORT`         |
| Outbound redirect     | Исходящий TCP-трафик приложения перенаправляется на `OUTBOUND_PORT` |
| Exclude inbound ports | Порты из `EXCLUDE_INBOUND_PORTS` не редиректятся                    |
| Exclude outbound IPs  | IP/CIDR из `EXCLUDE_OUTBOUND_IPS` не редиректятся                   |
| Loop prevention       | Трафик от `UID` sidecar не зацикливается                            |
| Idempotency           | Повторный запуск скрипта не создает дубликатов цепочек/правил       |

## Ограничения MVP

- Поддержка только IPv4.
- Нет настройки `TPROXY` (исходный IP клиента теряется).
- Исключения задаются статически через переменные окружения; динамическое обновление не поддерживается.
- Не обрабатываются протоколы, отличные от TCP.
- Правила применяются на уровне всего pod network namespace; выборочное исключение отдельных app-контейнеров не поддерживается.

## Non-goals (MVP)

- Поддержка IPv6 правил.
- Runtime-реконфигурация iptables без перезапуска pod.
- L7-фильтрация трафика на уровне init-контейнера.

## Пример Dockerfile

```dockerfile
FROM alpine:3.18
RUN apk add --no-cache iptables
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
```

## Пример entrypoint.sh

```bash
#!/bin/sh
set -e

# Обязательные переменные окружения
: ${INBOUND_PORTS:?} ${INBOUND_PLAIN_PORT:?} ${OUTBOUND_PORT:?} ${UID:?}

# Удаляем старые цепочки (игнорируем ошибки, если их нет)
iptables -t nat -D PREROUTING -j MESH_INBOUND 2>/dev/null || true
iptables -t nat -D OUTPUT -j MESH_OUTPUT 2>/dev/null || true
iptables -t nat -F MESH_INBOUND 2>/dev/null || true
iptables -t nat -X MESH_INBOUND 2>/dev/null || true
iptables -t nat -F MESH_OUTPUT 2>/dev/null || true
iptables -t nat -X MESH_OUTPUT 2>/dev/null || true

# Создаём новые цепочки
iptables -t nat -N MESH_INBOUND
iptables -t nat -N MESH_OUTPUT
iptables -t nat -A OUTPUT -j MESH_OUTPUT

# Inbound правила
for port in $(echo $INBOUND_PORTS | tr ',' ' '); do
    iptables -t nat -A PREROUTING -p tcp --dport $port -j MESH_INBOUND
done

if [ -n "$EXCLUDE_INBOUND_PORTS" ]; then
    for port in $(echo $EXCLUDE_INBOUND_PORTS | tr ',' ' '); do
        iptables -t nat -A MESH_INBOUND -p tcp --dport $port -j RETURN
    done
fi

iptables -t nat -A MESH_INBOUND -p tcp -j REDIRECT --to-port $INBOUND_PLAIN_PORT

# Outbound правила
iptables -t nat -A MESH_OUTPUT -m owner --uid-owner $UID -j RETURN
if [ -n "$EXCLUDE_OUTBOUND_IPS" ]; then
    for ip in $(echo $EXCLUDE_OUTBOUND_IPS | tr ',' ' '); do
        iptables -t nat -A MESH_OUTPUT -d $ip -j RETURN
    done
fi
iptables -t nat -A MESH_OUTPUT -d 127.0.0.0/8 -j RETURN
iptables -t nat -A MESH_OUTPUT -p udp --dport 53 -j RETURN
iptables -t nat -A MESH_OUTPUT -p tcp --dport 53 -j RETURN
iptables -t nat -A MESH_OUTPUT -p tcp -j REDIRECT --to-port $OUTBOUND_PORT

echo "iptables rules applied successfully"
```

## Практические команды (MVP)

Запускайте команды из директории `k&s/mesh/iptables`.

```bash
cd k\&s/mesh/iptables
```

### Сборка Docker-образа

```bash
make docker-build VERSION=v0.1.0 DOCKERHUB_NAMESPACE=lliepjiok IMAGE_NAME=iptables-init
```

Команда собирает образ и выставляет 2 тега:

- `lliepjiok/iptables-init:v0.1.0`
- `lliepjiok/iptables-init:latest`

Для получения тегов в формате hook-контракта (например, `mesh/iptables-init:latest`) укажите соответствующий namespace:

```bash
make docker-build VERSION=v0.1.0 DOCKERHUB_NAMESPACE=mesh IMAGE_NAME=iptables-init
```

### Push в Docker Hub

```bash
docker login
make docker-push VERSION=v0.1.0 DOCKERHUB_NAMESPACE=lliepjiok IMAGE_NAME=iptables-init
```

Для полного цикла (build + push) можно использовать:

```bash
make docker-build-push VERSION=v0.1.0 DOCKERHUB_NAMESPACE=lliepjiok IMAGE_NAME=iptables-init
```

## Интеграция с hook

Хук добавляет init‑контейнер со всеми необходимыми переменными окружения. Полный контракт см. в [Service mesh hook](../hook/README.md).

## Связанные разделы

- [Proxy sidecar](../sidecar/docs/proxy.md)
- [Service mesh hook](../hook/README.md)
- [Жизненный цикл sidecar](../sidecar/docs/lifecycle.md)
