package middleware

import "net/http"

type Context struct {
	Request     *http.Request
	Response    http.ResponseWriter
	Params      map[string]string
	TenantID    string
	UserID      string
	WorkspaceID string
	RequestID   string
	TraceID     string
	Aborted     bool
	abortCode   int
	abortBody   []byte
}

func (c *Context) Abort(code int, body []byte) {
	if c.Aborted {
		return
	}
	c.Aborted = true
	c.abortCode = code
	c.abortBody = body
	if len(body) > 0 {
		c.Response.Header().Set("Content-Type", "application/json")
	}
	c.Response.WriteHeader(code)
	if len(body) > 0 {
		_, _ = c.Response.Write(body)
	}
}

type Middleware interface {
	Name() string
	Handle(ctx *Context, next func())
}

type Chain struct {
	middlewares []Middleware
}

func NewChain(middlewares ...Middleware) *Chain {
	return &Chain{middlewares: middlewares}
}

func (c *Chain) Then(handler func(*Context)) func(*Context) {
	return func(ctx *Context) {
		if ctx.Aborted {
			return
		}
		c.exec(0, ctx, handler)
	}
}

func (c *Chain) exec(index int, ctx *Context, handler func(*Context)) {
	if ctx.Aborted || index >= len(c.middlewares) {
		if !ctx.Aborted {
			handler(ctx)
		}
		return
	}
	c.middlewares[index].Handle(ctx, func() {
		c.exec(index+1, ctx, handler)
	})
}

func (c *Chain) Add(mw Middleware) {
	c.middlewares = append(c.middlewares, mw)
}

func (c *Chain) Middlewares() []Middleware {
	return c.middlewares
}
