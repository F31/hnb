package upstream

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

type NATSUpstream struct {
	name    string
	subject string
	nc      *nats.Conn
	timeout time.Duration
	healthy bool
}

func NewNATSUpstream(name, subject string, nc *nats.Conn, timeout time.Duration) *NATSUpstream {
	return &NATSUpstream{
		name:    name,
		subject: subject,
		nc:      nc,
		timeout: timeout,
		healthy: true,
	}
}

func (u *NATSUpstream) Name() string { return u.name }

func (u *NATSUpstream) Call(req *InternalRequest) (*InternalResponse, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	msg, err := u.nc.Request(u.subject, data, u.timeout)
	if err != nil {
		u.healthy = false
		return nil, fmt.Errorf("nats request: %w", err)
	}

	var resp InternalResponse
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	u.healthy = true
	return &resp, nil
}

func (u *NATSUpstream) Health() bool { return u.healthy }

func (u *NATSUpstream) SetHealthy(h bool) { u.healthy = h }
