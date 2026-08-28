package tunnel

import (
	"encoding/json"
	"fmt"
	"time"
)

type MessageType string

const (
	MsgRegister   MessageType = "register"
	MsgHeartbeat  MessageType = "heartbeat"
	MsgRequest    MessageType = "request"
	MsgResponse   MessageType = "response"
	MsgError      MessageType = "error"
	MsgUnregister MessageType = "unregister"
)

type Message struct {
	ID        string          `json:"id"`
	Type      MessageType     `json:"type"`
	ClusterID string          `json:"cluster_id,omitempty"`
	Token     string          `json:"token,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
}

type RegisterPayload struct {
	ClusterID    string `json:"cluster_id"`
	AgentVersion string `json:"agent_version,omitempty"`
	KubeVersion  string `json:"kube_version,omitempty"`
	Hostname     string `json:"hostname,omitempty"`
}

type HeartbeatPayload struct {
	ClusterID string `json:"cluster_id"`
	NodeCount int    `json:"node_count,omitempty"`
	CPUCores  int    `json:"cpu_cores,omitempty"`
	MemoryMB  int64  `json:"memory_mb,omitempty"`
}

type RequestPayload struct {
	RequestID string            `json:"request_id"`
	Method    string            `json:"method"`
	Path      string            `json:"path"`
	RawQuery  string            `json:"raw_query,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      []byte            `json:"body,omitempty"`
}

type ResponsePayload struct {
	RequestID  string            `json:"request_id"`
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       []byte            `json:"body,omitempty"`
}

type AgentInfo struct {
	ClusterID     string    `json:"cluster_id"`
	AgentVersion  string    `json:"agent_version,omitempty"`
	KubeVersion   string    `json:"kube_version,omitempty"`
	Hostname      string    `json:"hostname,omitempty"`
	ConnectedAt   time.Time `json:"connected_at"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
	NodeCount     int       `json:"node_count"`
	Status        string    `json:"status"`
}

func NewMessage(msgType MessageType, clusterID string, payload any) (*Message, error) {
	var data json.RawMessage
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}
		data = raw
	}

	return &Message{
		Type:      msgType,
		ClusterID: clusterID,
		Payload:   data,
		Timestamp: time.Now().UTC(),
	}, nil
}
