# Отказоустойчивость

## Описание

Sidecar является универсальным компонентом, который может быть использован в качестве точки для обеспечения отказоустойчивости приложения. В частности он может быть использован для реализации таких механизмов, как повторения, таймауты и предохранитель.

> [!Note]
> Механизмы отказоустойчивости применяются только к исходящему трафику и только на этапе установления соединения.

В MVP sidecar не анализирует HTTP-ответы и не выполняет retry по `5xx`. Повторения выполняются только при ошибках установления соединения.

## Повторения

Повторения - механизм, который позволяет повторить запрос в случае его неудачи. Это может быть полезно в случае временных сбоев, таких как сетевые ошибки или перегрузка сервиса.

В MVP retry срабатывает при ошибках типа dial/TLS-handshake/timeout. Ошибки прикладного уровня (например, HTTP-коды) не участвуют в решении о повторе.

> [!IMPORTANT]
> Идемпотентность повторяемых запросов остаётся ответственностью разработчиков приложения

Поддерживаются следующие типы повторений:

1. Линейные - выполняются с фиксированным интервалом времени между попытками.
2. Экспоненциальные - выполняются с увеличивающимся интервалом времени между попытками, что позволяет избежать перегрузки сервиса при его временной недоступности.

Повторения настраиваются с помощью политики `retryPolicy`, которая содержит количество повторения и тип (экспоненциальный или линейный) с базовым интервалом между повторениями. Например:

```yaml
retryPolicy:
  attempts: 3
  backoff:
    type: exponential # exponential | linear
    baseInterval: 100ms
```

### Реализация

Повторения реализованы в виде middleware (см. раздел ["Контракт middleware"](implementation.md#контракт-middleware)), который вызывает следующий обработчик несколько раз при dial-ошибках.

```go
// Пример middleware для реализации политики повторных попыток при ошибках соединения
type DialRetryMiddleware struct {
    maxRetries int
    backoff    BackoffStrategy
}

func (m *DialRetryMiddleware) Handle(ctx *proxy.ConnContext, next func(*proxy.ConnContext) error) error {
    var lastErr error
    for attempt := 0; attempt <= m.maxRetries; attempt++ {
        if attempt > 0 {
            time.Sleep(m.backoff.Duration(attempt))
        }
        err := next(ctx)
        if err == nil {
            return nil
        }
        // Повторяем только ошибки установления соединения
        if isDialError(err) {
            lastErr = err
            // Можно также выбрать другой endpoint из балансировщика и обновить ctx.OriginalDst
            continue
        }
        return err // другие ошибки не повторяем
    }
    return fmt.Errorf("dial failed after %d retries: %w", m.maxRetries, lastErr)
}
```

## Таймауты

Таймауты - механизм, который позволяет ограничить время ожидания ответа от сервиса. Это может быть полезно для предотвращения зависания приложения при недоступности сервиса или при его медленной работе.

Таймаут настраивается с помощью переменной `timeout`, которая задаёт максимальное время ожидания трафика соответственно. Например:

```yaml
timeout: 5s # Максимальное время ожидания ответа от сервиса
```

## Реализация

Таймауты реализованы с помощью middleware (см. раздел ["Контракт middleware"](implementation.md#контракт-middleware)), которое использует контекст с дедлайном. Например:

```go
// Пример middleware для реализации таймаута на уровне отдельных endpoint'ов
type TimeoutMiddleware struct {
    timeout time.Duration
}

func (m *TimeoutMiddleware) Handle(ctx *proxy.ConnContext, next func(*proxy.ConnContext) error) error {
    ctxWithTimeout, cancel := context.WithTimeout(ctx.Context, m.timeout)
    defer cancel()

    nextCtx := *ctx
    nextCtx.Context = ctxWithTimeout

    done := make(chan error, 1)
    go func() {
        done <- next(&nextCtx)
    }()

    select {
    case err := <-done:
        return err
    case <-ctxWithTimeout.Done():
        return fmt.Errorf("request timed out after %s", m.timeout)
    }
}
```

## Предохранитель

Предохранитель - механизм, который позволяет предотвратить перегрузку сервиса при его временной недоступности. Он работает по принципу "открытого" и "закрытого" состояния. В "открытом" состоянии предохранитель пропускает все запросы, а в "закрытом" состоянии он блокирует все запросы. Переход между состояниями происходит на основе определённых условий, таких как количество неудачных запросов или время, прошедшее с момента последнего успешного запроса.

Предохранитель настраивается с помощью политики `circuitBreakerPolicy`, которая содержит условия для перехода между состояниями. Например:

```yaml
circuitBreakerPolicy:
  failureThreshold: 5 # Количество неудачных запросов для перехода в "открытое" состояние
  recoveryTime: 30s # Время, через которое предохранитель перейдёт в "закрытое" состояние после перехода в "открытое" состояние
```

### Реализация

Предохранитель реализован в виде middleware (см. раздел ["Контракт middleware"](implementation.md#контракт-middleware)), который блокирует dial при переходе в "open" состояние.

В MVP "неудачей" считается ошибка установления соединения (`dial`, `tls handshake`, `timeout`).

```go
// Пример middleware для реализации circuit breaker на уровне отдельных endpoint'ов
type EndpointBreaker struct {
    breaker *gobreaker.CircuitBreaker
    ip      string
}

func NewEndpointBreaker(ip string, settings gobreaker.Settings) *EndpointBreaker {
    return &EndpointBreaker{
        breaker: gobreaker.NewCircuitBreaker(settings),
        ip:      ip,
    }
}

func (b *EndpointBreaker) Execute(fn func() (interface{}, error)) (interface{}, error) {
    return b.breaker.Execute(fn)
}
```

> [!NOTE]
> Дедлайны и таймауты можно задать в net.Dialer

## См. также

- [MVP Spec](mvp-spec.md)
- [Балансировка нагрузки](balancing.md)
- [Наблюдаемость](observability.md)
- [Реализация sidecar](implementation.md)
