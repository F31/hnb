package middleware

import "log"

func (r *RecoveryMW) Handle(ctx *Context, next func()) {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("[recovery] panic: %v", err)
			ctx.Abort(500, []byte(`{"code":50000,"message":"internal server error"}`))
		}
	}()
	next()
}

type RecoveryMW struct{}

func (r *RecoveryMW) Name() string { return "recovery" }

func NewRecovery() *RecoveryMW { return &RecoveryMW{} }
