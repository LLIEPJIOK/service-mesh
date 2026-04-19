# Appendix: Code Snippets

Документ содержит длинные reference-сниппеты, вынесенные из основных разделов. Примеры показывают рекомендуемую структуру, но могут требовать адаптации под фактический код проекта.

## Middleware chain и контекст

```go
package proxy

import (
    "context"
    "net"
)

type ConnContext struct {
    Context     context.Context
    ClientConn  net.Conn
    OriginalDst string
    Metadata    map[string]any
}

type Handler interface {
    Handle(ctx *ConnContext, next func(*ConnContext) error) error
}

func Chain(middlewares ...Handler) Handler {
    return &chainHandler{middlewares: middlewares}
}

type chainHandler struct {
    middlewares []Handler
}

func (c *chainHandler) Handle(ctx *ConnContext, next func(*ConnContext) error) error {
    return c.dispatch(0, ctx, next)
}

func (c *chainHandler) dispatch(i int, ctx *ConnContext, next func(*ConnContext) error) error {
    if i >= len(c.middlewares) {
        return next(ctx)
    }

    return c.middlewares[i].Handle(ctx, func(nextCtx *ConnContext) error {
        return c.dispatch(i+1, nextCtx, next)
    })
}
```

## SO_ORIGINAL_DST и TransparentListener

> [!IMPORTANT]
> Для получения `SO_ORIGINAL_DST` предпочтительно использовать `SyscallConn().Control(...)` (RawConn), чтобы избежать лишнего дублирования файловых дескрипторов. Вызов `conn.File()` создает дополнительный FD и при высокой нагрузке может приводить к избыточным аллокациям/дескрипторам.

> [!NOTE]
> Если используется fallback-вариант с `conn.File()`, его следует вызывать не чаще одного раза на соединение и всегда закрывать дубликат FD.

```go
package proxy

import (
    "context"
    "encoding/binary"
    "net"
    "strconv"
    "syscall"
)

const soOriginalDst = 80

func GetOriginalDst(conn *net.TCPConn) (string, error) {
    rawConn, err := conn.SyscallConn()
    if err != nil {
        return "", err
    }

    var (
        dst      string
        innerErr error
    )

    if err := rawConn.Control(func(fd uintptr) {
        raw, sockErr := syscall.GetsockoptIPv6Mreq(int(fd), syscall.IPPROTO_IP, soOriginalDst)
        if sockErr != nil {
            innerErr = sockErr
            return
        }

        port := binary.BigEndian.Uint16(raw.Multiaddr[2:4])
        ip := net.IPv4(raw.Multiaddr[4], raw.Multiaddr[5], raw.Multiaddr[6], raw.Multiaddr[7])
        dst = net.JoinHostPort(ip.String(), strconv.Itoa(int(port)))
    }); err != nil {
        return "", err
    }
    if innerErr != nil {
        return "", innerErr
    }
    return dst, nil
}

type TransparentListener struct {
    listener net.Listener
}

func NewTransparentListener(addr string) (*TransparentListener, error) {
    l, err := net.Listen("tcp", addr)
    if err != nil {
        return nil, err
    }

    return &TransparentListener{listener: l}, nil
}

func (l *TransparentListener) Accept() (*ConnContext, error) {
    conn, err := l.listener.Accept()
    if err != nil {
        return nil, err
    }

    originalDst := conn.LocalAddr().String()
    if tcpConn, ok := conn.(*net.TCPConn); ok {
        if dst, dstErr := GetOriginalDst(tcpConn); dstErr == nil {
            originalDst = dst
        }
    }

    return &ConnContext{
        Context:     context.Background(),
        ClientConn:  conn,
        OriginalDst: originalDst,
        Metadata:    make(map[string]any),
    }, nil
}
```

## Forwarder с выбором endpoint и mTLS

```go
package proxy

import (
    "crypto/tls"
    "io"
    "math/rand"
    "net"
    "strconv"

    "proxy/internal/discovery"
    "proxy/internal/mtls"
)

type Forwarder struct {
    UseTLS                bool
    TLSConfig             *tls.Config
    Cache                 *discovery.ServiceCache
    LoadBalancerAlgorithm string // roundRobin | random
    RoundRobinState       map[string]int // для хранения состояния round-robin по каждому сервису
}

func (f *Forwarder) selectEndpoint(originalDst string) (discovery.Endpoint, bool) {
    endpoints := f.Cache.GetEndpoints(originalDst)
    if len(endpoints) == 0 {
        return discovery.Endpoint{}, false
    }

    if f.LoadBalancerAlgorithm == "random" {
        return endpoints[rand.Intn(len(endpoints))], true
    }

    // roundRobin
    target := endpoints[f.RoundRobinState[originalDst]]
    f.RoundRobinState[originalDst] = (f.RoundRobinState[originalDst] + 1) % len(endpoints)

    return target, true
}

func (f *Forwarder) Handle(ctx *ConnContext) error {
    ep, inMesh := f.selectEndpoint(ctx.OriginalDst)
    targetAddr := ctx.OriginalDst
    serverName := ""

    if inMesh {
        // Жестко направляем на порт mTLS принимающего сайдкара
    	targetAddr = net.JoinHostPort(ep.IP, "15001")
        serverName = ep.ServiceName
    }

    var (
        targetConn net.Conn
        err        error
    )

    if f.UseTLS && inMesh {
        targetConn, err = mtls.ClientMTLS(targetAddr, serverName, f.TLSConfig)
    } else {
        targetConn, err = net.Dial("tcp", targetAddr)
    }
    if err != nil {
        return err
    }
    defer targetConn.Close()

    errCh := make(chan error, 2)

    go func() {
        _, copyErr := io.Copy(targetConn, ctx.ClientConn)
        errCh <- copyErr
    }()

    go func() {
        _, copyErr := io.Copy(ctx.ClientConn, targetConn)
        errCh <- copyErr
    }()

    return <-errCh
}
```

## mTLS Client signature

```go
func ClientMTLS(addr string, serverName string, tlsConfig *tls.Config) (net.Conn, error)
```

## Service Discovery LIST WATCH

```go
func (c *Controller) Sync(ctx context.Context) error {
    if err := c.buildServiceIPMap(ctx); err != nil {
        return err
    }

    if err := c.initialList(ctx); err != nil {
        return err
    }

    for {
        if err := c.watchLoop(ctx); err != nil {
            if ctx.Err() != nil {
                return ctx.Err()
            }
            if err := c.initialList(ctx); err != nil {
                return err
            }
            continue
        }

        return nil
    }
}
```

## Graceful shutdown skeleton

```go
func run(ctx context.Context, srv *proxyServer) error {
    go func() {
        <-ctx.Done()
        _ = srv.listener.Close()
    }()

    for {
        connCtx, err := srv.listener.Accept()
        if err != nil {
            if ctx.Err() != nil {
                break
            }
            continue
        }

        srv.wg.Add(1)
        go func(c *proxy.ConnContext) {
            defer srv.wg.Done()
            _ = srv.chain.Handle(c, srv.forwarder.Handle)
        }(connCtx)
    }

    done := make(chan struct{})
    go func() {
        defer close(done)
        srv.wg.Wait()
    }()

    select {
    case <-done:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

## См. также

- [MVP Spec](mvp-spec.md)
- [Реализация sidecar](implementation.md)
