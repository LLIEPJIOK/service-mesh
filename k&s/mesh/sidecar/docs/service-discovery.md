# Обнаружение сервисов

Sidecar использует Kubernetes API для получения списка доступных экземпляров сервиса через ресурсы EndpointSlice и Service.

При запуске sidecar выполняет начальную загрузку (LIST), после чего подписывается на изменения (WATCH). Все изменения автоматически обновляют локальный кэш endpoint’ов.

Локальный кэш используется для маршрутизации и балансировки.

## Контракт MVP

- Источник данных: `EndpointSlice` и `Service` в текущем namespace.
- В кэш попадают только endpoint'ы с `Ready == true`.
- В кэше endpoint хранится как структура с `IP`, `Port` и `ServiceName` (FQDN для TLS ServerName).
- Headless-сервисы (`clusterIP: None`) не поддерживаются.
- Кэш должен быть потокобезопасным (`RWMutex` или эквивалент).

## Сопоставление ClusterIP и сервиса

`SO_ORIGINAL_DST` возвращает IP-адрес и порт назначения (обычно ClusterIP сервиса Kubernetes). Для поиска списка endpoint'ов по этому ключу необходимо знать соответствие между ClusterIP и именем сервиса.

При старте sidecar выполняет LIST `Service` в текущем namespace. На основе этого строится маппинг:

```
"имя_сервиса:порт" -> "ClusterIP:порт"

```

Далее при обработке EndpointSlice список endpoint'ов сохраняется под двумя ключами:

- `имя_сервиса:порт` (для внутреннего использования)
- `ClusterIP:порт` (для поиска по результату `SO_ORIGINAL_DST`)

Таким образом, когда Forwarder получает `10.96.0.15:8080`, он может напрямую обратиться к кэшу и получить список доступных подов.

Чтобы sidecar видел новые сервисы после старта, mapping `ClusterIP -> ServiceName` должен поддерживаться через WATCH `Service`, а не только через стартовый LIST.

Reference-код вынесен в [Appendix: Code Snippets](appendix-code-snippets.md#service-discovery-list-watch).

## Начальная загрузка (LIST)

Sidecar сначала получает текущее состояние EndpointSlice и использует `serviceIPMap`, чтобы сохранить endpoint'ы под обоими ключами.

Алгоритм LIST:

1. Получить все `Service` в namespace и построить `serviceIPMap` (`serviceName:port -> clusterIP:port`).
2. Получить все `EndpointSlice` в namespace.
3. Для каждого slice вычислить `serviceName` через label `kubernetes.io/service-name`.
4. Отфильтровать endpoint'ы с `Ready != true`.
5. Сохранить endpoint-структуры под ключами `serviceName:port` и `clusterIP:port`.

В кэше endpoint хранится как структура:

```go
type Endpoint struct {
	IP          string
	Port        int
	ServiceName string // например "reviews.default.svc.cluster.local"
}
```

`ServiceName` используется как TLS `ServerName` при исходящем mTLS-соединении.

> [!IMPORTANT]
> При обработке EndpointSlice необходимо учитывать состояние Ready, так как не все endpoint’ы могут быть готовы к приёму трафика. Поэтому важно использовать только те endpoint’ы, которые имеют `Ready` в `true`.

## Подписка на изменения (WATCH)

Алгоритм WATCH:

1. Подписаться на изменения `EndpointSlice`.
2. Подписаться на изменения `Service`.
3. На `Service Added/Modified/Deleted` обновлять `serviceIPMap`.
4. На `EndpointSlice Added/Modified` пересчитывать endpoint-лист и обновлять кэш по обоим ключам.
5. На `EndpointSlice Deleted` удалять оба ключа из кэша.
6. При потере любого watch-соединения выполнить повторный LIST (`Service` + `EndpointSlice`) и заново запустить WATCH.

Обновление кэша должно выполняться безопасно для конкурентного доступа и без блокировки чтения (через RWMutex), чтобы обеспечить высокую производительность и избежать блокировок при обработке запросов.

> [!Note]
> Для получения доступа к Kubernetes API сайдкару нужна роль [Pod viewer](../../../docs/role/README.md#pod-viewer) с доступом к `services` и `endpointslices`.

## Интеграция с балансировкой

Forwarder использует `OriginalDst` как ключ кэша и получает список endpoint'ов для выбора целевого адреса. Детали выбора см. в [Балансировка нагрузки](balancing.md).

## См. также

- [MVP Spec](mvp-spec.md)
- [Proxy](proxy.md)
- [Балансировка нагрузки](balancing.md)
