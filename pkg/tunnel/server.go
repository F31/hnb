package tunnel

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	tunnelAgentConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "hnb_tunnel_agent_connections",
		Help: "Current number of connected agents",
	})

	tunnelMessagesSent = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hnb_tunnel_messages_sent_total",
		Help: "Total tunnel messages sent",
	}, []string{"type"})

	tunnelMessagesReceived = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "hnb_tunnel_messages_received_total",
		Help: "Total tunnel messages received",
	}, []string{"type"})

	tunnelRequestDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "hnb_tunnel_request_duration_seconds",
		Help:    "Tunnel request round-trip duration",
		Buckets: prometheus.DefBuckets,
	})
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  65536,
	WriteBufferSize: 65536,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type AgentRegistry struct {
	mu     sync.RWMutex
	agents map[string]*AgentConnection
}

func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{
		agents: make(map[string]*AgentConnection),
	}
}

func (r *AgentRegistry) Register(clusterID string, conn *AgentConnection) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.agents[clusterID]; ok {
		log.Printf("[tunnel] replacing existing agent %s", clusterID)
		existing.Close()
	}

	r.agents[clusterID] = conn
	tunnelAgentConnections.Set(float64(len(r.agents)))
	log.Printf("[tunnel] agent %s registered (total: %d)", clusterID, len(r.agents))
}

func (r *AgentRegistry) Unregister(clusterID string, expected *AgentConnection) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if conn, ok := r.agents[clusterID]; ok && conn == expected {
		conn.Close()
		delete(r.agents, clusterID)
		tunnelAgentConnections.Set(float64(len(r.agents)))
		log.Printf("[tunnel] agent %s unregistered (total: %d)", clusterID, len(r.agents))
	}
}

func (r *AgentRegistry) Get(clusterID string) (*AgentConnection, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	conn, ok := r.agents[clusterID]
	return conn, ok
}

func (r *AgentRegistry) List() []AgentInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]AgentInfo, 0, len(r.agents))
	for _, conn := range r.agents {
		infos = append(infos, conn.Info)
	}
	return infos
}

func (r *AgentRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.agents)
}

type AgentConnection struct {
	ClusterID string
	Conn      *websocket.Conn
	Info      AgentInfo
	mu        sync.Mutex
	pendingMu sync.Mutex
	pending   map[string]chan *ResponsePayload
	closed    chan struct{}
	closeOnce sync.Once
}

func NewAgentConnection(clusterID string, conn *websocket.Conn, info AgentInfo) *AgentConnection {
	info.ConnectedAt = time.Now().UTC()
	info.LastHeartbeat = time.Now().UTC()
	return &AgentConnection{
		ClusterID: clusterID,
		Conn:      conn,
		Info:      info,
		pending:   make(map[string]chan *ResponsePayload),
		closed:    make(chan struct{}),
	}
}

func (ac *AgentConnection) SendMessage(msg *Message) error {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if msg.ID == "" {
		msg.ID = fmt.Sprintf("%s-%d", ac.ClusterID, time.Now().UnixNano())
	}
	msg.ClusterID = ac.ClusterID
	msg.Timestamp = time.Now().UTC()

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	if err := ac.Conn.SetWriteDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return err
	}
	if err := ac.Conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("write message: %w", err)
	}

	tunnelMessagesSent.WithLabelValues(string(msg.Type)).Inc()
	return nil
}

func (ac *AgentConnection) ReadMessage() (*Message, error) {
	_, data, err := ac.Conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("read message: %w", err)
	}

	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("unmarshal message: %w", err)
	}

	tunnelMessagesReceived.WithLabelValues(string(msg.Type)).Inc()
	return &msg, nil
}

func (ac *AgentConnection) SendRequest(req *RequestPayload) (*ResponsePayload, error) {
	response := make(chan *ResponsePayload, 1)
	ac.pendingMu.Lock()
	if _, exists := ac.pending[req.RequestID]; exists {
		ac.pendingMu.Unlock()
		return nil, fmt.Errorf("request %q is already pending", req.RequestID)
	}
	ac.pending[req.RequestID] = response
	ac.pendingMu.Unlock()
	defer func() {
		ac.pendingMu.Lock()
		delete(ac.pending, req.RequestID)
		ac.pendingMu.Unlock()
	}()

	msg, err := NewMessage(MsgRequest, ac.ClusterID, req)
	if err != nil {
		return nil, err
	}
	msg.ID = req.RequestID

	start := time.Now()
	if err := ac.SendMessage(msg); err != nil {
		return nil, err
	}

	select {
	case resp := <-response:
		tunnelRequestDuration.Observe(time.Since(start).Seconds())
		return resp, nil
	case <-ac.closed:
		return nil, fmt.Errorf("agent connection closed")
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("agent response timeout")
	}
}

func (ac *AgentConnection) DeliverResponse(payload json.RawMessage) error {
	var resp ResponsePayload
	if err := json.Unmarshal(payload, &resp); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}
	ac.pendingMu.Lock()
	response, ok := ac.pending[resp.RequestID]
	ac.pendingMu.Unlock()
	if !ok {
		return fmt.Errorf("no pending request %q", resp.RequestID)
	}
	response <- &resp
	return nil
}

func (ac *AgentConnection) Close() {
	ac.closeOnce.Do(func() {
		ac.mu.Lock()
		defer ac.mu.Unlock()
		ac.Conn.Close()
		close(ac.closed)
	})
}

func (ac *AgentConnection) CloseExpired() {
	ac.closeOnce.Do(func() {
		ac.mu.Lock()
		defer ac.mu.Unlock()
		deadline := time.Now().Add(time.Second)
		_ = ac.Conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "service token expired"), deadline)
		_ = ac.Conn.Close()
		close(ac.closed)
	})
}

func (ac *AgentConnection) UpdateHeartbeat(payload HeartbeatPayload) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.Info.LastHeartbeat = time.Now().UTC()
	ac.Info.NodeCount = payload.NodeCount
}
