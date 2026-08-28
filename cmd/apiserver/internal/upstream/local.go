package upstream

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type LocalHandler func(req *InternalRequest) (*InternalResponse, error)

type LocalUpstream struct {
	name    string
	handler LocalHandler
	healthy bool
}

func NewLocalUpstream(name string, handler LocalHandler) *LocalUpstream {
	return &LocalUpstream{
		name:    name,
		handler: handler,
		healthy: true,
	}
}

func (u *LocalUpstream) Name() string { return u.name }

func (u *LocalUpstream) Call(req *InternalRequest) (*InternalResponse, error) {
	if u.handler == nil {
		return nil, fmt.Errorf("no handler registered for %s", u.name)
	}
	return u.handler(req)
}

func (u *LocalUpstream) Health() bool { return u.healthy }

func HTTPHandlerToLocal(h func(http.ResponseWriter, *http.Request)) LocalHandler {
	return func(req *InternalRequest) (*InternalResponse, error) {
		return &InternalResponse{
			StatusCode: http.StatusOK,
			Body:       []byte("{}"),
		}, nil
	}
}

func EnsureJSON(body []byte) json.RawMessage {
	if len(body) == 0 {
		return json.RawMessage("null")
	}
	if body[0] != '{' && body[0] != '[' {
		return json.RawMessage(fmt.Sprintf(`"%s"`, string(body)))
	}
	return json.RawMessage(body)
}
