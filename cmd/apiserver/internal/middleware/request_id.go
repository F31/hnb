package middleware

import (
	"strings"

	"github.com/google/uuid"
)

type RequestIDMW struct{}

func (r *RequestIDMW) Name() string { return "request_id" }

func NewRequestID() *RequestIDMW { return &RequestIDMW{} }

func (r *RequestIDMW) Handle(ctx *Context, next func()) {
	id := strings.ToLower(ctx.Request.Header.Get("X-Trace-Id"))
	if id == "" {
		id = strings.ToLower(ctx.Request.Header.Get("X-Correlation-ID"))
	}
	parsed, err := uuid.Parse(id)
	if err != nil || parsed.String() != id {
		id = uuid.NewString()
	}
	ctx.RequestID = id
	ctx.Request.Header.Set("X-Correlation-ID", id)
	ctx.Request.Header.Set("X-Trace-Id", id)
	ctx.Response.Header().Set("X-Correlation-ID", id)
	ctx.Response.Header().Set("X-Trace-Id", id)
	next()
}
