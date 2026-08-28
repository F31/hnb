package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"log"
)

type TracingMW struct{}

func NewTracing() *TracingMW { return &TracingMW{} }

func (t *TracingMW) Name() string { return "tracing" }

func (t *TracingMW) Handle(ctx *Context, next func()) {
	traceID := ctx.Request.Header.Get("X-Trace-ID")
	if traceID == "" {
		b := make([]byte, 16)
		rand.Read(b)
		traceID = "trace-" + hex.EncodeToString(b)
	}
	ctx.TraceID = traceID
	ctx.Response.Header().Set("X-Trace-ID", traceID)

	log.Printf("[trace] %s %s %s", traceID, ctx.Request.Method, ctx.Request.URL.Path)

	next()
}

func init() {
	_ = rand.Reader
}
