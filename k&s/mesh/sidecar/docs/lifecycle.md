# Жизненный цикл

Документ описывает порядок запуска и остановки sidecar, а также требования к listener-профилям в MVP.

## Запуск

При запуске pod sidecar выполняет следующие шаги:

1. Init container настраивает iptables
2. Sidecar читает token из `/var/run/secrets/kubernetes.io/serviceaccount/token`
3. Sidecar создаёт CSR и отправляет его в cert-manager (контракт запроса: [cert-manager API](../../certmanager/README.md#api), детали trust model: [Сертификаты для сервисов](../../../docs/cert/README.md#сертификаты-для-сервисов))
4. Sidecar получает рабочий сертификат и инициализирует TLS-конфигурацию
5. Sidecar поднимает listener'ы proxy
6. Приложение начинает обрабатывать трафик

> [!IMPORTANT]
> Sidecar MUST блокировать запуск listener'ов до получения сертификата, иначе mTLS-гарантии не выполняются.

## Профили listener'ов

| Listener | Порт                         | Назначение                              | Outgoing TLS                | Incoming mTLS |
| -------- | ---------------------------- | --------------------------------------- | --------------------------- | ------------- |
| incoming | `inboundPlainPort` (`15006`) | Трафик от внешних клиентов к приложению | `false`                     | `false`       |
| outgoing | `outboundPort` (`15002`)     | Исходящий трафик приложения             | `true` для mesh-endpoint'ов | `false`       |
| mtls     | `inboundMTLSPort` (`15001`)  | Входящий трафик от других sidecar       | `false`                     | `true`        |

Правила перехвата и mTLS-маршрутизация детализированы в [Proxy](proxy.md).

## Остановка

Для корректной остановки sidecar используется graceful shutdown. При SIGTERM/SIGINT sidecar выполняет следующие шаги:

1. Останавливает приём новых соединений (`listener.Close`)
2. Ждёт завершения активных обработчиков с таймаутом (`shutdownTimeout`)
3. Закрывает ресурсы и завершает процесс

## Ограничения MVP

- Ротация сертификатов не реализована; sidecar использует сертификат, полученный при старте pod.
- Graceful shutdown timeout фиксированный (по умолчанию `30s`) и может быть параметризован в будущих версиях.

## См. также

- [MVP Spec](mvp-spec.md)
- [Proxy](proxy.md)
- [Реализация sidecar](implementation.md)
