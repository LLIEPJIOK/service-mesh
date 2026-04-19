# Балансировка нагрузки

## Описание

Sidecar выполняет балансировку исходящих запросов между экземплярами целевого сервиса.

При обращении к сервису (например, `reviews.default.svc.cluster.local`) sidecar получает список доступных endpoint’ов из Kubernetes и выбирает конкретный экземпляр сервиса для выполнения запроса.

Поддерживаются следующие алгоритмы:

- Round Robin
- Random

Настройка алгоритма балансировки выполняется с помощью переменной `loadBalancerAlgorithm`, которая может принимать значения `roundRobin` или `random`. Например:

```yaml
sidecar:
  loadBalancerAlgorithm: roundRobin # roundRobin | random
```

Балансировка выполняется на этапе установления соединения и не влияет на уже установленные соединения.

> [!NOTE]
> Балансировка выполняется на уровне sidecar, а не Kubernetes, что позволяет реализовывать более сложные политики, такие как retry и circuit breaker для отдельных экземпляров сервиса.

## Имплементация

Балансировка реализуется в `Forwarder`, который:

1. Берет `ctx.OriginalDst`.
2. Получает endpoint-структуры (`IP`, `Port`, `ServiceName`) из `ServiceCache`.
3. Выбирает endpoint алгоритмом `roundRobin` или `random`.
4. Устанавливает соединение с выбранным endpoint.

Если endpoint'ы не найдены в кэше, sidecar использует fallback на исходный адрес назначения.

```go
type Forwarder struct {
    UseTLS                bool
    TLSConfig             *tls.Config
    Cache                 *discovery.ServiceCache
    LoadBalancerAlgorithm string // roundRobin | random
}

func (f *Forwarder) selectEndpoint(originalDst string) (discovery.Endpoint, bool) {
    endpoints := f.Cache.GetEndpoints(originalDst)
    if len(endpoints) == 0 {
        return discovery.Endpoint{}, false
    }

    if f.LoadBalancerAlgorithm == "random" {
        return endpoints[rand.Intn(len(endpoints))], true
    }

    // default: roundRobin
    return f.nextRoundRobin(endpoints), true
}

func (f *Forwarder) Handle(ctx *ConnContext) error {
    ep, inMesh := f.selectEndpoint(ctx.OriginalDst)
    targetAddr := ctx.OriginalDst

    if inMesh {
        // Жестко направляем на порт mTLS принимающего сайдкара
    	targetAddr = net.JoinHostPort(ep.IP, "15001") 
    }

    var targetConn net.Conn
    var err error

    if f.UseTLS && inMesh {
        targetConn, err = mtls.ClientMTLS(targetAddr, ep.ServiceName, f.TLSConfig)
    } else {
        targetConn, err = net.Dial("tcp", targetAddr)
    }

    if err != nil {
        return err
    }

    // ... двунаправленное копирование
    return nil
}
```

Подробный reference-код см. в [Appendix: Code Snippets](appendix-code-snippets.md#forwarder-с-выбором-endpoint-и-mtls).

## Связь с mTLS

- Для mesh-внутренних endpoint'ов sidecar использует `mtls.ClientMTLS(targetAddr, ep.ServiceName, tlsConfig)`.
- Для внешних адресов sidecar использует plain TCP dial.

Сигнатура mTLS-клиента:

```go
func ClientMTLS(addr string, serverName string, tlsConfig *tls.Config) (net.Conn, error)
```

Это поведение должно совпадать с правилами в [Proxy](proxy.md#правила-mtls-для-исходящего-трафика).

## Связь с retry

При неуспешном dial retry-механизм может повторно выбрать endpoint. Политика повторов описана в [Отказоустойчивость](reliability.md).

## См. также

- [MVP Spec](mvp-spec.md)
- [Обнаружение сервисов](service-discovery.md)
- [Отказоустойчивость](reliability.md)
- [Proxy](proxy.md)
