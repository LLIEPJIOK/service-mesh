package domain

type NextFunc func(*ConnContext) error

type Handler interface {
	Handle(ctx *ConnContext, next NextFunc) error
}

type chainHandler struct {
	middlewares []Handler
}

func Chain(middlewares ...Handler) Handler {
	return &chainHandler{middlewares: middlewares}
}

func (c *chainHandler) Handle(ctx *ConnContext, next NextFunc) error {
	return c.dispatch(0, ctx, next)
}

func (c *chainHandler) dispatch(index int, ctx *ConnContext, next NextFunc) error {
	if index >= len(c.middlewares) {
		return next(ctx)
	}

	return c.middlewares[index].Handle(ctx, func(nextCtx *ConnContext) error {
		return c.dispatch(index+1, nextCtx, next)
	})
}
