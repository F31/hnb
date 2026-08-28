package middleware

import (
	"net/http"
	"time"
)

type CORSMW struct{}

func NewCORS() *CORSMW { return &CORSMW{} }

func (c *CORSMW) Name() string { return "cors" }

func (c *CORSMW) Handle(ctx *Context, next func()) {
	w := ctx.Response
	r := ctx.Request

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, If-Match, Idempotency-Key, X-Correlation-ID, traceparent")
	w.Header().Set("Access-Control-Max-Age", "86400")

	if r.Method == "OPTIONS" {
		ctx.Abort(http.StatusNoContent, nil)
		return
	}

	next()
}

func init() {
	_ = time.Now
}
